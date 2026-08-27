package auth

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/vibeknow/cli/internal/clerr"
	"github.com/vibeknow/cli/internal/config"
	"github.com/vibeknow/cli/internal/keychain"
)

// deviceAuthServer stands in for the account service. authorized controls
// whether the token endpoint reports the user as done or still pending.
func deviceAuthServer(t *testing.T, authorized *bool, tokenCalls *int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/auth/device/token":
			if tokenCalls != nil {
				*tokenCalls++
			}
			if !*authorized {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"code":    40010,
					"message": "authorization_pending",
				})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code":    0,
				"message": "ok",
				"data": map[string]any{
					"access_token":       "tok_resumed",
					"refresh_token":      "refresh_resumed",
					"token_type":         "bearer",
					"expires_in":         7200,
					"refresh_expires_in": 2592000,
				},
			})
		case "/v1/user/profile":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code":    0,
				"message": "ok",
				"data":    map[string]any{"uid": 7, "nickname": "resumed-user"},
			})
		default:
			http.Error(w, "unexpected path "+r.URL.Path, 404)
		}
	}))
}

// testCredentialRef returns a credential ref unique to the calling test, and
// deletes it when the test ends.
//
// `VIBEKNOW_CONFIG_HOME` does not isolate stored credentials, and cannot: they
// live in the OS credential store precisely so they are not sitting in a
// directory. Windows makes that concrete — tokens go to DPAPI +
// `HKCU\Software\VibeknowCLI\keychain`, which no amount of temp-dir juggling
// scopes. So two tests sharing one ref share one credential: the first to
// store a token hands it to every test that runs after, and a test asserting
// "not authenticated" fails on a token it never wrote. The service name is
// fixed (`vibeknow`) and the ref is the account within it, so varying the ref
// is what separates them.
//
// A fixed ref is also how a test suite eats a developer's real login: writing
// to `vibeknow.default` — the ref the actual CLI uses — replaces the token
// they are signed in with, on their own machine, as a side effect of running
// `go test`.
func testCredentialRef(t *testing.T) string {
	t.Helper()
	ref := "vibeknow.test-" + strings.Map(func(r rune) rune {
		if r == '/' || r == ' ' {
			return '-'
		}
		return r
	}, t.Name())
	t.Cleanup(func() {
		if kc, err := keychain.OpenFor("vibeknow"); err == nil {
			_ = kc.Delete(ref)
		}
	})
	return ref
}

// setupPendingProfile isolates config, registers a profile pointing at srv,
// and parks a device code against it.
func setupPendingProfile(t *testing.T, srvURL string, expiresIn time.Duration) {
	t.Helper()
	t.Setenv("VIBEKNOW_CONFIG_HOME", t.TempDir())
	t.Setenv("VIBEKNOW_TOKEN", "")

	if err := config.AddProfile(config.Profile{
		Name:          "default",
		CredentialRef: testCredentialRef(t),
		Endpoints:     map[string]string{"account": srvURL},
		Trust:         "dev",
		IsProduction:  false,
	}); err != nil {
		t.Fatalf("add profile: %v", err)
	}
	if err := config.UseProfile("default"); err != nil {
		t.Fatalf("use profile: %v", err)
	}
	if err := savePendingDevice(pendingDevice{
		DeviceCode:      "dc_parked",
		UserCode:        "WBUD-PARK",
		VerificationURI: srvURL + "/activate",
		Interval:        0,
		ExpiresAt:       time.Now().Add(expiresIn),
		Profile:         "default",
		AccountURL:      srvURL,
	}); err != nil {
		t.Fatalf("park device code: %v", err)
	}
}

// execStatusJSON runs status and returns the payload plus the exit code.
// Unauthenticated is a normal outcome here, and it now exits non-zero, so the
// helper must not treat that as a test failure.
func execStatusJSON(t *testing.T) (map[string]any, int) {
	t.Helper()
	root := &cobra.Command{Use: "vibeknow"}
	// Match production: cmd/root.go sets SilenceUsage, so a command that
	// returns an error does not dump usage text onto stdout. Without it a
	// test harness sees output the real CLI never produces.
	root.SilenceUsage = true
	root.PersistentFlags().String("output", "", "output format")
	root.AddCommand(statusCmd)
	var stdout bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"status", "--output", "json"})
	exitCode := clerr.ExitCodeFor(root.Execute())
	var got map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("decode status stdout %q: %v", stdout.String(), err)
	}
	return got, exitCode
}

// TestStatus_CompletesParkedDeviceLogin is the recovery this whole
// mechanism exists for. A host that spawns `auth login --headless` may kill
// it before the user finishes authorizing — WorkBuddy allows the auth
// subprocess five minutes while the device code lives fifteen. When that
// happens the browser authorization still succeeds, and without a parked
// code the CLI would never learn about it: the user would be told to
// connect again and authorize a second time.
//
// Because the connect and reconnect flows both poll `auth status`, finishing
// the exchange there turns that dead end into a delay.
func TestStatus_CompletesParkedDeviceLogin(t *testing.T) {
	authorized := true
	srv := deviceAuthServer(t, &authorized, nil)
	defer srv.Close()

	setupPendingProfile(t, srv.URL, 10*time.Minute)

	got, exitCode := execStatusJSON(t)
	if got["authenticated"] != true {
		t.Fatalf("authenticated = %v, want true (parked code should have been exchanged): %+v", got["authenticated"], got)
	}

	// The code is single-use; leaving it parked would have `status` keep
	// trying to spend a code that is already spent.
	if _, ok := loadPendingDevice(); ok {
		t.Errorf("parked device code survived a successful exchange")
	}
	// A resumed login is a connected one, and the exit code has to say so —
	// this is the poll on which a connector host stops waiting.
	if exitCode != clerr.ExitOK {
		t.Errorf("exit code = %d, want 0 after a successful resume", exitCode)
	}
}

