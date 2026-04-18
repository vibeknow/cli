package auth

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func runStatusJSON(t *testing.T, env map[string]string) map[string]any {
	t.Helper()

	for k, v := range env {
		t.Setenv(k, v)
	}
	t.Setenv("VIBEKNOW_CONFIG_HOME", t.TempDir())

	root := &cobra.Command{Use: "vibeknow"}
	root.PersistentFlags().String("output", "", "output format")
	root.AddCommand(statusCmd)

	var stdout bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(os.Stderr)
	root.SetArgs([]string{"status", "--output", "json"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("decode json %q: %v", stdout.String(), err)
	}
	return got
}

func TestStatusJSONUnauthenticated(t *testing.T) {
	got := runStatusJSON(t, map[string]string{})

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
}

func TestStatusJSONEnvTokenPresent(t *testing.T) {
	got := runStatusJSON(t, map[string]string{"VIBEKNOW_TOKEN": "tok_abc"})

	if got["authenticated"] != true {
		t.Errorf("authenticated = %v, want true", got["authenticated"])
	}
	if got["source"] != "env" {
		t.Errorf("source = %v, want \"env\"", got["source"])
	}
	if got["auth_method"] != "pat" {
		t.Errorf("auth_method = %v, want \"pat\"", got["auth_method"])
	}
	if got["token_status"] != "valid" {
		t.Errorf("token_status = %v, want \"valid\" (PAT with no expiry)", got["token_status"])
	}
}
