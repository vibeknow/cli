package integration

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// TestAuth_ServerKeyRotated_Exit3AndPurges pins what happens to a credential
// that is still valid by the client's own clock but that the server will no
// longer accept.
//
// This is not a hypothetical. go-account's signing secret was rotated in
// `feat(auth)!: upgrade go-atlas to v0.7.0 and rotate the production secret`
// (config/config.prod.yaml:42), and go-atlas rejects a token whose signature
// does not verify with a plain 401 (pillar/auth/jwt.go:parse →
// aerrors.CodeUnauthorized). Every CLI user holding a keychain credential
// minted under the old secret therefore meets this sequence the moment that
// deploy lands:
//
//   - the access token still has up to two hours on it locally, so the token
//     provider hands it over without refreshing
//   - the API answers 401
//   - RefreshRetryMiddleware forces a refresh
//   - the refresh token was signed with the same dead key, so that 401s too
//
// The contract being pinned: the run ends in exit 3 with a re-login
// instruction, and the dead credential is purged so the next command does not
// repeat the same two doomed round-trips. Exit 1 here would be a silent
// disaster — an agent reads exit 1 as "something broke, retry" and would
// retry a credential that can never work again instead of telling the user to
// reconnect their account.
func TestAuth_ServerKeyRotated_Exit3AndPurges(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	var mu sync.Mutex
	rotated := false
	var refreshAttempts int

	isRotated := func() bool {
		mu.Lock()
		defer mu.Unlock()
		return rotated
	}

	// unauthorized answers the way go-atlas does for a signature it cannot
	// verify: HTTP 401 with the aether envelope carrying code 401.
	// aerrors.CodeUnauthorized is literally 401, not a 40100-range code
	// (go-atlas/aether/errors/errors.go:21), so this also exercises the CLI's
	// fallback from envelope code to HTTP status in mapEnvelopeCode.
	unauthorized := func(w http.ResponseWriter) {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":    401,
			"message": "token is invalid",
		})
	}

	account := http.NewServeMux()
	account.HandleFunc("/v1/auth/device/code", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 0, "message": "ok",
			"data": map[string]any{
				"device_code":      "dc_rotate",
				"user_code":        "ROTA-TED1",
				"verification_uri": "https://example.test/activate",
				"expires_in":       900,
				"interval":         1,
			},
		})
	})
	account.HandleFunc("/v1/auth/device/token", func(w http.ResponseWriter, r *http.Request) {
		// Authorized straight away: this test is about what happens *after*
		// a good login, not about the device flow itself.
		//
		// The lifetimes are the ones go-account now issues for a device
		// grant: 2h access (auth.access_expire) and 90d refresh (the
		// "device" entry under auth.clients).
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 0, "message": "ok",
			"data": map[string]any{
				"access_token":       "at_minted_under_old_key",
				"refresh_token":      "rt_minted_under_old_key",
				"token_type":         "bearer",
				"expires_in":         7200,
				"refresh_expires_in": 7776000,
			},
		})
	})
	account.HandleFunc("/v1/auth/token/refresh", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		refreshAttempts++
		mu.Unlock()
		if isRotated() {
			unauthorized(w)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 0, "message": "ok",
			"data": map[string]any{
				"access_token":       "at_rotated_in",
				"refresh_token":      "rt_rotated_in",
				"expires_in":         7200,
				"refresh_expires_in": 7776000,
			},
		})
	})
	account.HandleFunc("/v1/user/profile", func(w http.ResponseWriter, r *http.Request) {
		if isRotated() {
			unauthorized(w)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 0, "message": "ok",
			"data": map[string]any{"uid": 42, "nickname": "rotation-user"},
		})
	})
	accountSrv := httptest.NewServer(account)
	defer accountSrv.Close()

	figlens := http.NewServeMux()
	figlens.HandleFunc("/v1/works/page", func(w http.ResponseWriter, r *http.Request) {
		if isRotated() {
			unauthorized(w)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 0, "message": "ok",
			"data": map[string]any{"list": []any{}, "total": 0},
		})
	})
	figlensSrv := httptest.NewServer(figlens)
	defer figlensSrv.Close()

	bin := build(t)
	configHome := buildProfile(t, map[string]string{
		"account": accountSrv.URL,
		"figlens": figlensSrv.URL,
	})
	noToken := []string{"VIBEKNOW_TOKEN="}

	// Always clean up: this test writes a real keychain entry.
	t.Cleanup(func() {
		_, _, _ = runCmdEnv(t, bin, configHome, noToken, "auth", "logout")
	})

	// --- log in for real, so the credential under test is one the CLI wrote ---
	// --headless is the connector's own path: it prints the URL and polls
	// until authorized, which the stub above grants on the first poll.
	if out, stderr, code := runCmdEnv(t, bin, configHome, noToken, "auth", "login", "--headless"); code != 0 {
		t.Fatalf("setup login: exit %d\nstdout: %s\nstderr: %s", code, out, stderr)
	}
	out, stderr, code := runCmdEnv(t, bin, configHome, noToken, "auth", "status", "--output", "json")
	if code != 0 {
		t.Fatalf("auth status before rotation: exit %d\nstdout: %s\nstderr: %s", code, out, stderr)
	}
	var before map[string]any
	if err := json.Unmarshal([]byte(out), &before); err != nil {
		t.Fatalf("decode pre-rotation status %q: %v", out, err)
	}
	if before["authenticated"] != true {
		t.Fatalf("setup failed: not authenticated before rotation: %+v", before)
	}

	// --- the deploy lands: every token signed with the old key is now junk ---
	mu.Lock()
	rotated = true
	refreshAttempts = 0
	mu.Unlock()

	out, stderr, code = runCmdEnv(t, bin, configHome, noToken, "video", "list", "--output", "json")

	if code != 3 {
		t.Errorf("exit = %d, want 3 (re-authenticate)\nstdout: %s\nstderr: %s", code, out, stderr)
	}
	combined := out + stderr
	if !strings.Contains(combined, "auth login") {
		t.Errorf("no re-login instruction in output; an agent has nothing to act on\nstdout: %s\nstderr: %s", out, stderr)
	}
	if strings.Contains(combined, "network_error") {
		t.Errorf("reported network_error for a rejected credential\nstdout: %s\nstderr: %s", out, stderr)
	}

	// The CLI must actually have tried to refresh: handing back the 401
	// without attempting one would also exit 3, but for the wrong reason,
	// and would break the ordinary "access token merely expired" case.
	mu.Lock()
	attempts := refreshAttempts
	mu.Unlock()
	if attempts == 0 {
		t.Error("no refresh attempted on 401; RefreshRetryMiddleware did not fire")
	}

	// --- the dead credential must be gone, not left to fail again ---
	// Exit 3, not 0: with no usable credential left, the exit code has to
	// agree with the payload — a connector host starts its login from this.
	out, stderr, code = runCmdEnv(t, bin, configHome, noToken, "auth", "status", "--output", "json")
	if code != 3 {
		t.Fatalf("auth status after rotation: exit %d, want 3\nstdout: %s\nstderr: %s", code, out, stderr)
	}
	var after map[string]any
	if err := json.Unmarshal([]byte(out), &after); err != nil {
		t.Fatalf("decode post-rotation status %q: %v", out, err)
	}
	if after["authenticated"] == true {
		t.Errorf("credential survived a permanent rejection; every later command repeats the same two doomed requests: %+v", after)
	}
}

