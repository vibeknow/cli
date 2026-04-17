# Auth Login Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `vibeknow auth login` with Device Code Flow, PAT support, token auto-refresh, and default profile auto-creation — reducing first-time setup from 4 manual steps to 1 command.

**Architecture:** Device Code Flow (RFC 8628) as primary login, PAT as Agent/CI fallback. Token lifecycle managed via three-level state model (valid/needs_refresh/expired) with dual-layer locking (singleflight + flock). New `RefreshableTokenProvider` interface extends existing `TokenProvider` for backward-compatible 401 retry.

**Tech Stack:** Go 1.25, Cobra CLI, 99designs/keyring, gofrs/flock, golang.org/x/sync/singleflight

**Spec:** `docs/superpowers/specs/2026-04-17-auth-login-design.md` v1.1

---

## File Map

### New Files

| File | Responsibility |
|------|---------------|
| `internal/credential/token.go` | `StoredToken` struct, JSON marshal/unmarshal, `ParseStored()`, three-level status |
| `internal/credential/token_test.go` | Unit tests for StoredToken |
| `internal/credential/refresh_lock.go` | Cross-process flock + double-check refresh lock |
| `internal/credential/refresh_lock_test.go` | Unit tests for refresh lock |
| `internal/httpclient/mw_refresh_retry.go` | `RefreshRetryMiddleware` — intercepts 401, type-asserts `RefreshableTokenProvider`, retries |
| `internal/httpclient/mw_refresh_retry_test.go` | Unit tests for refresh retry middleware |
| `internal/cliauth/token_provider.go` | `RefreshableTokenProvider` implementation — `Token()` with three-level pre-check, `ForceRefresh()`, `TokenType()` |
| `internal/cliauth/token_provider_test.go` | Unit tests for token provider |
| `client/account/device.go` | `DeviceCode()`, `DeviceToken()` — unauthenticated account client methods |
| `client/account/device_test.go` | Unit tests with httptest server |
| `client/account/refresh.go` | `RefreshToken()` method |
| `client/account/refresh_test.go` | Unit tests with httptest server |
| `cmd/auth/login.go` | `loginCmd` — interactive, --with-token, --no-wait, --device-code |
| `tests/integration/login_flow_test.go` | End-to-end login integration test |

### Modified Files

| File | Change |
|------|--------|
| `internal/httpclient/mw_auth.go` | Move `TokenProvider` interface to its own file, add `RefreshableTokenProvider` |
| `internal/httpclient/transport.go` | Insert `RefreshRetryMiddleware` into `StandardChain()` |
| `internal/credential/resolver.go` | `KeychainSource.Get()` calls `ParseStored()` to extract access_token from JSON |
| `internal/cliauth/resolver.go` | Add `TokenProviderFor()` function |
| `cmd/auth/auth.go` | Register `loginCmd` |
| `cmd/auth/status.go` | Enhanced output with token expiry, type, status |
| `go.mod` | Add `golang.org/x/sync` dependency (for singleflight) |

---

## Task Dependency Graph

```
Task 1 (StoredToken)
  ↓
Task 2 (Account Client: device + refresh)
  ↓
Task 3 (RefreshableTokenProvider interface + middleware)
  ↓
Task 4 (Refresh Lock)
  ↓
Task 5 (cliauth TokenProvider)
  ↓
Task 6 (Resolver + StandardChain wiring)
  ↓
Task 7 (login command)
  ↓
Task 8 (status enhancement)
  ↓
Task 9 (integration test)
```

---

### Task 1: StoredToken — Credential JSON Structure

**Files:**
- Create: `internal/credential/token.go`
- Create: `internal/credential/token_test.go`

- [ ] **Step 1: Write failing tests for StoredToken**

```go
// internal/credential/token_test.go
package credential

import (
	"testing"
	"time"
)

func TestParseStored_JSON(t *testing.T) {
	raw := `{"version":"1","access_token":"at_abc","refresh_token":"rt_xyz","token_type":"oauth","expires_at":"2026-04-17T16:00:00Z","refresh_expires_at":"2026-05-17T14:00:00Z"}`
	st := ParseStored(raw)
	if st.AccessToken != "at_abc" {
		t.Fatalf("access_token = %q, want at_abc", st.AccessToken)
	}
	if st.RefreshToken != "rt_xyz" {
		t.Fatalf("refresh_token = %q, want rt_xyz", st.RefreshToken)
	}
	if st.TokenType != "oauth" {
		t.Fatalf("token_type = %q, want oauth", st.TokenType)
	}
	if st.ExpiresAt.IsZero() {
		t.Fatal("expires_at is zero")
	}
}

func TestParseStored_PlainString(t *testing.T) {
	raw := "eyJhbGciOiJSUzI1NiJ9.payload.sig"
	st := ParseStored(raw)
	if st.AccessToken != raw {
		t.Fatalf("access_token = %q, want plain token", st.AccessToken)
	}
	if st.TokenType != "pat" {
		t.Fatalf("token_type = %q, want pat", st.TokenType)
	}
	if !st.ExpiresAt.IsZero() {
		t.Fatal("expires_at should be zero for plain string")
	}
}

func TestStoredToken_Status_Valid(t *testing.T) {
	st := StoredToken{
		TokenType:        "oauth",
		ExpiresAt:        time.Now().Add(1 * time.Hour),
		RefreshExpiresAt: time.Now().Add(30 * 24 * time.Hour),
	}
	if s := st.Status(); s != StatusValid {
		t.Fatalf("status = %q, want valid", s)
	}
}

func TestStoredToken_Status_NeedsRefresh(t *testing.T) {
	st := StoredToken{
		TokenType:        "oauth",
		ExpiresAt:        time.Now().Add(3 * time.Minute), // within 5min window
		RefreshExpiresAt: time.Now().Add(30 * 24 * time.Hour),
	}
	if s := st.Status(); s != StatusNeedsRefresh {
		t.Fatalf("status = %q, want needs_refresh", s)
	}
}

func TestStoredToken_Status_Expired(t *testing.T) {
	st := StoredToken{
		TokenType:        "oauth",
		ExpiresAt:        time.Now().Add(-1 * time.Hour),
		RefreshExpiresAt: time.Now().Add(-1 * time.Hour),
	}
	if s := st.Status(); s != StatusExpired {
		t.Fatalf("status = %q, want expired", s)
	}
}

func TestStoredToken_Status_PAT(t *testing.T) {
	st := StoredToken{TokenType: "pat", AccessToken: "pat_abc"}
	if s := st.Status(); s != StatusValid {
		t.Fatalf("PAT status = %q, want valid", s)
	}
}

func TestStoredToken_Marshal(t *testing.T) {
	st := StoredToken{
		Version:          "1",
		AccessToken:      "at_abc",
		RefreshToken:     "rt_xyz",
		TokenType:        "oauth",
		ExpiresAt:        time.Date(2026, 4, 17, 16, 0, 0, 0, time.UTC),
		RefreshExpiresAt: time.Date(2026, 5, 17, 14, 0, 0, 0, time.UTC),
	}
	data := st.Marshal()
	st2 := ParseStored(string(data))
	if st2.AccessToken != st.AccessToken || st2.RefreshToken != st.RefreshToken {
		t.Fatalf("round-trip failed: got %+v", st2)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/nullkey/laoshen/vibeknow-cli && go test ./internal/credential/ -run TestParseStored -v`
Expected: FAIL — `ParseStored` not defined

- [ ] **Step 3: Implement StoredToken**

