package integration

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestAuthWhoamiAgainstFakeAccount(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/user/profile" {
			http.Error(w, "not found", 404)
			return
		}
		if r.Header.Get("Authorization") != "Bearer e2e-token" {
			http.Error(w, "forbidden", 401)
			return
		}
		w.Header().Set("X-Vibeknow-Api-Version", "v1")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"uid":      "u_e2e",
			"nickname": "e2eUser",
			"email":    "e2e@example.com",
		})
	}))
	defer srv.Close()

	bin := build(t)
	home := t.TempDir()

	// Create dev profile with account endpoint pointing at fake server.
	_, _, code := run(t, bin, home,
		"profile", "add", "dev",
		"--credential-ref", "vibeknow.dev",
		"--trust", "dev",
		"--is-production=false",
		"--endpoint-account", srv.URL,
	)
	if code != 0 {
		t.Fatal("profile add")
	}
	_, _, _ = run(t, bin, home, "profile", "use", "dev")

	// Call whoami with VIBEKNOW_TOKEN set.
	cmd := exec.Command(bin, "auth", "whoami")
	cmd.Env = append(os.Environ(),
		"VIBEKNOW_CONFIG_HOME="+home,
		"VIBEKNOW_TOKEN=e2e-token",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("whoami: err=%v out=%s", err, string(out))
	}
	s := string(out)
	if !strings.Contains(s, "u_e2e") || !strings.Contains(s, "e2eUser") {
		t.Errorf("whoami output missing user info: %q", s)
	}
}
