package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreateFlow_FileToVideo(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	// Fake vectoria server.
	vectoria := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "POST" && r.URL.Path == "/knowledgebases":
			json.NewEncoder(w).Encode(map[string]string{"id": "kb_test123"})
		case r.Method == "POST" && strings.Contains(r.URL.Path, "/documents/file"):
			r.ParseMultipartForm(32 << 20)
			json.NewEncoder(w).Encode(map[string]string{"id": "doc_test123abc", "status": "processing"})
		case r.Method == "GET" && strings.Contains(r.URL.Path, "/documents/"):
			json.NewEncoder(w).Encode(map[string]string{"id": "doc_test123abc", "status": "completed"})
		default:
			w.WriteHeader(404)
		}
	}))
	defer vectoria.Close()

	// Fake figlens server.
	figlens := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/v1/tasks/init":
			json.NewEncoder(w).Encode(map[string]any{
				"code": 200,
				"data": map[string]any{"task_id": 42, "session_id": "s_integ", "work_id": "w_integ"},
			})
		case r.URL.Path == "/v1/agent3forVideo/stream":
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(200)
			flusher, _ := w.(http.Flusher)
			events := []string{
				`data: {"code":200,"data":{"type":"process","log":{"step_id":"prepare","status":"start","message":"go"}}}`,
				`data: {"code":200,"data":{"type":"process","log":{"step_id":"prepare","status":"success","message":"ok"}}}`,
				`data: {"code":200,"data":{"type":"aim_result","session_id":"s_integ"}}`,
				`data: [DONE]`,
			}
			for _, e := range events {
				fmt.Fprintln(w, e)
				fmt.Fprintln(w)
				if flusher != nil {
					flusher.Flush()
				}
			}
		case r.URL.Path == "/v1/works/detailBySession":
			json.NewEncoder(w).Encode(map[string]any{
				"code": 200,
				"data": map[string]any{
					"id": "w_integ", "title": "Integration Test Video",
					"video_path": "/test.mp4", "cover_url": "", "duration": 30,
				},
			})
		default:
			w.WriteHeader(404)
		}
	}))
	defer figlens.Close()

	// Temp directory setup.
	tmpDir := t.TempDir()

	// Write test file.
	testFile := filepath.Join(tmpDir, "test.txt")
	os.WriteFile(testFile, []byte("test content"), 0644)

	// Write profile config at $XDG_CONFIG_HOME/vibeknow/profiles.yaml.
	configDir := filepath.Join(tmpDir, "config", "vibeknow")
	os.MkdirAll(configDir, 0755)
	profileYAML := fmt.Sprintf(`schema_version: "2"
current: test
profiles:
  - name: test
    endpoints:
      vectoria: %s
      figlens: %s
    credential_ref: test
    trust: dev
    is_production: false
`, vectoria.URL, figlens.URL)
	os.WriteFile(filepath.Join(configDir, "profiles.yaml"), []byte(profileYAML), 0644)

	// Build binary.
	bin := build(t)

	// Run create command.
	cmd := exec.Command(bin, "create", "--from", testFile)
	cmd.Env = append(os.Environ(),
		"VIBEKNOW_TOKEN=fake-token",
		"VECTORIA_API_KEY=fake-key",
		"XDG_CONFIG_HOME="+filepath.Join(tmpDir, "config"),
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("create failed (exit %v):\n%s", err, out)
	}

	output := string(out)
	if !strings.Contains(output, "task_id=42") {
		t.Errorf("expected task_id=42 in output:\n%s", output)
	}
	if !strings.Contains(output, "work_id=w_integ") {
		t.Errorf("expected work_id=w_integ in output:\n%s", output)
	}
}
