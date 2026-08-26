package integration

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
)

// runUnauthed runs the binary with NO credential at all: no VIBEKNOW_TOKEN
// in the environment and a config home whose credential_ref points at a
// keychain entry that does not exist.
//
// It builds the environment from scratch rather than filtering os.Environ()
// so a VIBEKNOW_TOKEN in the developer's own shell cannot make an
// unauthenticated test pass locally and fail in CI.
func runUnauthed(t *testing.T, bin, configHome string, args ...string) (string, string, int) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Env = []string{
		"VIBEKNOW_CONFIG_HOME=" + configHome,
		"PATH=/usr/bin:/bin",
		"HOME=" + t.TempDir(),
	}
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatalf("run %v: %v", args, err)
	}
	return stdout.String(), stderr.String(), code
}

// TestNewCommands_Unauthenticated_Exit3WithLoginHint pins the auth-guidance
// contract for the commands added for the v3 mode adaptation. An agent
// driving the CLI (WorkBuddy included) decides "the user must connect their
// account" from exit code 3 alone; a new command that leaks a raw transport
// error as exit 1 makes that decision impossible and the agent retries
// forever instead of asking the user to log in.
//
// The catalog commands are the risk here: they were written after the
// create/export paths and never went through an unauthenticated run.
func TestNewCommands_Unauthenticated_Exit3WithLoginHint(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	// The server must never be reached: the CLI is expected to fail on the
	// missing credential before it opens a connection.
	var reached bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
		http.Error(w, "should not be reached", 500)
	}))
	defer srv.Close()

	bin := build(t)
	configHome := buildProfile(t, map[string]string{
		"figlens":  srv.URL,
		"vibeknow": srv.URL,
	})

	cases := []struct {
		name string
		args []string
	}{
		{"theme list", []string{"theme", "list"}},
		{"avatar list", []string{"avatar", "list"}},
		{"voice list", []string{"voice", "list"}},
		{"avatar-retry", []string{"video", "avatar-retry", "--session-id", "s_x"}},
		// Pre-existing commands are in the same table on purpose: the bug
		// this test caught lived in the shared HTTP layer, so pinning only
		// the new commands would let it come back through the old ones.
		{"video list", []string{"video", "list"}},
		{"kb list", []string{"kb", "list"}},
		// create degrades a failed prompt optimisation to a default query;
		// it must NOT degrade past a missing credential.
		{"create", []string{"create", "--from", "doc_authguidance1", "--kb-id", "kb_x"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr, code := runUnauthed(t, bin, configHome, tc.args...)
			if code != 3 {
				t.Errorf("exit = %d, want 3 (auth)\nstdout: %s\nstderr: %s", code, stdout, stderr)
			}
			all := stdout + stderr
			if !strings.Contains(all, "auth login") {
				t.Errorf("no `auth login` hint in output:\nstdout: %s\nstderr: %s", stdout, stderr)
			}
			// The condition is not retryable: no amount of retrying fixes a
			// missing credential, and an agent that reads "network error"
			// will retry instead of asking the user to connect an account.
			if strings.Contains(all, "network_error") {
				t.Errorf("missing credential reported as network_error:\nstdout: %s\nstderr: %s", stdout, stderr)
			}
		})
	}

	if reached {
		t.Errorf("CLI hit the backend without a credential; the missing-token check should short-circuit first")
	}
}

// TestNewCommands_ExpiredToken_Exit3 covers the other half of the auth
// story: a credential exists but the backend rejects it. The CLI must still
// land on exit 3 with a re-login hint, not the generic exit 1, so the
// WorkBuddy connector card can flip back to "disconnected".
func TestNewCommands_ExpiredToken_Exit3(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":    401,
			"message": "token expired",
		})
	}))
	defer srv.Close()

	bin := build(t)
	configHome := buildProfile(t, map[string]string{
		"figlens":  srv.URL,
		"vibeknow": srv.URL,
	})

	cases := []struct {
		name string
		args []string
	}{
		{"theme list", []string{"theme", "list"}},
		{"avatar list", []string{"avatar", "list"}},
		{"voice list", []string{"voice", "list"}},
		{"avatar-retry", []string{"video", "avatar-retry", "--session-id", "s_x"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr, code := runVideoCmd(t, bin, configHome, tc.args...)
			if code != 3 {
				t.Errorf("exit = %d, want 3 (auth)\nstdout: %s\nstderr: %s", code, stdout, stderr)
			}
		})
	}
}