```go
// internal/credential/token.go
package credential

import (
	"encoding/json"
	"time"
)

const (
	StatusValid        = "valid"
	StatusNeedsRefresh = "needs_refresh"
	StatusExpired      = "expired"

	refreshWindow = 5 * time.Minute
)

type StoredToken struct {
	Version          string    `json:"version,omitempty"`
	AccessToken      string    `json:"access_token"`
	RefreshToken     string    `json:"refresh_token,omitempty"`
	TokenType        string    `json:"token_type"`
	ExpiresAt        time.Time `json:"expires_at,omitempty"`
	RefreshExpiresAt time.Time `json:"refresh_expires_at,omitempty"`
}

// ParseStored parses a keychain value as JSON StoredToken.
// If the value is not JSON, treats it as a plain access_token (PAT-like, no refresh).
func ParseStored(raw string) StoredToken {
	var st StoredToken
	if err := json.Unmarshal([]byte(raw), &st); err != nil || st.AccessToken == "" {
		return StoredToken{
			AccessToken: raw,
			TokenType:   "pat",
		}
	}
	if st.TokenType == "" {
		st.TokenType = "oauth"
	}
	return st
}

// Status returns the three-level token status.
// PAT tokens are always valid (no expiry).
func (st StoredToken) Status() string {
	if st.TokenType == "pat" {
		return StatusValid
	}
	now := time.Now()
	if !st.ExpiresAt.IsZero() && now.Before(st.ExpiresAt.Add(-refreshWindow)) {
		return StatusValid
	}
	if !st.RefreshExpiresAt.IsZero() && now.Before(st.RefreshExpiresAt) {
		return StatusNeedsRefresh
	}
	if st.ExpiresAt.IsZero() && st.RefreshExpiresAt.IsZero() {
		return StatusValid // no expiry info → treat as valid (legacy)
	}
	return StatusExpired
}

// Marshal serializes the StoredToken to JSON bytes for keychain storage.
func (st StoredToken) Marshal() []byte {
	data, _ := json.Marshal(st)
	return data
}

// NewOAuthToken creates a StoredToken from a device/token or refresh response.
// expiresIn and refreshExpiresIn are in seconds. A 30s safety margin is subtracted.
func NewOAuthToken(accessToken, refreshToken string, expiresIn, refreshExpiresIn int) StoredToken {
	const safetyMargin = 30 * time.Second
	now := time.Now()
	st := StoredToken{
		Version:      "1",
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		TokenType:    "oauth",
	}
	if expiresIn > 0 {
		st.ExpiresAt = now.Add(time.Duration(expiresIn)*time.Second - safetyMargin)
	}
	if refreshExpiresIn > 0 {
		st.RefreshExpiresAt = now.Add(time.Duration(refreshExpiresIn)*time.Second - safetyMargin)
	}
	return st
}

// NewPATToken creates a StoredToken for a PAT (no refresh, no expiry).
func NewPATToken(token string) StoredToken {
	return StoredToken{
		Version:     "1",
		AccessToken: token,
		TokenType:   "pat",
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/nullkey/laoshen/vibeknow-cli && go test ./internal/credential/ -run "TestParseStored|TestStoredToken" -v`
Expected: All PASS

- [ ] **Step 5: Commit**

```bash
cd /Users/nullkey/laoshen/vibeknow-cli && git add internal/credential/token.go internal/credential/token_test.go && git commit -m "feat(credential): add StoredToken with JSON storage and three-level status"
```

---

### Task 2: Account Client — Device Code + Refresh Endpoints

**Files:**
- Create: `client/account/device.go`
- Create: `client/account/device_test.go`
- Create: `client/account/refresh.go`
- Create: `client/account/refresh_test.go`

- [ ] **Step 1: Write failing tests for DeviceCode and DeviceToken**

```go
// client/account/device_test.go
package account

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDeviceCode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/v1/auth/device/code" {
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
		var body struct {
			ClientID string `json:"client_id"`
			Scope    string `json:"scope"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		if body.ClientID != "vibeknow-cli" {
			t.Fatalf("client_id = %q", body.ClientID)
		}
		json.NewEncoder(w).Encode(map[string]any{
			"code": 0,
			"data": map[string]any{
				"device_code":      "dc_test123",
				"user_code":        "ABCD-1234",
				"verification_uri": "https://vibeknow.com/device",
				"expires_in":       900,
				"interval":         5,
			},
		})
	}))
	defer srv.Close()

	c := NewUnauthenticated(srv.URL)
	resp, err := c.DeviceCode(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if resp.DeviceCode != "dc_test123" {
		t.Fatalf("device_code = %q", resp.DeviceCode)
	}
	if resp.UserCode != "ABCD-1234" {
		t.Fatalf("user_code = %q", resp.UserCode)
	}
	if resp.Interval != 5 {
		t.Fatalf("interval = %d", resp.Interval)
	}
}

func TestDeviceToken_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"code": 0,
			"data": map[string]any{
				"access_token":       "at_test",
				"refresh_token":      "rt_test",
				"token_type":         "Bearer",
				"expires_in":         7200,
				"refresh_expires_in": 2592000,
			},
		})
	}))
	defer srv.Close()

	c := NewUnauthenticated(srv.URL)
	resp, err := c.DeviceToken(context.Background(), "dc_test123")
	if err != nil {
		t.Fatal(err)
	}
	if resp.AccessToken != "at_test" {
		t.Fatalf("access_token = %q", resp.AccessToken)
	}
	if resp.RefreshExpiresIn != 2592000 {
		t.Fatalf("refresh_expires_in = %d", resp.RefreshExpiresIn)
	}
}

func TestDeviceToken_Pending(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"code":    40010,
			"message": "authorization_pending",
		})
	}))
	defer srv.Close()

	c := NewUnauthenticated(srv.URL)
	_, err := c.DeviceToken(context.Background(), "dc_test123")
	if err == nil {
		t.Fatal("expected error")
	}
	var pe *PollError
	if !asError(err, &pe) {
		t.Fatalf("expected PollError, got %T: %v", err, err)
	}
	if pe.Status != PollPending {
		t.Fatalf("status = %q, want pending", pe.Status)
	}
}

func TestDeviceToken_SlowDown(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"code":    40011,
			"message": "slow_down",
		})
	}))
	defer srv.Close()

	c := NewUnauthenticated(srv.URL)
	_, err := c.DeviceToken(context.Background(), "dc_test123")
	var pe *PollError
	if !asError(err, &pe) || pe.Status != PollSlowDown {
		t.Fatalf("expected PollSlowDown, got %v", err)
	}
}

func TestDeviceToken_Expired(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"code":    40012,
			"message": "expired_token",
		})
	}))
	defer srv.Close()

	c := NewUnauthenticated(srv.URL)
	_, err := c.DeviceToken(context.Background(), "dc_test123")
	var pe *PollError
	if !asError(err, &pe) || pe.Status != PollExpired {
		t.Fatalf("expected PollExpired, got %v", err)
	}
}

func asError[T any](err error, target *T) bool {
	return err != nil && func() bool {
		e, ok := err.(*PollError)
		if ok {
			*target = any(e).(T)
		}
		return ok
	}()
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/nullkey/laoshen/vibeknow-cli && go test ./client/account/ -run TestDevice -v`
Expected: FAIL — `NewUnauthenticated` not defined

- [ ] **Step 3: Implement device.go**

```go
// client/account/device.go
package account

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/vibeknow/cli/internal/httpclient"
)

// NewUnauthenticated creates a client with no auth middleware.
// Used for pre-login endpoints (device/code, device/token).
func NewUnauthenticated(baseURL string) *Client {
	return &Client{http: httpclient.New(baseURL).WithEnvelope()}
}

// DeviceCodeResponse is the response from POST /v1/auth/device/code.
type DeviceCodeResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

// DeviceCode requests a new device code for Device Authorization Grant.
func (c *Client) DeviceCode(ctx context.Context) (*DeviceCodeResponse, error) {
	body := map[string]string{
		"client_id": "vibeknow-cli",
		"scope":     "full",
	}
	var resp DeviceCodeResponse
	if err := c.http.Do(ctx, "POST", "/v1/auth/device/code", body, &resp); err != nil {
		return nil, err
	}
	if resp.Interval == 0 {
		resp.Interval = 5
	}
	if resp.ExpiresIn == 0 {
		resp.ExpiresIn = 900
	}
	return &resp, nil
}

// DeviceTokenResponse is the successful response from POST /v1/auth/device/token.
type DeviceTokenResponse struct {
	AccessToken      string `json:"access_token"`
	RefreshToken     string `json:"refresh_token"`
	TokenType        string `json:"token_type"`
	ExpiresIn        int    `json:"expires_in"`
	RefreshExpiresIn int    `json:"refresh_expires_in"`
}

// Poll status constants.
const (
	PollPending  = "authorization_pending"
	PollSlowDown = "slow_down"
	PollExpired  = "expired_token"
	PollDenied   = "access_denied"
)

// PollError represents a non-success device token poll response.
type PollError struct {
	Status  string
	Message string
}

func (e *PollError) Error() string { return fmt.Sprintf("device auth: %s", e.Status) }

