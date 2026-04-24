package cliauth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/vibeknow/cli/internal/credential"
	"github.com/vibeknow/cli/internal/errs"
	"github.com/vibeknow/cli/internal/httpclient"
)

type fakeKeychain struct{ data map[string][]byte }

func (f *fakeKeychain) Get(key string) ([]byte, error) {
	v, ok := f.data[key]
	if !ok {
		return nil, credential.ErrNotFound
	}
	return v, nil
}

func (f *fakeKeychain) Set(key string, data []byte) error {
	f.data[key] = data
	return nil
}

func (f *fakeKeychain) Delete(key string) error {
	delete(f.data, key)
	return nil
}

func TestOAuthTokenProvider_Valid(t *testing.T) {
	// Create a valid oauth token (expires in 1 hour, refresh in 24 hours).
	st := credential.NewOAuthToken("access-abc", "refresh-xyz", 3600, 86400)
	kc := &fakeKeychain{data: map[string][]byte{
		"test-entry": st.Marshal(),
	}}
	src := credential.KeychainSource{Keychain: kc, Entry: "test-entry"}
	provider := NewOAuthTokenProvider(src, "https://account.example.com", t.TempDir())

	ctx := context.Background()

	tok, err := provider.Token(ctx)
	if err != nil {
		t.Fatalf("Token() error: %v", err)
	}
	if tok != "access-abc" {
		t.Errorf("Token() = %q, want %q", tok, "access-abc")
	}

	tt := provider.TokenType()
	if tt != "oauth" {
		t.Errorf("TokenType() = %q, want %q", tt, "oauth")
	}
}

func TestOAuthTokenProvider_PAT(t *testing.T) {
	st := credential.NewPATToken("pat-my-token")
	kc := &fakeKeychain{data: map[string][]byte{
		"test-entry": st.Marshal(),
	}}
	src := credential.KeychainSource{Keychain: kc, Entry: "test-entry"}
	provider := NewOAuthTokenProvider(src, "https://account.example.com", t.TempDir())

	ctx := context.Background()

	tok, err := provider.Token(ctx)
	if err != nil {
		t.Fatalf("Token() error: %v", err)
	}
	if tok != "pat-my-token" {
		t.Errorf("Token() = %q, want %q", tok, "pat-my-token")
	}

	tt := provider.TokenType()
	if tt != "pat" {
		t.Errorf("TokenType() = %q, want %q", tt, "pat")
	}

	// ForceRefresh should fail for PAT.
	_, err = provider.ForceRefresh(ctx)
	if err == nil {
		t.Error("ForceRefresh() should return error for PAT")
	}
}

// newAccountStub returns a test server that responds to POST
// /v1/auth/token/refresh with the given envelope code, HTTP status, and
// optional data payload. data==nil means no data field (error response).
func newAccountStub(t *testing.T, code int, httpStatus int, data map[string]any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/auth/token/refresh" {
			http.Error(w, "wrong path", http.StatusNotFound)
			return
		}
		body := map[string]any{
			"code":    code,
			"message": "test",
		}
		if data != nil {
			body["data"] = data
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(httpStatus)
		_ = json.NewEncoder(w).Encode(body)
	}))
}

// expiredOAuthToken returns a token whose access_token is past expiry but
// whose refresh_token still has plenty of validity left, so Token() will
// trigger doRefresh.
func expiredOAuthToken() credential.StoredToken {
	return credential.NewOAuthToken("access-stale", "refresh-xyz", -60, 86400)
}

func TestOAuthTokenProvider_RefreshSuccess(t *testing.T) {
	srv := newAccountStub(t, 0, http.StatusOK, map[string]any{
		"access_token":       "access-new",
		"refresh_token":      "refresh-new",
		"expires_in":         3600,
		"refresh_expires_in": 86400,
	})
	defer srv.Close()

	kc := &fakeKeychain{data: map[string][]byte{"entry": expiredOAuthToken().Marshal()}}
	provider := NewOAuthTokenProvider(
		credential.KeychainSource{Keychain: kc, Entry: "entry"},
		srv.URL,
		t.TempDir(),
	)

	tok, err := provider.Token(context.Background())
	if err != nil {
		t.Fatalf("Token() error: %v", err)
	}
	if tok != "access-new" {
		t.Errorf("Token() = %q, want %q", tok, "access-new")
	}

	// Rotation: keychain should now hold the new refresh token.
	fresh, _ := credential.KeychainSource{Keychain: kc, Entry: "entry"}.GetStored()
	if fresh.AccessToken != "access-new" || fresh.RefreshToken != "refresh-new" {
		t.Errorf("keychain not rotated: got access=%q refresh=%q", fresh.AccessToken, fresh.RefreshToken)
	}
}

