package integration

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// deviceLoginMux is the smallest account stub that `auth login --headless`
// will complete against.
func deviceLoginMux(t *testing.T, access, refresh string) *http.ServeMux {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/auth/device/code", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 0, "message": "ok",
			"data": map[string]any{
				"device_code": "dc_logout", "user_code": "LOGO-UT01",
				"verification_uri": "https://example.test/activate",
				"expires_in":       900, "interval": 1,
			},
		})
	})
	mux.HandleFunc("/v1/auth/device/token", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 0, "message": "ok",
			"data": map[string]any{
				"access_token": access, "refresh_token": refresh,
				"token_type": "bearer",
				"expires_in": 7200, "refresh_expires_in": 7776000,
			},
		})
	})
	mux.HandleFunc("/v1/user/profile", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 0, "message": "ok",
			"data": map[string]any{"uid": 7, "nickname": "logout-user"},
		})
	})
	return mux
}

// TestAuthLogout_RevokesServerSide pins the behaviour that the session store
// was built for: disconnecting has to end the session on the server, not just
// delete the local copy.
//
// Deleting locally was all the CLI could do before, and it left the refresh
// token usable by anyone holding it for the rest of its life — ninety days on
// a device grant. The test asserts the call is made, and that it carries the
// stored refresh token, because a logout that posts an empty body would pass a
// weaker check while revoking nothing.
func TestAuthLogout_RevokesServerSide(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	var mu sync.Mutex
	var logoutCalls int
	var sawToken string

	mux := deviceLoginMux(t, "at_logout", "rt_logout")
	mux.HandleFunc("/v1/auth/logout", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct {
			RefreshToken string `json:"refresh_token"`
		}
		_ = json.Unmarshal(body, &req)
		mu.Lock()
		logoutCalls++
		sawToken = req.RefreshToken
		mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 0, "message": "ok", "data": map[string]any{"ok": true},
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	bin := build(t)
	configHome := buildProfile(t, map[string]string{"account": srv.URL})
	noToken := []string{"VIBEKNOW_TOKEN="}

	if out, stderr, code := runCmdEnv(t, bin, configHome, noToken, "auth", "login", "--headless"); code != 0 {
		t.Fatalf("setup login: exit %d\nstdout: %s\nstderr: %s", code, out, stderr)
	}

	out, stderr, code := runCmdEnv(t, bin, configHome, noToken, "auth", "logout", "--output", "json")
	if code != 0 {
		t.Fatalf("logout: exit %d\nstdout: %s\nstderr: %s", code, out, stderr)
	}

	mu.Lock()
	calls, token := logoutCalls, sawToken
	mu.Unlock()
	if calls != 1 {
		t.Errorf("server-side logout called %d times, want 1 — the session would stay alive "+
			"for up to ninety days", calls)
	}
	if token != "rt_logout" {
		t.Errorf("logout carried refresh_token %q, want %q; the server has nothing to revoke", token, "rt_logout")
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("decode logout output %q: %v", out, err)
	}
	if payload["revoked"] != true {
		t.Errorf("revoked = %v, want true: a caller needs to know whether the token is dead "+
			"everywhere or only gone from this machine: %+v", payload["revoked"], payload)
	}
	if payload["cleared"] != true {
		t.Errorf("cleared = %v, want true: %+v", payload["cleared"], payload)
	}
}

// TestAuthLogout_ServerDown_StillDisconnectsLocally is the more important half.
//
// "Disconnect" is a user's decision about their own machine, and it must not
// be blocked by a server that cannot be reached. Failing here would leave a
// connector host unable to disconnect during an outage — with a credential
// still on disk, still being used by every command.
func TestAuthLogout_ServerDown_StillDisconnectsLocally(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	mux := deviceLoginMux(t, "at_down", "rt_down")
	mux.HandleFunc("/v1/auth/logout", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "gateway is having a day", http.StatusBadGateway)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	bin := build(t)
	configHome := buildProfile(t, map[string]string{"account": srv.URL})
	noToken := []string{"VIBEKNOW_TOKEN="}

	if out, stderr, code := runCmdEnv(t, bin, configHome, noToken, "auth", "login", "--headless"); code != 0 {
		t.Fatalf("setup login: exit %d\nstdout: %s\nstderr: %s", code, out, stderr)
	}

	out, stderr, code := runCmdEnv(t, bin, configHome, noToken, "auth", "logout", "--output", "json")
	if code != 0 {
		t.Fatalf("logout during an outage: exit %d, want 0 — disconnecting must not depend on the "+
			"server being reachable\nstdout: %s\nstderr: %s", code, out, stderr)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("decode %q: %v", out, err)
	}
	if payload["cleared"] != true {
		t.Errorf("cleared = %v, want true — the local credential must go regardless: %+v",
			payload["cleared"], payload)
	}
	// And it must say so rather than claim a revocation that did not happen.
	if payload["revoked"] != false {
		t.Errorf("revoked = %v, want false — reporting a revocation the server never performed "+
			"would be worse than reporting the failure: %+v", payload["revoked"], payload)
	}
	if !strings.Contains(stderr, "could not end the session on the server") {
		t.Errorf("no note explaining the partial logout on stderr: %s", stderr)
	}

	// Genuinely logged out locally — exit 3 is how status says so, which is
	// what lets the host offer a reconnect rather than sit on a stale card.
	out, _, code = runCmdEnv(t, bin, configHome, noToken, "auth", "status", "--output", "json")
	if code != 3 {
		t.Fatalf("status after logout: exit %d, want 3", code)
	}
	var status map[string]any
	if err := json.Unmarshal([]byte(out), &status); err != nil {
		t.Fatalf("decode status %q: %v", out, err)
	}
	if status["authenticated"] == true {
		t.Errorf("still authenticated after logout: %+v", status)
	}
}