// DeviceToken polls for token using device_code.
// Returns PollError for 40010-40013 codes. Returns DeviceTokenResponse on success.
func (c *Client) DeviceToken(ctx context.Context, deviceCode string) (*DeviceTokenResponse, error) {
	body := map[string]string{
		"client_id":  "vibeknow-cli",
		"device_code": deviceCode,
		"grant_type":  "device_code",
	}
	// We need raw envelope access to handle 40010-40013 as flow-control, not errors.
	// Use a temporary non-envelope client to read the raw response.
	rawClient := httpclient.New(c.http.BaseURL())
	var env struct {
		Code    int             `json:"code"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
	}
	if err := rawClient.Do(ctx, "POST", "/v1/auth/device/token", body, &env); err != nil {
		return nil, err
	}

	switch env.Code {
	case 0:
		var resp DeviceTokenResponse
		if err := json.Unmarshal(env.Data, &resp); err != nil {
			return nil, fmt.Errorf("decode device token response: %w", err)
		}
		return &resp, nil
	case 40010:
		return nil, &PollError{Status: PollPending, Message: env.Message}
	case 40011:
		return nil, &PollError{Status: PollSlowDown, Message: env.Message}
	case 40012:
		return nil, &PollError{Status: PollExpired, Message: env.Message}
	case 40013:
		return nil, &PollError{Status: PollDenied, Message: env.Message}
	default:
		return nil, fmt.Errorf("unexpected device token code %d: %s", env.Code, env.Message)
	}
}
```

- [ ] **Step 4: Check if httpclient.Client exposes BaseURL()**

The `DeviceToken` method needs raw envelope access. The current `httpclient.Client` doesn't expose `BaseURL()`. We need to add it.

Add to `internal/httpclient/client.go`:

```go
// BaseURL returns the client's base URL.
func (c *Client) BaseURL() string { return c.baseURL }
```

- [ ] **Step 5: Run device tests to verify they pass**

Run: `cd /Users/nullkey/laoshen/vibeknow-cli && go test ./client/account/ -run TestDevice -v`
Expected: All PASS

- [ ] **Step 6: Write failing tests for RefreshToken**

```go
// client/account/refresh_test.go
package account

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRefreshToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/v1/auth/token/refresh" {
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
		var body struct {
			RefreshToken string `json:"refresh_token"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		if body.RefreshToken != "rt_old" {
			t.Fatalf("refresh_token = %q", body.RefreshToken)
		}
		json.NewEncoder(w).Encode(map[string]any{
			"code": 0,
			"data": map[string]any{
				"access_token":       "at_new",
				"refresh_token":      "rt_new",
				"expires_in":         7200,
				"refresh_expires_in": 2592000,
			},
		})
	}))
	defer srv.Close()

	c := NewUnauthenticated(srv.URL)
	resp, err := c.RefreshToken(context.Background(), "rt_old")
	if err != nil {
		t.Fatal(err)
	}
	if resp.AccessToken != "at_new" {
		t.Fatalf("access_token = %q", resp.AccessToken)
	}
	if resp.RefreshToken != "rt_new" {
		t.Fatalf("refresh_token = %q", resp.RefreshToken)
	}
}
```

- [ ] **Step 7: Implement refresh.go**

```go
// client/account/refresh.go
package account

import "context"

// RefreshTokenResponse is the response from POST /v1/auth/token/refresh.
type RefreshTokenResponse struct {
	AccessToken      string `json:"access_token"`
	RefreshToken     string `json:"refresh_token"`
	ExpiresIn        int    `json:"expires_in"`
	RefreshExpiresIn int    `json:"refresh_expires_in"`
}

// RefreshToken exchanges a refresh_token for a new access_token + refresh_token.
func (c *Client) RefreshToken(ctx context.Context, refreshToken string) (*RefreshTokenResponse, error) {
	body := map[string]string{"refresh_token": refreshToken}
	var resp RefreshTokenResponse
	if err := c.http.Do(ctx, "POST", "/v1/auth/token/refresh", body, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
```

- [ ] **Step 8: Run all account tests**

Run: `cd /Users/nullkey/laoshen/vibeknow-cli && go test ./client/account/ -v`
Expected: All PASS

- [ ] **Step 9: Commit**

```bash
cd /Users/nullkey/laoshen/vibeknow-cli && git add client/account/device.go client/account/device_test.go client/account/refresh.go client/account/refresh_test.go internal/httpclient/client.go && git commit -m "feat(account): add Device Code Flow and token refresh client methods"
```

---

### Task 3: RefreshableTokenProvider Interface + RefreshRetryMiddleware

**Files:**
- Modify: `internal/httpclient/mw_auth.go`
- Create: `internal/httpclient/mw_refresh_retry.go`
- Create: `internal/httpclient/mw_refresh_retry_test.go`

- [ ] **Step 1: Add RefreshableTokenProvider to mw_auth.go**

Add the new interface below the existing `TokenProvider` in `internal/httpclient/mw_auth.go`:

```go
// RefreshableTokenProvider extends TokenProvider with refresh capability.
// Used by RefreshRetryMiddleware to handle 401 responses.
type RefreshableTokenProvider interface {
	TokenProvider
	TokenType() string
	ForceRefresh(ctx context.Context) (string, error)
}
```

- [ ] **Step 2: Write failing test for RefreshRetryMiddleware**

```go
// internal/httpclient/mw_refresh_retry_test.go
package httpclient

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"
)

type mockRefreshableProvider struct {
	token        string
	tokenType    string
	refreshCalls int32
	refreshToken string
}

func (m *mockRefreshableProvider) Token(ctx context.Context) (string, error) {
	return m.token, nil
}
func (m *mockRefreshableProvider) TokenType() string { return m.tokenType }
func (m *mockRefreshableProvider) ForceRefresh(ctx context.Context) (string, error) {
	atomic.AddInt32(&m.refreshCalls, 1)
	m.token = m.refreshToken
	return m.refreshToken, nil
}

func TestRefreshRetryMiddleware_401_OAuth(t *testing.T) {
	callCount := int32(0)
	backend := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		n := atomic.AddInt32(&callCount, 1)
		if n == 1 {
			return &http.Response{StatusCode: 401, Body: http.NoBody}, nil
		}
		// Second call should have new token
		if r.Header.Get("X-Authorization-Token") != "new_token" {
			t.Fatalf("retry did not use new token, got %q", r.Header.Get("X-Authorization-Token"))
		}
		return &http.Response{StatusCode: 200, Body: http.NoBody}, nil
	})

	provider := &mockRefreshableProvider{token: "old_token", tokenType: "oauth", refreshToken: "new_token"}
	mw := RefreshRetryMiddleware{Provider: provider}
	chain := Chain(backend, AuthMiddleware{Provider: provider}, mw)

	req, _ := http.NewRequest("GET", "http://example.com/test", nil)
	resp, err := chain.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if atomic.LoadInt32(&provider.refreshCalls) != 1 {
		t.Fatalf("refresh called %d times, want 1", provider.refreshCalls)
	}
}

func TestRefreshRetryMiddleware_401_PAT_NoRetry(t *testing.T) {
	backend := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 401, Body: http.NoBody}, nil
	})

	provider := &mockRefreshableProvider{token: "pat_abc", tokenType: "pat"}
	mw := RefreshRetryMiddleware{Provider: provider}
	chain := Chain(backend, AuthMiddleware{Provider: provider}, mw)

	req, _ := http.NewRequest("GET", "http://example.com/test", nil)
	resp, _ := chain.RoundTrip(req)
	if resp.StatusCode != 401 {
		t.Fatalf("PAT should not retry, got status %d", resp.StatusCode)
	}
	if atomic.LoadInt32(&provider.refreshCalls) != 0 {
		t.Fatal("PAT should not trigger refresh")
	}
}

