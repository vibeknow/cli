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

// TestCreate_InsufficientCreditsOnInit_Exits5 covers the bug fixed in 0.5.1:
// when the backend rejects InitTask with envelope code 100001 (insufficient
// credits), the CLI must exit 5 (business failure) to match the stream-side
// path's behavior, not exit 1 from cobra's default error handler.
func TestCreate_InsufficientCreditsOnInit_Exits5(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/tasks/init", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusPaymentRequired) // 402, matches backend
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":    100001,
			"message": "insufficient credits",
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	bin := build(t)
	configHome := buildVideoProfile(t, srv.URL)

	cmd := exec.Command(bin, "create", "--from", "doc_abc12345")
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Env = append(os.Environ(),
		"VIBEKNOW_TOKEN=fake-token",
		"VIBEKNOW_CONFIG_HOME="+configHome,
	)

	err := cmd.Run()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatalf("run: %v\nstderr: %s", err, stderr.String())
	}

	if code != 5 {
		t.Fatalf("exit code = %d, want 5 (business failure)\nstderr: %s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "insufficient credits") {
		t.Fatalf("stderr missing insufficient-credits message:\n%s", stderr.String())
	}
}