// TestAuthStatus_ServerKeyRotated_ReportsDisconnected covers the same rotation
// through the only command a connector host runs on its own initiative.
//
// The WorkBuddy connector spec drives reconnection off `status` alone (§11.7:
// 自动重连只执行 status), polling it every three seconds. So `status` is not a
// convenience view here — it is the single input to "is this connection
// live?". A status that answers from the local clock alone reports a healthy
// connection for as long as the stored access token has time left on it,
// while every real request fails. The user sees a connector that says
// "connected" and does nothing, with no prompt to reconnect.
//
// Spec §12 anticipates this and recommends the opposite (方案 B: status 命令
// 执行时从服务端拉取 token). status already makes the server call — it just has
// to stop discarding the answer when the answer is "this credential is dead".
func TestAuthStatus_ServerKeyRotated_ReportsDisconnected(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	var mu sync.Mutex
	rotated := false
	isRotated := func() bool {
		mu.Lock()
		defer mu.Unlock()
		return rotated
	}

	unauthorized := func(w http.ResponseWriter) {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 401, "message": "token is invalid"})
	}

	account := http.NewServeMux()
	account.HandleFunc("/v1/auth/device/code", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 0, "message": "ok",
			"data": map[string]any{
				"device_code": "dc_statusrot", "user_code": "ROTA-TED2",
				"verification_uri": "https://example.test/activate",
				"expires_in":       900, "interval": 1,
			},
		})
	})
	account.HandleFunc("/v1/auth/device/token", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 0, "message": "ok",
			"data": map[string]any{
				"access_token": "at_statusrot", "refresh_token": "rt_statusrot",
				"token_type": "bearer",
				// A full two hours left locally, so nothing about the stored
				// credential looks wrong to the client.
				"expires_in": 7200, "refresh_expires_in": 7776000,
			},
		})
	})
	account.HandleFunc("/v1/auth/token/refresh", func(w http.ResponseWriter, r *http.Request) {
		unauthorized(w) // the refresh token was signed with the dead key too
	})
	account.HandleFunc("/v1/user/profile", func(w http.ResponseWriter, r *http.Request) {
		if isRotated() {
			unauthorized(w)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 0, "message": "ok",
			"data": map[string]any{"uid": 43, "nickname": "status-rotation-user"},
		})
	})
	srv := httptest.NewServer(account)
	defer srv.Close()

	bin := build(t)
	configHome := buildProfile(t, map[string]string{"account": srv.URL})
	noToken := []string{"VIBEKNOW_TOKEN="}
	t.Cleanup(func() {
		_, _, _ = runCmdEnv(t, bin, configHome, noToken, "auth", "logout")
	})

	if out, stderr, code := runCmdEnv(t, bin, configHome, noToken, "auth", "login", "--headless"); code != 0 {
		t.Fatalf("setup login: exit %d\nstdout: %s\nstderr: %s", code, out, stderr)
	}

	mu.Lock()
	rotated = true
	mu.Unlock()

	// No other command runs in between: this is the connector's reconnect
	// poll, on its own, against a credential the server will never accept.
	// Exit 3 is the report, not a crash: status still printed a complete
	// payload on stdout and nothing on stderr. The code says "not connected"
	// so the host's connect sequence — status, then auth if non-zero —
	// actually offers the user a way back in.
	out, stderr, code := runCmdEnv(t, bin, configHome, noToken, "auth", "status", "--output", "json")
	if code != 3 {
		t.Fatalf("auth status: exit %d, want 3\nstdout: %s\nstderr: %s", code, out, stderr)
	}
	if stderr != "" {
		t.Errorf("stderr should stay empty so a host parsing merged output still sees clean JSON, got %q", stderr)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("decode status %q: %v", out, err)
	}
	if got["authenticated"] == true {
		t.Errorf("authenticated = true for a credential the server rejects; "+
			"the connector will show a live connection and every command will fail: %+v", got)
	}
	if hint, _ := got["hint"].(string); !strings.Contains(hint, "auth login") {
		t.Errorf("no re-login hint for a rejected credential: %+v", got)
	}
}