func TestRefreshRetryMiddleware_NonRefreshable_NoRetry(t *testing.T) {
	backend := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 401, Body: http.NoBody}, nil
	})

	// Plain TokenProvider, not RefreshableTokenProvider
	mw := RefreshRetryMiddleware{Provider: nil}
	chain := Chain(backend, mw)

	req, _ := http.NewRequest("GET", "http://example.com/test", nil)
	resp, _ := chain.RoundTrip(req)
	if resp.StatusCode != 401 {
		t.Fatalf("non-refreshable should not retry, got status %d", resp.StatusCode)
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `cd /Users/nullkey/laoshen/vibeknow-cli && go test ./internal/httpclient/ -run TestRefreshRetry -v`
Expected: FAIL — `RefreshRetryMiddleware` not defined

- [ ] **Step 4: Implement RefreshRetryMiddleware**

```go
// internal/httpclient/mw_refresh_retry.go
package httpclient

import (
	"bytes"
	"io"
	"net/http"
)

// RefreshRetryMiddleware intercepts 401 responses and retries once after
// refreshing the token, if the provider implements RefreshableTokenProvider.
type RefreshRetryMiddleware struct {
	Provider TokenProvider
}

func (m RefreshRetryMiddleware) Wrap(next http.RoundTripper) http.RoundTripper {
	rtp, ok := m.Provider.(RefreshableTokenProvider)
	if !ok || rtp == nil {
		return next // no refresh capability, pass through
	}

	return roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		// Buffer request body for potential retry
		var bodyBytes []byte
		if r.Body != nil {
			var err error
			bodyBytes, err = io.ReadAll(r.Body)
			if err != nil {
				return nil, err
			}
			r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		}

		resp, err := next.RoundTrip(r)
		if err != nil || resp.StatusCode != 401 {
			return resp, err
		}

		// Only refresh OAuth tokens
		if rtp.TokenType() != "oauth" {
			return resp, nil
		}

		// Attempt refresh
		newToken, refreshErr := rtp.ForceRefresh(r.Context())
		if refreshErr != nil {
			return resp, nil // return original 401
		}

		// Drain and close original response
		resp.Body.Close()

		// Retry with new token
		if bodyBytes != nil {
			r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		}
		r.Header.Set("X-Authorization-Token", newToken)
		return next.RoundTrip(r)
	})
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd /Users/nullkey/laoshen/vibeknow-cli && go test ./internal/httpclient/ -run TestRefreshRetry -v`
Expected: All PASS

- [ ] **Step 6: Commit**

```bash
cd /Users/nullkey/laoshen/vibeknow-cli && git add internal/httpclient/mw_auth.go internal/httpclient/mw_refresh_retry.go internal/httpclient/mw_refresh_retry_test.go && git commit -m "feat(httpclient): add RefreshableTokenProvider and RefreshRetryMiddleware"
```

---

### Task 4: Cross-Process Refresh Lock

**Files:**
- Create: `internal/credential/refresh_lock.go`
- Create: `internal/credential/refresh_lock_test.go`
- Modify: `go.mod` — add `golang.org/x/sync`

- [ ] **Step 1: Add singleflight dependency**

Run: `cd /Users/nullkey/laoshen/vibeknow-cli && go get golang.org/x/sync`

- [ ] **Step 2: Write failing test for RefreshLock**

```go
// internal/credential/refresh_lock_test.go
package credential

import (
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
)

func TestRefreshLock_SingleProcess(t *testing.T) {
	dir := t.TempDir()
	lockDir := filepath.Join(dir, "locks")
	os.MkdirAll(lockDir, 0o700)

	rl := NewRefreshLock(lockDir, "test-ref")

	callCount := int32(0)
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = rl.Do(func() (string, error) {
				atomic.AddInt32(&callCount, 1)
				return "new_token", nil
			})
		}()
	}
	wg.Wait()

	if n := atomic.LoadInt32(&callCount); n != 1 {
		t.Fatalf("refresh called %d times, want 1 (singleflight should deduplicate)", n)
	}
}

func TestRefreshLock_DoubleCheck(t *testing.T) {
	dir := t.TempDir()
	lockDir := filepath.Join(dir, "locks")
	os.MkdirAll(lockDir, 0o700)

	rl := NewRefreshLock(lockDir, "test-ref")

	// First call should execute fn
	tok, err := rl.DoWithDoubleCheck(
		func() bool { return false }, // not already refreshed
		func() (string, error) { return "refreshed", nil },
	)
	if err != nil || tok != "refreshed" {
		t.Fatalf("tok = %q, err = %v", tok, err)
	}

	// Second call with positive check should skip fn
	tok, err = rl.DoWithDoubleCheck(
		func() bool { return true }, // already refreshed
		func() (string, error) { t.Fatal("should not be called"); return "", nil },
	)
	// When already refreshed, returns empty (caller should re-read from keychain)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `cd /Users/nullkey/laoshen/vibeknow-cli && go test ./internal/credential/ -run TestRefreshLock -v`
Expected: FAIL — `NewRefreshLock` not defined

- [ ] **Step 4: Implement RefreshLock**

```go
// internal/credential/refresh_lock.go
package credential

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gofrs/flock"
	"golang.org/x/sync/singleflight"
)

// RefreshLock provides dual-layer locking for token refresh:
// 1. Process-internal: singleflight deduplication
// 2. Cross-process: file-based flock
type RefreshLock struct {
	lockDir       string
	credentialRef string
	group         singleflight.Group
}

func NewRefreshLock(lockDir, credentialRef string) *RefreshLock {
	return &RefreshLock{
		lockDir:       lockDir,
		credentialRef: credentialRef,
	}
}

// Do executes fn at most once across concurrent goroutines in this process.
func (rl *RefreshLock) Do(fn func() (string, error)) (string, error) {
	v, err, _ := rl.group.Do(rl.credentialRef, func() (any, error) {
		return fn()
	})
	if err != nil {
		return "", err
	}
	return v.(string), nil
}

// DoWithDoubleCheck acquires the cross-process flock, then calls isAlreadyRefreshed.
// If another process already refreshed, returns ("", nil) — caller should re-read keychain.
// Otherwise calls fn under the lock.
func (rl *RefreshLock) DoWithDoubleCheck(isAlreadyRefreshed func() bool, fn func() (string, error)) (string, error) {
	return rl.Do(func() (string, error) {
		// Acquire cross-process lock
		lockPath := filepath.Join(rl.lockDir, fmt.Sprintf("refresh_%s.lock", rl.credentialRef))
		if err := os.MkdirAll(filepath.Dir(lockPath), 0o700); err != nil {
			return "", fmt.Errorf("refresh lock: mkdir: %w", err)
		}
		fl := flock.New(lockPath)

		ctx, cancel := newTimeoutCtx(30 * time.Second)
		defer cancel()
		locked, err := fl.TryLockContext(ctx, 500*time.Millisecond)
		if err != nil || !locked {
			return "", fmt.Errorf("refresh lock: timeout acquiring %s", lockPath)
		}
		defer fl.Unlock()

		// Double-check: another process may have refreshed while we waited
		if isAlreadyRefreshed() {
			return "", nil
		}

		return fn()
	})
}

func newTimeoutCtx(d time.Duration) (context.Context, context.CancelFunc) {
	return contextWithTimeout(d)
}
```

Note: We need a small helper since `context` isn't imported. Fix the import:

```go
// Add to imports at top of file:
import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gofrs/flock"
	"golang.org/x/sync/singleflight"
)

