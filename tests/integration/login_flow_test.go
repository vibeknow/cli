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
	// Fake account server handling GET /v1/user/profile.
	accountSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "GET" && r.URL.Path == "/v1/user/profile":
			json.NewEncoder(w).Encode(map[string]any{
				"code": 0,
				"data": map[string]any{
					"uid": 12345, "nickname": "TestUser", "email": "test@example.com",
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer accountSrv.Close()

	// Create temp config dir with profiles.yaml pointing to fake server.
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
	if err := os.WriteFile(filepath.Join(configDir, "profiles.yaml"), []byte(profileYAML), 0o600); err != nil {
		t.Fatalf("write profiles.yaml: %v", err)
	}

	// Build the CLI binary.
	bin := build(t)

	// Run: binary auth login --with-token, piping "fake_pat_token_123\n" to stdin.
	cmd := exec.Command(bin, "auth", "login", "--with-token")
	cmd.Env = append(os.Environ(),
		"VIBEKNOW_CONFIG_HOME="+configDir,
		"VIBEKNOW_TOKEN=", // clear env token
	)
	cmd.Stdin = strings.NewReader("fake_pat_token_123\n")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("login --with-token failed (exit %v):\n%s", err, out)
	}
	if !strings.Contains(string(out), "TestUser") {
		t.Fatalf("expected welcome message containing 'TestUser', got:\n%s", out)
	}
}

func TestLoginNoWait(t *testing.T) {
	// Fake account server handling POST /v1/auth/device/code.
	accountSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "POST" && r.URL.Path == "/v1/auth/device/code":
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

	// Create temp config dir with profiles.yaml pointing to fake server.
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
	if err := os.WriteFile(filepath.Join(configDir, "profiles.yaml"), []byte(profileYAML), 0o600); err != nil {
		t.Fatalf("write profiles.yaml: %v", err)
	}

	// Build the CLI binary.
	bin := build(t)

	// Run: binary auth login --no-wait.
	cmd := exec.Command(bin, "auth", "login", "--no-wait")
	cmd.Env = append(os.Environ(),
		"VIBEKNOW_CONFIG_HOME="+configDir,
		"VIBEKNOW_TOKEN=", // clear env token
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("login --no-wait failed (exit %v):\n%s", err, out)
	}

	// Assert stdout is valid JSON with expected fields.
	var result map[string]any
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatalf("expected JSON output, got:\n%s", out)
	}
	if result["device_code"] != "dc_test_123" {
		t.Fatalf("device_code = %v, want dc_test_123", result["device_code"])
	}
	if result["user_code"] != "TEST-CODE" {
		t.Fatalf("user_code = %v, want TEST-CODE", result["user_code"])
	}
}