// TestStatus_PendingAuthorizationIsDistinctFromLoggedOut pins the third
// state. "Not signed in" and "a login is open, waiting on the user" call for
// different actions — start a login vs. wait and re-check — and a caller
// that cannot tell them apart starts a second login on every poll, invalidating
// the code the user is currently looking at.
func TestStatus_PendingAuthorizationIsDistinctFromLoggedOut(t *testing.T) {
	authorized := false
	srv := deviceAuthServer(t, &authorized, nil)
	defer srv.Close()

	setupPendingProfile(t, srv.URL, 10*time.Minute)

	got, exitCode := execStatusJSON(t)
	if got["authenticated"] != false {
		t.Fatalf("authenticated = %v, want false", got["authenticated"])
	}
	if got["pending_authorization"] != true {
		t.Errorf("pending_authorization = %v, want true: %+v", got["pending_authorization"], got)
	}

	// Still unspent — the user has not authorized yet, so the next poll
	// must be able to try again.
	if _, ok := loadPendingDevice(); !ok {
		t.Errorf("parked device code was dropped while still pending")
	}
	// Waiting on the user is still "not connected": the host must keep
	// polling rather than conclude it is done.
	if exitCode != clerr.ExitAuth {
		t.Errorf("exit code = %d, want %d while authorization is pending", exitCode, clerr.ExitAuth)
	}
}

// TestStatus_ExpiredParkedCodeIsDropped keeps a dead code from advertising a
// resume that can never happen: an expired code would otherwise report
// pending_authorization forever, and a caller waiting on it would never
// start the new login that is actually required.
func TestStatus_ExpiredParkedCodeIsDropped(t *testing.T) {
	authorized := true
	tokenCalls := 0
	srv := deviceAuthServer(t, &authorized, &tokenCalls)
	defer srv.Close()

	setupPendingProfile(t, srv.URL, -1*time.Second) // already expired

	got, exitCode := execStatusJSON(t)
	if got["authenticated"] != false {
		t.Fatalf("authenticated = %v, want false", got["authenticated"])
	}
	if _, ok := got["pending_authorization"]; ok {
		t.Errorf("expired code still advertised as pending: %+v", got)
	}
	if tokenCalls != 0 {
		t.Errorf("token endpoint called %d times for an expired code; want 0", tokenCalls)
	}
	if _, ok := loadPendingDevice(); ok {
		t.Errorf("expired device code was not cleared")
	}
	if exitCode != clerr.ExitAuth {
		t.Errorf("exit code = %d, want %d for a dead parked code", exitCode, clerr.ExitAuth)
	}
}

// TestLogout_ClearsParkedDeviceCode covers the disconnect path. `auth
// status` completes parked logins, so a code left behind by a logout would
// let the next status call silently sign the user back in after they
// explicitly disconnected.
func TestLogout_ClearsParkedDeviceCode(t *testing.T) {
	authorized := true
	srv := deviceAuthServer(t, &authorized, nil)
	defer srv.Close()

	setupPendingProfile(t, srv.URL, 10*time.Minute)

	root := &cobra.Command{Use: "vibeknow"}
	// Match production: cmd/root.go sets SilenceUsage, so a command that
	// returns an error does not dump usage text onto stdout. Without it a
	// test harness sees output the real CLI never produces.
	root.SilenceUsage = true
	root.PersistentFlags().String("output", "", "output format")
	root.AddCommand(logoutCmd)
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"logout"})
	if err := root.Execute(); err != nil {
		t.Fatalf("logout: %v", err)
	}

	if _, ok := loadPendingDevice(); ok {
		t.Errorf("logout left a parked device code behind; the next `auth status` would sign the user back in")
	}
}

// TestLogout_WithNoProfileIsStillSuccess pins the disconnect requirement the
// connector spec states outright: unAuth must be idempotent and return
// normally when nothing is signed in.
//
// This is the reachable version of "nothing to log out of" — a machine where
// no profile was ever written, which is exactly what a host sees if it calls
// disconnect before, or twice after, a connect. Returning an error would have
// reported a failed disconnect for a user who is plainly disconnected.
func TestLogout_WithNoProfileIsStillSuccess(t *testing.T) {
	t.Setenv("VIBEKNOW_CONFIG_HOME", t.TempDir())
	t.Setenv("VIBEKNOW_TOKEN", "")

	root := &cobra.Command{Use: "vibeknow"}
	root.SilenceUsage = true
	root.PersistentFlags().String("output", "", "output format")
	root.AddCommand(logoutCmd)
	var stdout bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"logout", "--output", "json"})

	exitCode := clerr.ExitCodeFor(root.Execute())
	if exitCode != clerr.ExitOK {
		t.Fatalf("exit code = %d, want 0: logging out of nothing is a success", exitCode)
	}

	var got map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("decode logout stdout %q: %v", stdout.String(), err)
	}
	// The payload shape has to hold across every logout outcome, so a caller
	// never has to guess which keys exist in which case. `revoked` was
	// missing from this branch alone.
	for _, key := range []string{"profile", "cleared", "revoked", "env_token_still_set"} {
		if _, ok := got[key]; !ok {
			t.Errorf("payload is missing %q: %+v", key, got)
		}
	}
	if got["cleared"] != false {
		t.Errorf("cleared = %v, want false — there was nothing to clear", got["cleared"])
	}
}