func TestOAuthTokenProvider_SessionDeadPurgesKeychain(t *testing.T) {
	// Backend returns 110008 session_replaced (HTTP 401) — device session
	// was invalidated (account soft-deleted, or legacy token's tv bumped
	// by a Web login).
	srv := newAccountStub(t, 110008, http.StatusUnauthorized, nil)
	defer srv.Close()

	kc := &fakeKeychain{data: map[string][]byte{"entry": expiredOAuthToken().Marshal()}}
	provider := NewOAuthTokenProvider(
		credential.KeychainSource{Keychain: kc, Entry: "entry"},
		srv.URL,
		t.TempDir(),
	)

	_, err := provider.Token(context.Background())
	if err == nil {
		t.Fatal("Token() should fail when backend returns session_replaced")
	}
	if !errs.HasCode(err, CodeSessionExpired) {
		t.Errorf("want code %q, got error: %v", CodeSessionExpired, err)
	}
	if _, ok := kc.data["entry"]; ok {
		t.Error("keychain entry should be purged after session_replaced")
	}
	// The underlying backend code should be preserved in Details so bug
	// reports retain the root cause even though the user-facing Message
	// is the short re-login prompt.
	var out *errs.Object
	if !errors.As(err, &out) {
		t.Fatalf("expected *errs.Object, got %T", err)
	}
	if got := out.Details["cause_code"]; got != httpclient.CodeSessionReplaced {
		t.Errorf("Details[cause_code] = %v, want %q", got, httpclient.CodeSessionReplaced)
	}
}

func TestOAuthTokenProvider_SessionDeadOnForceRefresh(t *testing.T) {
	// Same scenario but via the 401-retry path (ForceRefresh), which is
	// the middleware-triggered variant.
	srv := newAccountStub(t, 110004, http.StatusUnauthorized, nil)
	defer srv.Close()

	kc := &fakeKeychain{data: map[string][]byte{"entry": expiredOAuthToken().Marshal()}}
	provider := NewOAuthTokenProvider(
		credential.KeychainSource{Keychain: kc, Entry: "entry"},
		srv.URL,
		t.TempDir(),
	)

	_, err := provider.ForceRefresh(context.Background())
	if err == nil {
		t.Fatal("ForceRefresh() should fail when backend returns account_disabled")
	}
	if !errs.HasCode(err, CodeSessionExpired) {
		t.Errorf("want code %q, got error: %v", CodeSessionExpired, err)
	}
	if _, ok := kc.data["entry"]; ok {
		t.Error("keychain entry should be purged after account_disabled")
	}
}

func TestOAuthTokenProvider_TransientRefreshFailFallsBack(t *testing.T) {
	// Transient 5xx: the access token may still be valid for a few more
	// seconds in the refresh window, so Token() returns the stale access
	// token and lets the HTTP layer try it. Keychain is NOT purged.
	srv := newAccountStub(t, 50001, http.StatusInternalServerError, nil)
	defer srv.Close()

	// Token within the 5-min refresh window (60s remaining on access).
	st := credential.NewOAuthToken("access-stale", "refresh-xyz", 60, 86400)
	kc := &fakeKeychain{data: map[string][]byte{"entry": st.Marshal()}}
	provider := NewOAuthTokenProvider(
		credential.KeychainSource{Keychain: kc, Entry: "entry"},
		srv.URL,
		t.TempDir(),
	)

	tok, err := provider.Token(context.Background())
	if err != nil {
		t.Fatalf("Token() should fall back to stale access_token on transient 5xx, got err: %v", err)
	}
	if tok != "access-stale" {
		t.Errorf("Token() = %q, want fallback %q", tok, "access-stale")
	}
	if _, ok := kc.data["entry"]; !ok {
		t.Error("keychain should NOT be purged on transient failure")
	}
}
