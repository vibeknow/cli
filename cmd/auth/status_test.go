package auth

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/vibeknow/cli/internal/clerr"
	"github.com/vibeknow/cli/internal/credential"
)

// runStatusJSON runs `auth status --output json` and returns the decoded
// payload together with the exit code the process would have used.
//
// The exit code is part of what status reports, not an incidental detail: a
// connector host reads it to decide whether to start a login, so a test that
// only checked the payload would miss the two disagreeing.
func runStatusJSON(t *testing.T, env map[string]string) (map[string]any, int) {
	t.Helper()

	for k, v := range env {
		t.Setenv(k, v)
	}
	t.Setenv("VIBEKNOW_CONFIG_HOME", t.TempDir())

	root := &cobra.Command{Use: "vibeknow"}
	// Match production: cmd/root.go sets SilenceUsage, so a command that
	// returns an error does not dump usage text onto stdout. Without it a
	// test harness sees output the real CLI never produces.
	root.SilenceUsage = true
	root.PersistentFlags().String("output", "", "output format")
	root.AddCommand(statusCmd)

	var stdout bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(os.Stderr)
	root.SetArgs([]string{"status", "--output", "json"})

	exitCode := clerr.ExitCodeFor(root.Execute())

	var got map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("decode json %q: %v", stdout.String(), err)
	}
	return got, exitCode
}

func TestStatusJSONUnauthenticated(t *testing.T) {
	got, exitCode := runStatusJSON(t, map[string]string{})

	if got["schema_version"] != "1" {
		t.Errorf("schema_version = %v, want \"1\"", got["schema_version"])
	}
	if got["authenticated"] != false {
		t.Errorf("authenticated = %v, want false", got["authenticated"])
	}
	if got["source"] != "none" {
		t.Errorf("source = %v, want \"none\"", got["source"])
	}
	if hint, ok := got["hint"].(string); !ok || !strings.Contains(hint, "vibeknow auth login") {
		t.Errorf("hint should mention `vibeknow auth login`, got %v", got["hint"])
	}
	if _, hasUser := got["user"]; hasUser {
		t.Errorf("user should be omitted when unauthenticated, got %v", got["user"])
	}
	// The exit code has to agree with the payload. A connector host decides
	// whether to run its login step from this alone — the WorkBuddy connect
	// sequence is "status → exit code ≠ 0 → auth" — so exiting 0 here would
	// invite it to skip the login and show a connected card for a machine
	// holding no credential.
	if exitCode != clerr.ExitAuth {
		t.Errorf("exit code = %d, want %d: an unauthenticated status that exits 0 tells a "+
			"connector host it is connected", exitCode, clerr.ExitAuth)
	}
}

func TestStatusJSONEnvTokenPresent(t *testing.T) {
	got, exitCode := runStatusJSON(t, map[string]string{"VIBEKNOW_TOKEN": "tok_abc"})

	if got["authenticated"] != true {
		t.Errorf("authenticated = %v, want true", got["authenticated"])
	}
	if got["source"] != "env" {
		t.Errorf("source = %v, want \"env\"", got["source"])
	}
	if got["auth_method"] != "pat" {
		t.Errorf("auth_method = %v, want \"pat\"", got["auth_method"])
	}
	if exitCode != clerr.ExitOK {
		t.Errorf("exit code = %d, want 0 when authenticated", exitCode)
	}
	if got["token_status"] != "valid" {
		t.Errorf("token_status = %v, want \"valid\" (PAT with no expiry)", got["token_status"])
	}
}

func TestUsableCredential(t *testing.T) {
	past := time.Now().UTC().Add(-time.Hour)
	future := time.Now().UTC().Add(time.Hour)

	tests := []struct {
		desc   string
		tok    string
		stored credential.StoredToken
		want   bool
	}{
		{"no token at all", "", credential.StoredToken{}, false},
		{"PAT never expires", "tok", credential.NewPATToken("tok"), true},
		{"oauth valid", "tok", credential.StoredToken{TokenType: "oauth", AccessToken: "tok", ExpiresAt: future, RefreshExpiresAt: future}, true},
		// Access token past expiry but refresh alive → still usable
		// (refreshes transparently on the next call).
		{"oauth needs refresh", "tok", credential.StoredToken{TokenType: "oauth", AccessToken: "tok", ExpiresAt: past, RefreshExpiresAt: future}, true},
		// Refresh token dead too → nothing can succeed; reporting
		// authenticated here is how a dead credential kept showing as a
		// live connection.
		{"oauth fully expired", "tok", credential.StoredToken{TokenType: "oauth", AccessToken: "tok", ExpiresAt: past, RefreshExpiresAt: past}, false},
	}
	for _, tt := range tests {
		if got := usableCredential(tt.tok, tt.stored); got != tt.want {
			t.Errorf("%s: usableCredential = %v, want %v", tt.desc, got, tt.want)
		}
	}
}
