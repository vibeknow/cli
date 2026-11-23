package cliauth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/vibeknow/cli/internal/clerr"
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

// TestOAuthTokenProvider_PlainUnauthorizedPurgesKeychain covers the 401 the
// account service returns when it simply will not accept this refresh token
// — no named business code, just aether's 40101.
//
// It is the shape a refresh token that has run out its window produces, and
// the shape every stored credential produces the day the signing secret is
// rotated. Treated as transient it leaves a permanently dead token in the
// keychain and every later command fails the same opaque way; treated as
// dead, the user is told to log in again, which is the only thing that
// actually works.
func TestOAuthTokenProvider_PlainUnauthorizedPurgesKeychain(t *testing.T) {
	srv := newAccountStub(t, 40101, http.StatusUnauthorized, nil)
	defer srv.Close()

	kc := &fakeKeychain{data: map[string][]byte{"entry": expiredOAuthToken().Marshal()}}
	provider := NewOAuthTokenProvider(
		credential.KeychainSource{Keychain: kc, Entry: "entry"},
		srv.URL,
		t.TempDir(),
	)

	_, err := provider.Token(context.Background())
	if err == nil {
		t.Fatal("Token() should fail when the refresh endpoint returns 401")
	}
	if !errs.HasCode(err, CodeSessionExpired) {
		t.Errorf("want code %q, got error: %v", CodeSessionExpired, err)
	}
	if _, ok := kc.data["entry"]; ok {
		t.Error("keychain entry should be purged after a 401 on refresh")
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

// TestOAuthTokenProvider_NonRotatingRefreshKeepsCredential covers a refresh
// response that returns a new access token and nothing else.
//
// RFC 6749 §6 makes issuing a new refresh token optional, so a server that
// does not rotate replies with only access_token/expires_in. Storing that
// response verbatim overwrote the refresh token with "" and the refresh
// expiry with a time in the past — the next refresh had nothing to send and
// the credential read as expired, so a working session was destroyed on its
// first refresh and the user had to log in again every access-token lifetime.
//
// What the response omits, it does not change.
func TestOAuthTokenProvider_NonRotatingRefreshKeepsCredential(t *testing.T) {
	srv := newAccountStub(t, 0, http.StatusOK, map[string]any{
		"access_token": "access-new",
		"expires_in":   3600,
		// no refresh_token, no refresh_expires_in
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
		t.Errorf("Token() = %q, want access-new", tok)
	}

	fresh, _ := credential.KeychainSource{Keychain: kc, Entry: "entry"}.GetStored()
	if fresh.RefreshToken != "refresh-xyz" {
		t.Errorf("refresh token = %q, want the previous refresh-xyz to be kept", fresh.RefreshToken)
	}
	if fresh.Status() == credential.StatusExpired {
		t.Errorf("credential reads as expired right after a successful refresh "+
			"(refresh_expires_at=%v)", fresh.RefreshExpiresAt)
	}
}

// failingSetKeychain refuses writes to the credential entry but accepts
// everything else, the probe included. It models the store failing between
// the probe and the real write — the disk filling up, another tool replacing
// the entry — which is rare but not hypothetical, and which the probe cannot
// rule out.
type failingSetKeychain struct {
	fakeKeychain
	deleted bool
}

func (f *failingSetKeychain) Set(key string, data []byte) error {
	if key == "entry" {
		return errors.New("keychain is locked")
	}
	return f.fakeKeychain.Set(key, data)
}

func (f *failingSetKeychain) Delete(key string) error {
	if key == "entry" {
		f.deleted = true
	}
	return f.fakeKeychain.Delete(key)
}

// rotatingStub answers every refresh with a new pair and records which
// refresh tokens were presented.
func rotatingStub(t *testing.T, presented *[]string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			RefreshToken string `json:"refresh_token"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		*presented = append(*presented, body.RefreshToken)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 0, "message": "ok",
			"data": map[string]any{
				"access_token":       "at_rotated",
				"refresh_token":      "rt_rotated",
				"expires_in":         7200,
				"refresh_expires_in": 7776000,
			},
		})
	}))
}

// TestDoRefresh_UnwritableKeychain_DropsTheSupersededCredential covers a
// write that fails after the probe succeeded.
//
// The server has accepted the refresh and spent the presented token, and the
// CLI cannot persist what it got back. The keychain is left holding a token
// that has already been spent; the next process would present it, and a
// server that rotates treats a second presentation as a stolen credential and
// revokes the entire session.
//
// The stale copy must therefore be removed. It could never have worked again;
// what changes is that the next process finds no credential and asks for a
// login, instead of tripping the alarm on its way to the same outcome.
func TestDoRefresh_UnwritableKeychain_DropsTheSupersededCredential(t *testing.T) {
	var presented []string
	srv := rotatingStub(t, &presented)
	defer srv.Close()

	kc := &failingSetKeychain{fakeKeychain: fakeKeychain{data: map[string][]byte{
		"entry": expiredOAuthToken().Marshal(),
	}}}
	p := NewOAuthTokenProvider(
		credential.KeychainSource{Keychain: kc, Entry: "entry"},
		srv.URL, t.TempDir(),
	)

	tok, err := p.Token(context.Background())
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	// The command in flight still finishes: the new pair is held in memory.
	if tok != "at_rotated" {
		t.Errorf("token = %q, want the refreshed one — the in-flight command should not be punished "+
			"for a keychain that cannot be written", tok)
	}

	if !kc.deleted {
		t.Error("the superseded credential was not removed")
	}
	if _, ok := kc.data["entry"]; ok {
		t.Error("a spent refresh token is still in the keychain; the next process would replay it " +
			"and the server would revoke the whole session")
	}
}

// entryLockedKeychain refuses writes *and* deletes of the credential entry,
// while the probe still goes through: a store that failed after the probe in
// a way that also blocks the cleanup. That is the combination where the stale
// copy survives in the keychain.
type entryLockedKeychain struct{ fakeKeychain }

func (l *entryLockedKeychain) Set(key string, data []byte) error {
	if key == "entry" {
		return errors.New("keychain is locked")
	}
	return l.fakeKeychain.Set(key, data)
}

func (l *entryLockedKeychain) Delete(key string) error {
	if key == "entry" {
		return errors.New("keychain is locked")
	}
	return l.fakeKeychain.Delete(key)
}

// TestLockedKeychain_DoesNotReplayTheSpentRefreshToken pins the in-process
// half of that case: once the refresh has gone through and could neither be
// written back nor cleaned up, everything after it in the same process must
// use the pair held in memory. Reading the keychain instead returns the token
// the server just spent, and presenting it again revokes the whole session.
func TestLockedKeychain_DoesNotReplayTheSpentRefreshToken(t *testing.T) {
	var presented []string
	srv := rotatingStub(t, &presented)
	defer srv.Close()

	kc := &entryLockedKeychain{fakeKeychain: fakeKeychain{data: map[string][]byte{
		"entry": expiredOAuthToken().Marshal(),
	}}}
	p := NewOAuthTokenProvider(
		credential.KeychainSource{Keychain: kc, Entry: "entry"},
		srv.URL, t.TempDir(),
	)

	first, err := p.Token(context.Background())
	if err != nil {
		t.Fatalf("first Token: %v", err)
	}
	if first != "at_rotated" {
		t.Fatalf("first token = %q, want at_rotated", first)
	}

	second, err := p.Token(context.Background())
	if err != nil {
		t.Fatalf("second Token: %v", err)
	}
	if second != "at_rotated" {
		t.Errorf("second token = %q, want at_rotated — the in-memory copy should win "+
			"over the superseded keychain entry", second)
	}
	if len(presented) != 1 {
		t.Errorf("refresh endpoint called %d times %v, want 1: the spent refresh token was "+
			"presented again, which revokes the whole session under rotation",
			len(presented), presented)
	}
}

// readOnlyKeychain can be read but refuses every write and delete — what a
// sandbox does to a CLI whose credential store sits outside the paths it may
// write. It is the common way to end up unable to keep a refreshed pair, and
// the one the probe exists for.
type readOnlyKeychain struct{ fakeKeychain }

func (r *readOnlyKeychain) Set(string, []byte) error { return errors.New("operation not permitted") }
func (r *readOnlyKeychain) Delete(string) error      { return errors.New("operation not permitted") }

// TestReadOnlyStore_DoesNotSpendTheRefreshToken is the case behind sessions
// revoked minutes after a sandboxed agent ran a command.
//
// Refreshing in a read-only store gave the command in flight a new pair that
// died with the process, while every later command — in the sandbox, or the
// user's own terminal — read back the token the server had spent. The first of
// them to present it got the whole session revoked.
//
// So the refresh is not attempted: the token is never spent and the session
// survives for whichever process can store a new pair. With the access token
// already expired there is nothing worth sending, so the command fails with an
// error that says why, and — unlike a dead session — keeps the credential.
func TestReadOnlyStore_DoesNotSpendTheRefreshToken(t *testing.T) {
	var presented []string
	srv := rotatingStub(t, &presented)
	defer srv.Close()

	stored := expiredOAuthToken().Marshal()
	kc := &readOnlyKeychain{fakeKeychain: fakeKeychain{data: map[string][]byte{"entry": stored}}}
	p := NewOAuthTokenProvider(
		credential.KeychainSource{Keychain: kc, Entry: "entry"},
		srv.URL, t.TempDir(),
	)

	for i := 0; i < 2; i++ {
		_, err := p.Token(context.Background())
		if !isStoreReadOnly(err) {
			t.Fatalf("Token #%d err = %v, want the read-only store error", i+1, err)
		}
		var ce *clerr.Error
		if !errors.As(err, &ce) || ce.Code != clerr.ExitAuth || ce.Hint == "" {
			t.Errorf("Token #%d err = %#v, want an auth error (exit 3) with a hint", i+1, err)
		}
	}
	if len(presented) != 0 {
		t.Errorf("refresh endpoint called %d times %v, want 0: the token was spent with nowhere "+
			"to keep its replacement", len(presented), presented)
	}
	if got := kc.data["entry"]; string(got) != string(stored) {
		t.Error("the stored credential changed; a read-only store should leave the session as it was")
	}

	// ForceRefresh — what a 401 triggers — must hold back the same way.
	if _, err := p.ForceRefresh(context.Background()); !isStoreReadOnly(err) {
		t.Errorf("ForceRefresh err = %v, want the read-only store error", err)
	}
	if len(presented) != 0 {
		t.Errorf("ForceRefresh presented the refresh token %v", presented)
	}
}

// TestReadOnlyStore_UsesTheAccessTokenItStillHas: inside the refresh window
// the access token has not expired yet, so the command goes ahead with it
// rather than failing — still without spending the refresh token.
func TestReadOnlyStore_UsesTheAccessTokenItStillHas(t *testing.T) {
	var presented []string
	srv := rotatingStub(t, &presented)
	defer srv.Close()

	st := credential.NewOAuthToken("access-still-good", "refresh-xyz", 120, 86400)
	kc := &readOnlyKeychain{fakeKeychain: fakeKeychain{data: map[string][]byte{"entry": st.Marshal()}}}
	p := NewOAuthTokenProvider(
		credential.KeychainSource{Keychain: kc, Entry: "entry"},
		srv.URL, t.TempDir(),
	)

	tok, err := p.Token(context.Background())
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if tok != "access-still-good" {
		t.Errorf("token = %q, want the stored access token", tok)
	}
	if len(presented) != 0 {
		t.Errorf("refresh endpoint called %v, want no call", presented)
	}
}

// TestRefresh_ProbeLeavesNothingBehind: in a writable store the probe entry is
// written and removed again, and the refresh proceeds as before.
func TestRefresh_ProbeLeavesNothingBehind(t *testing.T) {
	var presented []string
	srv := rotatingStub(t, &presented)
	defer srv.Close()

	kc := &fakeKeychain{data: map[string][]byte{"entry": expiredOAuthToken().Marshal()}}
	p := NewOAuthTokenProvider(
		credential.KeychainSource{Keychain: kc, Entry: "entry"},
		srv.URL, t.TempDir(),
	)

	if tok, err := p.Token(context.Background()); err != nil || tok != "at_rotated" {
		t.Fatalf("Token = %q, %v; want at_rotated", tok, err)
	}
	if len(kc.data) != 1 {
		keys := make([]string, 0, len(kc.data))
		for k := range kc.data {
			keys = append(keys, k)
		}
		t.Errorf("store holds %v, want only the credential entry", keys)
	}
}
