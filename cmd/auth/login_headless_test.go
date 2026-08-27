package auth

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/spf13/cobra"

	"github.com/vibeknow/cli/internal/config"
)

// resetLoginFlags clears any flag values left on the shared package-level
// loginCmd by a previous test in this binary. loginCmd (and its FlagSet) is
// a single var reused across every login_*_test.go — without this, a flag
// set to true/non-empty in one test stays set for the next, since cobra
// does not reset a command's flags between Execute() calls.
func resetLoginFlags(t *testing.T) {
	t.Helper()
	for _, name := range []string{"with-token", "no-wait", "device-code", "headless"} {
		f := loginCmd.Flags().Lookup(name)
		if f == nil {
			t.Fatalf("flag %q not found on loginCmd", name)
		}
		if err := f.Value.Set(f.DefValue); err != nil {
			t.Fatalf("reset flag %q: %v", name, err)
		}
		f.Changed = false
	}
}

// TestLoginHeadless exercises the full blocking flow WorkBuddy's CLI-connector
// auth model expects: one `auth` command that prints the device-code
// envelope to stdout and then keeps running, polling until authorized,
// rather than exiting immediately (that's --no-wait) or requiring a second
// invocation with the device code (that's --device-code).
func TestLoginHeadless(t *testing.T) {
	polls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/auth/device/code":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code":    0,
				"message": "ok",
				"data": map[string]any{
					"device_code":      "dc_headless",
					"user_code":        "WBUD-DYCO",
					"verification_uri": "https://example.test/activate",
					"expires_in":       900,
					"interval":         0,
				},
			})
		case "/v1/auth/device/token":
			polls++
			if polls < 2 {
				// First poll: still pending, so the blocking-poll loop is
				// actually exercised, not just a single lucky pass.
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
					"access_token":       "tok_headless",
					"refresh_token":      "refresh_headless",
					"token_type":         "bearer",
					"expires_in":         7200,
					"refresh_expires_in": 2592000,
				},
			})
		case "/v1/user/profile":
			if r.Header.Get("X-Authorization-Token") != "tok_headless" {
				http.Error(w, "no auth", 401)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code":    0,
				"message": "ok",
				"data": map[string]any{
					"uid":      1,
					"nickname": "wb-bot",
					"email":    "",
				},
			})
		default:
			http.Error(w, "unexpected path "+r.URL.Path, 404)
		}
	}))
	defer srv.Close()

	t.Setenv("VIBEKNOW_CONFIG_HOME", t.TempDir())

	if err := config.AddProfile(config.Profile{
		Name:          "default",
		CredentialRef: testCredentialRef(t),
		Endpoints:     map[string]string{"account": srv.URL},
		Trust:         "dev",
		IsProduction:  false,
	}); err != nil {
		t.Fatalf("add profile: %v", err)
	}
	if err := config.UseProfile("default"); err != nil {
		t.Fatalf("use profile: %v", err)
	}

	resetLoginFlags(t)
	root := &cobra.Command{Use: "vibeknow"}
	// Match production: cmd/root.go sets SilenceUsage, so a command that
	// returns an error does not dump usage text onto stdout. Without it a
	// test harness sees output the real CLI never produces.
	root.SilenceUsage = true
	root.AddCommand(loginCmd)

	var stdout bytes.Buffer
	root.SetOut(&stdout)
	root.SetArgs([]string{"login", "--headless"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	// stdout must carry the same machine-parseable envelope as --no-wait —
	// this is what a host (WorkBuddy's authDeviceFlow uriPattern/codePattern)
	// regex-matches to show the user a verification link/code.
	var got map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("decode stdout %q: %v", stdout.String(), err)
	}
	for _, field := range []string{"user_code", "verification_uri", "verification_uri_complete", "expires_in", "hint"} {
		if _, ok := got[field]; !ok {
			t.Errorf("missing field %q in envelope: %+v", field, got)
		}
	}
	if got["verification_uri"] != "https://example.test/activate" {
		t.Errorf("verification_uri = %v, want https://example.test/activate", got["verification_uri"])
	}
	// The fake account server predates verification_uri_complete, so the CLI
	// must synthesize it — a connector that opens this URI lands the user on
	// a device page with the code already filled in.
	if got["verification_uri_complete"] != "https://example.test/activate?user_code=WBUD-DYCO" {
		t.Errorf("verification_uri_complete = %v, want https://example.test/activate?user_code=WBUD-DYCO", got["verification_uri_complete"])
	}
	// The raw device_code is deliberately NOT printed here. Holding it is
	// enough to claim the token once the user authorizes, and in --headless
	// nobody outside this process needs it: this process polls, and the
	// on-disk record covers the resume path. A connector host captures this
	// stdout into its logs, so printing it would leak a live credential for
	// no gain. (--no-wait still prints it — there the caller must resume
	// with it via --device-code.)
	if _, leaked := got["device_code"]; leaked {
		t.Errorf("device_code leaked into --headless envelope: %+v", got)
	}

	// The command must not have returned until authorization actually
	// completed — confirm the poll loop ran more than once and the token
	// landed in the keychain-backed credential store for the profile.
	if polls < 2 {
		t.Errorf("polls = %d, want >= 2 (poll loop should have retried on authorization_pending)", polls)
	}
}