// Replace newTimeoutCtx with direct call:
func (rl *RefreshLock) DoWithDoubleCheck(isAlreadyRefreshed func() bool, fn func() (string, error)) (string, error) {
	return rl.Do(func() (string, error) {
		lockPath := filepath.Join(rl.lockDir, fmt.Sprintf("refresh_%s.lock", rl.credentialRef))
		if err := os.MkdirAll(filepath.Dir(lockPath), 0o700); err != nil {
			return "", fmt.Errorf("refresh lock: mkdir: %w", err)
		}
		fl := flock.New(lockPath)

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		locked, err := fl.TryLockContext(ctx, 500*time.Millisecond)
		if err != nil || !locked {
			return "", fmt.Errorf("refresh lock: timeout acquiring %s", lockPath)
		}
		defer fl.Unlock()

		if isAlreadyRefreshed() {
			return "", nil
		}

		return fn()
	})
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd /Users/nullkey/laoshen/vibeknow-cli && go test ./internal/credential/ -run TestRefreshLock -v`
Expected: All PASS

- [ ] **Step 6: Commit**

```bash
cd /Users/nullkey/laoshen/vibeknow-cli && git add go.mod go.sum internal/credential/refresh_lock.go internal/credential/refresh_lock_test.go && git commit -m "feat(credential): add cross-process refresh lock with singleflight + flock"
```

---

### Task 5: cliauth TokenProvider — Three-Level Pre-Check + Refresh

**Files:**
- Create: `internal/cliauth/token_provider.go`
- Create: `internal/cliauth/token_provider_test.go`
- Modify: `internal/credential/resolver.go` — `KeychainSource.Get()` handles JSON

- [ ] **Step 1: Update KeychainSource.Get() to parse JSON**

Modify `internal/credential/resolver.go`, change `KeychainSource.Get()`:

```go
func (k KeychainSource) Get() (string, error) {
	if k.Keychain == nil || k.Entry == "" {
		return "", ErrNotFound
	}
	data, err := k.Keychain.Get(k.Entry)
	if err != nil {
		return "", err
	}
	raw := string(data)
	st := ParseStored(raw)
	return st.AccessToken, nil
}

// GetStored returns the full StoredToken from keychain (for token provider use).
func (k KeychainSource) GetStored() (StoredToken, error) {
	if k.Keychain == nil || k.Entry == "" {
		return StoredToken{}, ErrNotFound
	}
	data, err := k.Keychain.Get(k.Entry)
	if err != nil {
		return StoredToken{}, err
	}
	return ParseStored(string(data)), nil
}
```

- [ ] **Step 2: Write failing test for cliauth TokenProvider**

```go
// internal/cliauth/token_provider_test.go
package cliauth

import (
	"context"
	"testing"
	"time"

	"github.com/vibeknow/cli/internal/credential"
)

type fakeKeychain struct {
	data map[string][]byte
}

func (f *fakeKeychain) Get(key string) ([]byte, error) {
	d, ok := f.data[key]
	if !ok {
		return nil, credential.ErrNotFound
	}
	return d, nil
}
func (f *fakeKeychain) Set(key string, data []byte) error {
	f.data[key] = data
	return nil
}
func (f *fakeKeychain) Delete(key string) error {
	delete(f.data, key)
	return nil
}

type fakeRefresher struct {
	callCount int
	response  *fakeRefreshResponse
	err       error
}

type fakeRefreshResponse struct {
	AccessToken      string
	RefreshToken     string
	ExpiresIn        int
	RefreshExpiresIn int
}

func (f *fakeRefresher) Refresh(ctx context.Context, refreshToken string) (*fakeRefreshResponse, error) {
	f.callCount++
	return f.response, f.err
}

func TestOAuthTokenProvider_Valid(t *testing.T) {
	kc := &fakeKeychain{data: map[string][]byte{}}
	st := credential.StoredToken{
		Version:          "1",
		AccessToken:      "at_valid",
		RefreshToken:     "rt_valid",
		TokenType:        "oauth",
		ExpiresAt:        time.Now().Add(1 * time.Hour),
		RefreshExpiresAt: time.Now().Add(30 * 24 * time.Hour),
	}
	kc.Set("vibeknow.default", st.Marshal())

	tp := NewOAuthTokenProvider(
		credential.KeychainSource{Keychain: kc, Entry: "vibeknow.default"},
		"", "", // no refresh needed for valid token
	)

	tok, err := tp.Token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if tok != "at_valid" {
		t.Fatalf("token = %q, want at_valid", tok)
	}
	if tp.TokenType() != "oauth" {
		t.Fatalf("type = %q, want oauth", tp.TokenType())
	}
}

func TestOAuthTokenProvider_PAT(t *testing.T) {
	kc := &fakeKeychain{data: map[string][]byte{}}
	st := credential.NewPATToken("pat_abc")
	kc.Set("vibeknow.default", st.Marshal())

	tp := NewOAuthTokenProvider(
		credential.KeychainSource{Keychain: kc, Entry: "vibeknow.default"},
		"", "",
	)

	tok, err := tp.Token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if tok != "pat_abc" {
		t.Fatalf("token = %q, want pat_abc", tok)
	}
	if tp.TokenType() != "pat" {
		t.Fatalf("type = %q, want pat", tp.TokenType())
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `cd /Users/nullkey/laoshen/vibeknow-cli && go test ./internal/cliauth/ -run TestOAuthTokenProvider -v`
Expected: FAIL — `NewOAuthTokenProvider` not defined

- [ ] **Step 4: Implement cliauth TokenProvider**

```go
// internal/cliauth/token_provider.go
package cliauth

import (
	"context"
	"fmt"
	"sync"

	"github.com/vibeknow/cli/client/account"
	"github.com/vibeknow/cli/internal/credential"
)

// OAuthTokenProvider implements httpclient.RefreshableTokenProvider.
// It reads StoredToken from keychain, checks three-level status,
// and refreshes automatically when needed.
type OAuthTokenProvider struct {
	keychainSrc credential.KeychainSource
	accountURL  string
	lockDir     string

	mu          sync.Mutex
	cachedToken *credential.StoredToken // in-memory fallback if keychain write fails
}

// NewOAuthTokenProvider creates a token provider that reads from keychain and refreshes via account service.
func NewOAuthTokenProvider(keychainSrc credential.KeychainSource, accountURL, lockDir string) *OAuthTokenProvider {
	return &OAuthTokenProvider{
		keychainSrc: keychainSrc,
		accountURL:  accountURL,
		lockDir:     lockDir,
	}
}

func (p *OAuthTokenProvider) loadToken() (credential.StoredToken, error) {
	p.mu.Lock()
	cached := p.cachedToken
	p.mu.Unlock()
	if cached != nil {
		return *cached, nil
	}
	return p.keychainSrc.GetStored()
}

func (p *OAuthTokenProvider) Token(ctx context.Context) (string, error) {
	st, err := p.loadToken()
	if err != nil {
		return "", fmt.Errorf("no credential available; run `vibeknow auth login` or set VIBEKNOW_TOKEN")
	}

	switch st.Status() {
	case credential.StatusValid:
		return st.AccessToken, nil
	case credential.StatusNeedsRefresh:
		newTok, refreshErr := p.doRefresh(ctx, st)
		if refreshErr != nil {
			// If refresh fails but access_token hasn't expired yet, use it
			if st.AccessToken != "" {
				return st.AccessToken, nil
			}
			return "", refreshErr
		}
		return newTok, nil
	case credential.StatusExpired:
		// Clear expired token
		if p.keychainSrc.Keychain != nil && p.keychainSrc.Entry != "" {
			p.keychainSrc.Keychain.Delete(p.keychainSrc.Entry)
		}
		return "", fmt.Errorf("login expired; run `vibeknow auth login`")
	default:
		return st.AccessToken, nil
	}
}

func (p *OAuthTokenProvider) TokenType() string {
	st, err := p.loadToken()
	if err != nil {
		return "pat" // default to pat behavior (no refresh) if can't read
	}
	return st.TokenType
}

func (p *OAuthTokenProvider) ForceRefresh(ctx context.Context) (string, error) {
	st, err := p.loadToken()
	if err != nil {
		return "", err
	}
	if st.TokenType == "pat" {
		return "", fmt.Errorf("PAT tokens cannot be refreshed")
	}
	return p.doRefresh(ctx, st)
}

func (p *OAuthTokenProvider) doRefresh(ctx context.Context, st credential.StoredToken) (string, error) {
	if st.RefreshToken == "" {
		return "", fmt.Errorf("no refresh_token available; run `vibeknow auth login`")
	}
	if p.accountURL == "" {
		return "", fmt.Errorf("account URL not configured")
	}

	rl := credential.NewRefreshLock(p.lockDir, p.keychainSrc.Entry)
	tok, err := rl.DoWithDoubleCheck(
		func() bool {
			// Re-read from keychain to check if another process refreshed
			fresh, readErr := p.keychainSrc.GetStored()
			if readErr != nil {
				return false
			}
			return fresh.Status() == credential.StatusValid && fresh.AccessToken != st.AccessToken
		},
		func() (string, error) {
			c := account.NewUnauthenticated(p.accountURL)
			resp, err := c.RefreshToken(ctx, st.RefreshToken)
			if err != nil {
				return "", err
			}

			newSt := credential.NewOAuthToken(
				resp.AccessToken, resp.RefreshToken,
				resp.ExpiresIn, resp.RefreshExpiresIn,
			)

			// Persist to keychain
			if p.keychainSrc.Keychain != nil && p.keychainSrc.Entry != "" {
				if err := p.keychainSrc.Keychain.Set(p.keychainSrc.Entry, newSt.Marshal()); err != nil {
					// Keychain write failed — cache in memory
					p.mu.Lock()
					p.cachedToken = &newSt
					p.mu.Unlock()
					// Log warning (stderr) — callers with verbose can see this
				}
			}

			return newSt.AccessToken, nil
		},
	)
	if err != nil {
		return "", err
	}
	if tok == "" {
		// Double-check found refreshed token — re-read
		fresh, err := p.keychainSrc.GetStored()
		if err != nil {
			return "", err
		}
		return fresh.AccessToken, nil
	}
	return tok, nil
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd /Users/nullkey/laoshen/vibeknow-cli && go test ./internal/cliauth/ -run TestOAuthTokenProvider -v`
Expected: All PASS

- [ ] **Step 6: Commit**

```bash
cd /Users/nullkey/laoshen/vibeknow-cli && git add internal/credential/resolver.go internal/cliauth/token_provider.go internal/cliauth/token_provider_test.go && git commit -m "feat(cliauth): add OAuthTokenProvider with three-level status and auto-refresh"
```

---

### Task 6: Wiring — Resolver + StandardChain + TokenProviderFor

**Files:**
- Modify: `internal/cliauth/resolver.go` — add `TokenProviderFor()`
- Modify: `internal/httpclient/transport.go` — insert `RefreshRetryMiddleware`

- [ ] **Step 1: Add TokenProviderFor to cliauth/resolver.go**

Add after the existing `ResolverFor` function:

```go
// TokenProviderFor builds a RefreshableTokenProvider for the given profile.
// It reads tokens from keychain, checks three-level status, and refreshes automatically.
// Falls back to env var if set (as a plain TokenProvider, no refresh).
func TokenProviderFor(p config.Profile) httpclient.TokenProvider {
	// Env var takes priority — returns plain token, no refresh
	if tok := os.Getenv("VIBEKNOW_TOKEN"); tok != "" {
		return staticEnvToken(tok)
	}

	if p.CredentialRef == "" {
		return nil
	}

	kc, err := keychain.OpenFor("vibeknow")
	if err != nil {
		return nil
	}

	accountURL, _ := endpoints.Resolve(p, "account")
	lockDir, _ := config.ConfigDir()
	if lockDir != "" {
		lockDir = filepath.Join(lockDir, "locks")
	}

	return NewOAuthTokenProvider(
		credential.KeychainSource{Keychain: kc, Entry: p.CredentialRef},
		accountURL,
		lockDir,
	)
}

type staticEnvToken string

func (s staticEnvToken) Token(ctx context.Context) (string, error) { return string(s), nil }
```

Add imports:

```go
import (
	"context"
	"os"
	"path/filepath"

	"github.com/vibeknow/cli/internal/config"
	"github.com/vibeknow/cli/internal/credential"
	"github.com/vibeknow/cli/internal/endpoints"
	"github.com/vibeknow/cli/internal/httpclient"
	"github.com/vibeknow/cli/internal/keychain"
)
```

- [ ] **Step 2: Update StandardChain to include RefreshRetryMiddleware**

Modify `internal/httpclient/transport.go`:

```go
func StandardChain(tp TokenProvider, verboseOut io.Writer) http.RoundTripper {
	return Chain(http.DefaultTransport,
		AuthMiddleware{Provider: tp},
		RefreshRetryMiddleware{Provider: tp},
		TraceIDMiddleware{},
		VerboseMiddleware{Out: verboseOut},
		VersionMiddleware{Expected: ClientAPIVersion},
		RetryMiddleware{MaxAttempts: 3, BaseDelay: 200 * time.Millisecond},
	)
}
```

- [ ] **Step 3: Run full test suite to verify no regressions**

Run: `cd /Users/nullkey/laoshen/vibeknow-cli && go test ./... -count=1`
Expected: All PASS (existing tests unaffected because they pass `nil` or plain `TokenProvider`)

- [ ] **Step 4: Commit**

```bash
cd /Users/nullkey/laoshen/vibeknow-cli && git add internal/cliauth/resolver.go internal/httpclient/transport.go && git commit -m "feat: wire TokenProviderFor and RefreshRetryMiddleware into StandardChain"
```

---

### Task 7: Login Command

**Files:**
- Create: `cmd/auth/login.go`
- Modify: `cmd/auth/auth.go` — register loginCmd

- [ ] **Step 1: Implement login.go**

```go
// cmd/auth/login.go
package auth

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/vibeknow/cli/client/account"
	"github.com/vibeknow/cli/internal/cliauth"
	"github.com/vibeknow/cli/internal/config"
	"github.com/vibeknow/cli/internal/credential"
	"github.com/vibeknow/cli/internal/endpoints"
	"github.com/vibeknow/cli/internal/keychain"
	"golang.org/x/term"
)

var (
	flagWithToken  bool
	flagNoWait     bool
	flagDeviceCode string
)

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "authenticate with VibeKnow (Device Code Flow or PAT)",
	RunE:  runLogin,
}

func init() {
	loginCmd.Flags().BoolVar(&flagWithToken, "with-token", false, "read PAT from stdin")
	loginCmd.Flags().BoolVar(&flagNoWait, "no-wait", false, "print device code and exit (Agent use)")
	loginCmd.Flags().StringVar(&flagDeviceCode, "device-code", "", "resume polling for a device code (Agent use)")
}

func runLogin(cmd *cobra.Command, args []string) error {
	// Mutual exclusion
	set := 0
	if flagWithToken {
		set++
	}
	if flagNoWait {
		set++
	}
	if flagDeviceCode != "" {
		set++
	}
	if set > 1 {
		return fmt.Errorf("--with-token, --no-wait, and --device-code are mutually exclusive")
	}

	switch {
	case flagWithToken:
		return loginWithToken(cmd)
	case flagNoWait:
		return loginNoWait(cmd)
	case flagDeviceCode != "":
		return loginResumeDeviceCode(cmd, flagDeviceCode)
	default:
		return loginInteractive(cmd)
	}
}

func loginInteractive(cmd *cobra.Command) error {
	if !isTerminal() {
		return fmt.Errorf("non-interactive environment detected; use --with-token or --no-wait")
	}

	// Warn if VIBEKNOW_TOKEN is set
	if os.Getenv("VIBEKNOW_TOKEN") != "" {
		fmt.Fprintln(cmd.ErrOrStderr(), "注意: 检测到环境变量 VIBEKNOW_TOKEN，交互式登录的凭证优先级低于环境变量")
	}

	p, accountURL, err := resolveAccountURL()
	if err != nil {
		return err
	}

	// Check if already logged in
	if p.CredentialRef != "" {
		if kc, err := keychain.OpenFor("vibeknow"); err == nil {
			src := credential.KeychainSource{Keychain: kc, Entry: p.CredentialRef}
			if st, err := src.GetStored(); err == nil && st.Status() != credential.StatusExpired {
				ac := account.New(accountURL, staticToken(st.AccessToken))
				if u, err := ac.Whoami(context.Background()); err == nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "已登录为 %s (%s)，是否重新登录？(y/N): ", u.Nickname, u.Email)
					scanner := bufio.NewScanner(os.Stdin)
					scanner.Scan()
					if strings.TrimSpace(strings.ToLower(scanner.Text())) != "y" {
						return nil
					}
				}
			}
		}
	}

	fmt.Fprintln(cmd.ErrOrStderr(), "正在请求设备验证码...")

	c := account.NewUnauthenticated(accountURL)
	dcResp, err := c.DeviceCode(context.Background())
	if err != nil {
		return fmt.Errorf("请求设备验证码失败: %w", err)
	}

	fmt.Fprintf(cmd.ErrOrStderr(), "\n✓ 验证码: %s\n", dcResp.UserCode)
	fmt.Fprintf(cmd.ErrOrStderr(), "  请在浏览器中打开: %s\n", dcResp.VerificationURI)
	fmt.Fprintln(cmd.ErrOrStderr(), "  并输入上方验证码完成登录")
	fmt.Fprintln(cmd.ErrOrStderr(), "\n  按 Enter 打开浏览器，或按 Ctrl+C 取消...")

	// Wait for Enter
	bufio.NewScanner(os.Stdin).Scan()

	// Open browser
	if err := openBrowser(dcResp.VerificationURI); err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "  无法打开浏览器，请手动访问上方链接\n")
	} else {
		fmt.Fprintln(cmd.ErrOrStderr(), "✓ 已打开浏览器")
	}

	// Poll
	tokenResp, err := pollDeviceToken(cmd, c, dcResp.DeviceCode, dcResp.Interval, dcResp.ExpiresIn)
	if err != nil {
		return err
	}

	return finishLogin(cmd, p, accountURL, tokenResp, false)
}

func loginWithToken(cmd *cobra.Command) error {
	var token string
	if isTerminal() {
		fmt.Fprint(cmd.ErrOrStderr(), "? 粘贴你的 Personal Access Token: ")
		raw, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(cmd.ErrOrStderr())
		if err != nil {
			return fmt.Errorf("读取 token 失败: %w", err)
		}
		token = strings.TrimSpace(string(raw))
	} else {
		scanner := bufio.NewScanner(os.Stdin)
		if scanner.Scan() {
			token = strings.TrimSpace(scanner.Text())
		}
	}
	if token == "" {
		return fmt.Errorf("未提供 token")
	}

	p, accountURL, err := resolveAccountURL()
	if err != nil {
		return err
	}

	// Verify token
	ac := account.New(accountURL, staticToken(token))
	u, err := ac.Whoami(context.Background())
	if err != nil {
		os.Exit(3)
		return fmt.Errorf("token 验证失败: %w", err)
	}

	// Store as PAT
	st := credential.NewPATToken(token)
	if err := storeToken(p, st); err != nil {
		return err
	}

	fmt.Fprintf(cmd.ErrOrStderr(), "✓ 登录成��！欢迎，%s", u.Nickname)
	if u.Email != "" {
		fmt.Fprintf(cmd.ErrOrStderr(), " (%s)", u.Email)
	}
	fmt.Fprintln(cmd.ErrOrStderr())
	fmt.Fprintln(cmd.ErrOrStderr(), "  凭证已保存到系统密钥链")
	return nil
}

func loginNoWait(cmd *cobra.Command) error {
	_, accountURL, err := resolveAccountURL()
	if err != nil {
		return err
	}

	c := account.NewUnauthenticated(accountURL)
	dcResp, err := c.DeviceCode(context.Background())
	if err != nil {
		return fmt.Errorf("请求设备验证码失败: %w", err)
	}

	out := map[string]any{
		"verification_uri": dcResp.VerificationURI,
		"user_code":        dcResp.UserCode,
		"device_code":      dcResp.DeviceCode,
		"expires_in":       dcResp.ExpiresIn,
		"hint":             fmt.Sprintf("请在浏览器中打开 %s 并输入 %s，然后执行: vibeknow auth login --device-code %s", dcResp.VerificationURI, dcResp.UserCode, dcResp.DeviceCode),
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func loginResumeDeviceCode(cmd *cobra.Command, deviceCode string) error {
	p, accountURL, err := resolveAccountURL()
	if err != nil {
		return err
	}

	c := account.NewUnauthenticated(accountURL)

	// Default interval/expiry since we don't have the original response
	tokenResp, err := pollDeviceToken(cmd, c, deviceCode, 5, 900)
	if err != nil {
		return err
	}

	return finishLogin(cmd, p, accountURL, tokenResp, !isTerminal())
}

// pollDeviceToken polls the device/token endpoint until success or terminal error.
func pollDeviceToken(cmd *cobra.Command, c *account.Client, deviceCode string, interval, expiresIn int) (*account.DeviceTokenResponse, error) {
	deadline := time.Now().Add(time.Duration(expiresIn) * time.Second)
	pollInterval := time.Duration(interval) * time.Second

	for {
		resp, err := c.DeviceToken(context.Background(), deviceCode)
		if err == nil {
			return resp, nil
		}

		var pe *account.PollError
		if !errors.As(err, &pe) {
			return nil, fmt.Errorf("轮询失败: %w", err)
		}

		switch pe.Status {
		case account.PollPending:
			remaining := time.Until(deadline).Truncate(time.Second)
			if isTerminal() {
				fmt.Fprintf(cmd.ErrOrStderr(), "\r⠋ 等待授权... (%s 剩余)", remaining)
			}
		case account.PollSlowDown:
			pollInterval += 5 * time.Second
		case account.PollExpired:
			fmt.Fprintln(cmd.ErrOrStderr())
			os.Exit(3)
			return nil, fmt.Errorf("验证码已过期，请重新执行 vibeknow auth login")
		case account.PollDenied:
			fmt.Fprintln(cmd.ErrOrStderr())
			os.Exit(3)
			return nil, fmt.Errorf("授权被拒绝")
		}

		if time.Now().After(deadline) {
			fmt.Fprintln(cmd.ErrOrStderr())
			os.Exit(3)
			return nil, fmt.Errorf("验证码已过期")
		}

		select {
		case <-time.After(pollInterval):
		case <-context.Background().Done():
			return nil, context.Background().Err()
		}
	}
}

// finishLogin verifies token, stores it, and prints welcome message.
func finishLogin(cmd *cobra.Command, p config.Profile, accountURL string, tokenResp *account.DeviceTokenResponse, quiet bool) error {
	// Verify token
	ac := account.New(accountURL, staticToken(tokenResp.AccessToken))
	u, err := ac.Whoami(context.Background())
	if err != nil {
		os.Exit(3)
		return fmt.Errorf("token 验证失败: %w", err)
	}

	// Store token
	st := credential.NewOAuthToken(
		tokenResp.AccessToken, tokenResp.RefreshToken,
		tokenResp.ExpiresIn, tokenResp.RefreshExpiresIn,
	)
	if err := storeToken(p, st); err != nil {
		return err
	}

	if isTerminal() {
		fmt.Fprintln(cmd.ErrOrStderr()) // clear spinner line
	}
	fmt.Fprintf(cmd.ErrOrStderr(), "✓ 登录成功！欢迎，%s", u.Nickname)
	if u.Email != "" {
		fmt.Fprintf(cmd.ErrOrStderr(), " (%s)", u.Email)
	}
	fmt.Fprintln(cmd.ErrOrStderr())
	fmt.Fprintln(cmd.ErrOrStderr(), "  凭证已保存到系统密钥链")
	return nil
}

// resolveAccountURL gets the account endpoint, auto-creating default profile if needed.
func resolveAccountURL() (config.Profile, string, error) {
	p, err := cliauth.CurrentProfile()
	if err != nil {
		// No profile — auto-create default
		p = config.Profile{
			Name:          "default",
			CredentialRef: "vibeknow.default",
			Endpoints:     endpoints.CloudDefaults,
			Trust:         "user",
			IsProduction:  true,
		}
	}
	accountURL, err := endpoints.Resolve(p, "account")
	if err != nil {
		return p, "", err
	}
	return p, accountURL, nil
}

// storeToken saves the token to keychain and ensures the profile exists.
func storeToken(p config.Profile, st credential.StoredToken) error {
	// Ensure profile exists
	f, err := config.LoadProfiles()
	if err != nil {
		return err
	}
	found := false
	for _, ep := range f.Profiles {
		if ep.Name == p.Name {
			found = true
			break
		}
	}
	if !found {
		if err := config.AddProfile(p); err != nil {
			return fmt.Errorf("创建默认 profile 失败: %w", err)
		}
		_ = config.UseProfile(p.Name)
	}

	kc, err := keychain.OpenFor("vibeknow")
	if err != nil {
		return fmt.Errorf("无法打开密钥链: %w", err)
	}
	return kc.Set(p.CredentialRef, st.Marshal())
}

func isTerminal() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

func openBrowser(url string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", url).Start()
	case "linux":
		return exec.Command("xdg-open", url).Start()
	case "windows":
		return exec.Command("cmd", "/c", "start", url).Start()
	default:
		return fmt.Errorf("unsupported platform")
	}
}
```

- [ ] **Step 2: Add golang.org/x/term dependency**

Run: `cd /Users/nullkey/laoshen/vibeknow-cli && go get golang.org/x/term`

- [ ] **Step 3: Register loginCmd in auth.go**

Modify `cmd/auth/auth.go`:

```go
func init() {
	Cmd.AddCommand(loginCmd)
	Cmd.AddCommand(whoamiCmd)
	Cmd.AddCommand(statusCmd)
	Cmd.AddCommand(logoutCmd)
}
```

- [ ] **Step 4: Build to verify compilation**

Run: `cd /Users/nullkey/laoshen/vibeknow-cli && go build ./...`
Expected: Success, no errors

- [ ] **Step 5: Verify help output**

Run: `cd /Users/nullkey/laoshen/vibeknow-cli && go run . auth login --help`
Expected: Shows login command with --with-token, --no-wait, --device-code flags

- [ ] **Step 6: Commit**

```bash
cd /Users/nullkey/laoshen/vibeknow-cli && git add cmd/auth/login.go cmd/auth/auth.go go.mod go.sum && git commit -m "feat(auth): add login command with Device Code Flow, PAT, and two-phase Agent mode"
```

---

### Task 8: Auth Status Enhancement

**Files:**
- Modify: `cmd/auth/status.go`

- [ ] **Step 1: Rewrite status.go with enhanced output**

Replace the full `RunE` function in `cmd/auth/status.go`:

```go
// cmd/auth/status.go
package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/vibeknow/cli/client/account"
	"github.com/vibeknow/cli/internal/cliauth"
	"github.com/vibeknow/cli/internal/config"
	"github.com/vibeknow/cli/internal/credential"
	"github.com/vibeknow/cli/internal/endpoints"
	"github.com/vibeknow/cli/internal/keychain"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "show authentication state and token details",
	RunE: func(cmd *cobra.Command, args []string) error {
		f, err := config.LoadProfiles()
		if err != nil {
			return err
		}

		// Check env var first
		envTok := os.Getenv("VIBEKNOW_TOKEN")

		p, profileErr := cliauth.CurrentProfile()

		// Try to load stored token from keychain
		var st credential.StoredToken
		var tokenSource string
		if envTok != "" {
			st = credential.ParseStored(envTok)
			tokenSource = "env"
		} else if profileErr == nil && p.CredentialRef != "" {
			if kc, err := keychain.OpenFor("vibeknow"); err == nil {
				src := credential.KeychainSource{Keychain: kc, Entry: p.CredentialRef}
				if stored, err := src.GetStored(); err == nil {
					st = stored
					tokenSource = "keychain"
				}
			}
		}

		if st.AccessToken == "" {
			fmt.Println("未登录")
			fmt.Println("  运行 `vibeknow auth login` 或设置 VIBEKNOW_TOKEN 环境变量")
			return nil
		}

		// Try to get user info
		var user *account.User
		if profileErr == nil {
			if accountURL, err := endpoints.Resolve(p, "account"); err == nil {
				ac := account.New(accountURL, staticToken(st.AccessToken))
				user, _ = ac.Whoami(context.Background())
			}
		}

		// Determine auth method display name
		authMethod := "PAT"
		if st.TokenType == "oauth" {
			authMethod = "Device Code Flow"
		}

		// Format status
		status := st.Status()
		var statusDisplay string
		switch status {
		case credential.StatusValid:
			if !st.ExpiresAt.IsZero() {
				remaining := time.Until(st.ExpiresAt).Truncate(time.Minute)
				statusDisplay = fmt.Sprintf("有效 (%s后过期)", formatDuration(remaining))
			} else {
				statusDisplay = "有效"
			}
		case credential.StatusNeedsRefresh:
			statusDisplay = "需要刷新 (将自动刷新)"
		case credential.StatusExpired:
			statusDisplay = "已过期"
		}

		// Print
		if user != nil {
			fmt.Printf("✓ 已登录为 %s", user.Nickname)
			if user.Email != "" {
				fmt.Printf(" (%s)", user.Email)
			}
			fmt.Println()
		}
		fmt.Printf("  - 认证方式: %s\n", authMethod)
		fmt.Printf("  - Token 来源: %s", tokenSource)
		if tokenSource == "keychain" && p.CredentialRef != "" {
			fmt.Printf(" (%s)", p.CredentialRef)
		}
		fmt.Println()
		fmt.Printf("  - Token 状态: %s\n", statusDisplay)
		if profileErr == nil {
			fmt.Printf("  - Active profile: %s\n", p.Name)
		}

		return nil
	},
}

func formatDuration(d time.Duration) string {
	if d < 0 {
		return "0分"
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	if h > 0 {
		return fmt.Sprintf("%d小时%d分", h, m)
	}
	return fmt.Sprintf("%d分", m)
}

func orNone(s string) string {
	if s == "" {
		return "(none)"
	}
	return s
}
```

- [ ] **Step 2: Build to verify compilation**

Run: `cd /Users/nullkey/laoshen/vibeknow-cli && go build ./...`
Expected: Success

- [ ] **Step 3: Commit**

```bash
cd /Users/nullkey/laoshen/vibeknow-cli && git add cmd/auth/status.go && git commit -m "feat(auth): enhance status command with token expiry, type, and status display"
```

---

### Task 9: Integration Test

**Files:**
- Create: `tests/integration/login_flow_test.go`

- [ ] **Step 1: Write integration test with fake servers**

```go
// tests/integration/login_flow_test.go
package integration

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoginWithToken(t *testing.T) {
	// Fake account server
	accountSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/user/profile":
			json.NewEncoder(w).Encode(map[string]any{
				"code": 0,
				"data": map[string]any{
					"uid":      12345,
					"nickname": "TestUser",
					"email":    "test@example.com",
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer accountSrv.Close()

	// Setup temp config dir
	configDir := t.TempDir()

	// Create profile pointing to fake server
	profileYAML := `schema_version: "2"
current: "test"
profiles:
  - name: "test"
    credential_ref: "vibeknow.test"
    endpoints:
      account: "` + accountSrv.URL + `"
    trust: "dev"
    is_production: false
`
	os.WriteFile(filepath.Join(configDir, "profiles.yaml"), []byte(profileYAML), 0o600)

	// Build CLI
	binary := filepath.Join(t.TempDir(), "vibeknow")
	build := exec.Command("go", "build", "-o", binary, ".")
	build.Dir = filepath.Join("..", "..")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build failed: %s\n%s", err, out)
	}

	// Run login --with-token
	cmd := exec.Command(binary, "auth", "login", "--with-token")
	cmd.Env = append(os.Environ(),
		"VIBEKNOW_CONFIG_HOME="+configDir,
		"VIBEKNOW_TOKEN=", // clear env token
	)
	cmd.Stdin = strings.NewReader("fake_pat_token_123\n")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("login failed: %s\n%s", err, out)
	}
	if !strings.Contains(string(out), "TestUser") {
		t.Fatalf("expected welcome message with TestUser, got: %s", out)
	}
}

func TestLoginNoWait(t *testing.T) {
	// Fake account server with device code endpoint
	accountSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/auth/device/code":
			json.NewEncoder(w).Encode(map[string]any{
				"code": 0,
				"data": map[string]any{
					"device_code":      "dc_test_123",
					"user_code":        "TEST-CODE",
					"verification_uri": "https://vibeknow.com/device",
					"expires_in":       900,
					"interval":         5,
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer accountSrv.Close()

	configDir := t.TempDir()
	profileYAML := `schema_version: "2"
current: "test"
profiles:
  - name: "test"
    credential_ref: "vibeknow.test"
    endpoints:
      account: "` + accountSrv.URL + `"
    trust: "dev"
    is_production: false
`
	os.WriteFile(filepath.Join(configDir, "profiles.yaml"), []byte(profileYAML), 0o600)

	binary := filepath.Join(t.TempDir(), "vibeknow")
	build := exec.Command("go", "build", "-o", binary, ".")
	build.Dir = filepath.Join("..", "..")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build failed: %s\n%s", err, out)
	}

	cmd := exec.Command(binary, "auth", "login", "--no-wait")
	cmd.Env = append(os.Environ(),
		"VIBEKNOW_CONFIG_HOME="+configDir,
		"VIBEKNOW_TOKEN=",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("login --no-wait failed: %s\n%s", err, out)
	}

	var result map[string]any
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatalf("expected JSON output, got: %s", out)
	}
	if result["device_code"] != "dc_test_123" {
		t.Fatalf("device_code = %v", result["device_code"])
	}
	if result["user_code"] != "TEST-CODE" {
		t.Fatalf("user_code = %v", result["user_code"])
	}
}
```

- [ ] **Step 2: Run integration tests**

Run: `cd /Users/nullkey/laoshen/vibeknow-cli && go test ./tests/integration/ -run "TestLoginWithToken|TestLoginNoWait" -v -count=1`
Expected: All PASS

- [ ] **Step 3: Run full test suite**

Run: `cd /Users/nullkey/laoshen/vibeknow-cli && make test`
Expected: All PASS

- [ ] **Step 4: Commit**

```bash
cd /Users/nullkey/laoshen/vibeknow-cli && git add tests/integration/login_flow_test.go && git commit -m "test: add integration tests for auth login --with-token and --no-wait"
```

---

## Self-Review Checklist

### Spec Coverage
| Spec Section | Task |
|-------------|------|
| §3 Token Lifecycle | Task 1 (StoredToken) |
| §4.2 Interactive Login | Task 7 (loginInteractive) |
| §4.3 PAT Mode | Task 7 (loginWithToken) |
| §4.4 Two-Phase Agent | Task 7 (loginNoWait + loginResumeDeviceCode) |
| §4.5 Edge Cases | Task 7 (mutual exclusion, TTY check, env var warning) |
| §5.1 Refresh Architecture | Task 3 (RefreshRetryMiddleware) + Task 5 (OAuthTokenProvider) |
| §5.2 Concurrent Refresh | Task 4 (RefreshLock) |
| §6.1-6.3 Backend Contracts | Task 2 (device.go + refresh.go) |
| §7.1-7.2 Module Changes | Tasks 1-8 |
| §8 Default Profile | Task 7 (resolveAccountURL + storeToken) |
| §9 Status Enhancement | Task 8 |

### Placeholder Scan
No TBD, TODO, or vague steps found. All code blocks are complete.

### Type Consistency
- `StoredToken` — defined in Task 1, used consistently in Tasks 5, 7, 8
- `DeviceCodeResponse` / `DeviceTokenResponse` — defined in Task 2, used in Task 7
- `RefreshableTokenProvider` — defined in Task 3, implemented in Task 5, wired in Task 6
- `PollError` — defined in Task 2, handled in Task 7
- `OAuthTokenProvider` — defined in Task 5, constructed in Task 6