// TestAuthStatus_ServerUnreachable_KeepsLocalVerdict is the other half of the
// contract above, and the regression this fix could easily have introduced.
//
// Checking the credential against the server is only correct if "the server
// did not answer" is kept distinct from "the server said no". Collapsing the
// two would make `auth status` report a disconnected connector every time the
// network hiccuped — and since the connector polls status every three seconds,
// a few seconds of bad wifi would tear down a session that was never in any
// trouble, prompting the user to re-authorize for nothing.
func TestAuthStatus_ServerUnreachable_KeepsLocalVerdict(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	var mu sync.Mutex
	down := false
	isDown := func() bool {
		mu.Lock()
		defer mu.Unlock()
		return down
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/auth/device/code", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 0, "message": "ok",
			"data": map[string]any{
				"device_code": "dc_down", "user_code": "DOWN-0001",
				"verification_uri": "https://example.test/activate",
				"expires_in":       900, "interval": 1,
			},
		})
	})
	mux.HandleFunc("/v1/auth/device/token", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 0, "message": "ok",
			"data": map[string]any{
				"access_token": "at_down", "refresh_token": "rt_down",
				"token_type": "bearer",
				"expires_in": 7200, "refresh_expires_in": 7776000,
			},
		})
	})
	mux.HandleFunc("/v1/user/profile", func(w http.ResponseWriter, r *http.Request) {
		if isDown() {
			// A 5xx is the server failing, not the credential failing.
			http.Error(w, "bad gateway", http.StatusBadGateway)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 0, "message": "ok",
			"data": map[string]any{"uid": 44, "nickname": "outage-user"},
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	bin := build(t)
	configHome := buildProfile(t, map[string]string{"account": srv.URL})
	noToken := []string{"VIBEKNOW_TOKEN="}
	t.Cleanup(func() {
		_, _, _ = runCmdEnv(t, bin, configHome, noToken, "auth", "logout")
	})

	if out, stderr, code := runCmdEnv(t, bin, configHome, noToken, "auth", "login", "--headless"); code != 0 {
		t.Fatalf("setup login: exit %d\nstdout: %s\nstderr: %s", code, out, stderr)
	}

	mu.Lock()
	down = true
	mu.Unlock()

	out, stderr, code := runCmdEnv(t, bin, configHome, noToken, "auth", "status", "--output", "json")
	if code != 0 {
		t.Fatalf("auth status during outage: exit %d\nstdout: %s\nstderr: %s", code, out, stderr)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("decode status %q: %v", out, err)
	}
	if got["authenticated"] != true {
		t.Errorf("authenticated = %v during a server outage, want true — "+
			"a credential that was never rejected must survive an unreachable server: %+v",
			got["authenticated"], got)
	}

	// And the credential must still be there once the server comes back.
	mu.Lock()
	down = false
	mu.Unlock()
	out, _, _ = runCmdEnv(t, bin, configHome, noToken, "auth", "status", "--output", "json")
	var recovered map[string]any
	if err := json.Unmarshal([]byte(out), &recovered); err != nil {
		t.Fatalf("decode recovered status %q: %v", out, err)
	}
	if recovered["authenticated"] != true {
		t.Errorf("credential did not survive the outage: %+v", recovered)
	}
}

// TestAuthStatus_StaleAccessToken_RefreshesAndStaysConnected pins the ordinary
// case, which is by far the most common one and the easiest to break here.
//
// go-account issues device sessions with a two-hour access token against a
// ninety-day refresh window (config/config.prod.yaml: auth.access_expire, and
// the "device" entry under auth.clients). A connector left running overnight
// therefore meets `auth status` with an access token that expired hours ago
// and a refresh token with months left on it. That session is healthy — it
// just needs the refresh every other command would do. If status checked the
// server with the stale token as-is, the server would reject it and status
// would report a perfectly good connection as dead.
func TestAuthStatus_StaleAccessToken_RefreshesAndStaysConnected(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	var mu sync.Mutex
	var refreshed bool
	markRefreshed := func() {
		mu.Lock()
		refreshed = true
		mu.Unlock()
	}
	didRefresh := func() bool {
		mu.Lock()
		defer mu.Unlock()
		return refreshed
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/auth/device/code", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 0, "message": "ok",
			"data": map[string]any{
				"device_code": "dc_stale", "user_code": "STAL-0001",
				"verification_uri": "https://example.test/activate",
				"expires_in":       900, "interval": 1,
			},
		})
	})
	mux.HandleFunc("/v1/auth/device/token", func(w http.ResponseWriter, r *http.Request) {
		// Hand out an access token that is already past its expiry while the
		// refresh window is wide open — the overnight-connector state, without
		// having to wait two hours for it.
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 0, "message": "ok",
			"data": map[string]any{
				"access_token": "at_stale", "refresh_token": "rt_stale",
				"token_type": "bearer",
				"expires_in": -60, "refresh_expires_in": 7776000,
			},
		})
	})
	mux.HandleFunc("/v1/auth/token/refresh", func(w http.ResponseWriter, r *http.Request) {
		markRefreshed()
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 0, "message": "ok",
			"data": map[string]any{
				"access_token": "at_fresh", "refresh_token": "rt_fresh",
				"expires_in": 7200, "refresh_expires_in": 7776000,
			},
		})
	})
	// Record which token each identity lookup carried. `auth login` makes one
	// of these too, with the token it was just handed, so the handler cannot
	// simply reject the stale one — the assertion is on the *last* call, which
	// is the one `auth status` made.
	var profileTokens []string
	mux.HandleFunc("/v1/user/profile", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		profileTokens = append(profileTokens, r.Header.Get("X-Authorization-Token"))
		mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 0, "message": "ok",
			"data": map[string]any{"uid": 45, "nickname": "overnight-user"},
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	bin := build(t)
	configHome := buildProfile(t, map[string]string{"account": srv.URL})
	noToken := []string{"VIBEKNOW_TOKEN="}
	t.Cleanup(func() {
		_, _, _ = runCmdEnv(t, bin, configHome, noToken, "auth", "logout")
	})

	if out, stderr, code := runCmdEnv(t, bin, configHome, noToken, "auth", "login", "--headless"); code != 0 {
		t.Fatalf("setup login: exit %d\nstdout: %s\nstderr: %s", code, out, stderr)
	}

	out, stderr, code := runCmdEnv(t, bin, configHome, noToken, "auth", "status", "--output", "json")
	if code != 0 {
		t.Fatalf("auth status: exit %d\nstdout: %s\nstderr: %s", code, out, stderr)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("decode status %q: %v", out, err)
	}
	if got["authenticated"] != true {
		t.Fatalf("authenticated = %v for a session that only needed a refresh: %+v",
			got["authenticated"], got)
	}
	if !didRefresh() {
		t.Error("status never refreshed the stale access token")
	}
	// The identity lookup must have gone through on the refreshed token,
	// proving the server actually vouched for this credential.
	user, _ := got["user"].(map[string]any)
	if user == nil || user["nickname"] != "overnight-user" {
		t.Errorf("no server-confirmed identity after refresh: %+v", got)
	}
	mu.Lock()
	seen := append([]string(nil), profileTokens...)
	mu.Unlock()
	if len(seen) == 0 {
		t.Fatal("status made no identity lookup at all — it answered from the local clock")
	}
	if last := seen[len(seen)-1]; last != "at_fresh" {
		t.Errorf("status checked the server with %q, want the refreshed token %q; "+
			"calls seen: %v", last, "at_fresh", seen)
	}
}
