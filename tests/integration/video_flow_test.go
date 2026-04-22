package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

// buildVideoProfile writes a minimal profiles.yaml that points only at figlens
// (no vectoria needed for pure video commands) and returns the XDG_CONFIG_HOME
// path to pass to the binary.
func buildVideoProfile(t *testing.T, figlensURL string) string {
	t.Helper()
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, "vibeknow")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("mkdir config: %v", err)
	}
	profileYAML := fmt.Sprintf(`schema_version: "2"
current: test
profiles:
  - name: test
    endpoints:
      figlens: %s
    credential_ref: test
    trust: dev
    is_production: false
`, figlensURL)
	if err := os.WriteFile(filepath.Join(configDir, "profiles.yaml"), []byte(profileYAML), 0644); err != nil {
		t.Fatalf("write profiles.yaml: %v", err)
	}
	return tmpDir
}

// runVideoCmd runs the binary with the given args, capturing stdout and stderr
// separately. Returns stdout, combined output, and exit code.
func runVideoCmd(t *testing.T, bin, xdgHome string, args ...string) (string, string, int) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Env = append(os.Environ(),
		"VIBEKNOW_TOKEN=fake-token",
		"XDG_CONFIG_HOME="+xdgHome,
		// Keep export timeout short in tests.
		"VIBEKNOW_EXPORT_TIMEOUT=10s",
	)
	var stdoutBuf, stderrBuf strings.Builder
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	err := cmd.Run()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	}
	combined := stdoutBuf.String() + stderrBuf.String()
	return stdoutBuf.String(), combined, code
}

// TestDownload_BeforeExport_Exits2 verifies that `vk video download` exits
// with code 2 and prints "not ready" and "vk video export" when the work row
// has no video_path yet (i.e. export hasn't been run).
func TestDownload_BeforeExport_Exits2(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	figlens := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/v1/works/detailBySession":
			// video_path is empty — export not yet run.
			json.NewEncoder(w).Encode(map[string]any{
				"code": 0,
				"data": map[string]any{
					"id": 43, "title": "Integration Test Video",
					"video_path":  "",
					"share_token": "tok_integ",
					"exporting":   0,
				},
			})
		default:
			w.WriteHeader(404)
		}
	}))
	defer figlens.Close()

	bin := build(t)
	xdgHome := buildVideoProfile(t, figlens.URL)

	_, combined, code := runVideoCmd(t, bin, xdgHome,
		"video", "download", "42", "--session-id", "s_integ",
	)

	if code != 2 {
		t.Fatalf("expected exit 2, got %d\ncombined:\n%s", code, combined)
	}
	lc := strings.ToLower(combined)
	if !strings.Contains(lc, "not ready") {
		t.Errorf("expected 'not ready' in combined output:\n%s", combined)
	}
	if !strings.Contains(combined, "vk video export") {
		t.Errorf("expected 'vk video export' hint in combined output:\n%s", combined)
	}
}

// TestExport_AsyncReturnsImmediately verifies that `vk video export --async`
// submits the export and returns a JSON snapshot with export_task_id and
// status=="running" without polling.
func TestExport_AsyncReturnsImmediately(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	// Track whether exportResultV2 was called (it must NOT be called in async mode).
	var resultCalled int32

	figlens := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/v1/agent2forVideo/exportRemoteV2":
			json.NewEncoder(w).Encode(map[string]any{
				"code": 0,
				"data": map[string]any{"task_id": "exp_7"},
			})
		case r.URL.Path == "/v1/agent2forVideo/exportResultV2":
			atomic.AddInt32(&resultCalled, 1)
			json.NewEncoder(w).Encode(map[string]any{
				"code": 0,
				"data": map[string]any{"status": "processing", "progress": 50},
			})
		case r.URL.Path == "/v1/works/detailBySession":
			// exporting=1 so snapshot derives status=="running".
			json.NewEncoder(w).Encode(map[string]any{
				"code": 0,
				"data": map[string]any{
					"id": 43, "title": "Integration Test Video",
					"video_path":  "",
					"share_token": "tok_integ",
					"exporting":   1,
				},
			})
		default:
			w.WriteHeader(404)
		}
	}))
	defer figlens.Close()

	bin := build(t)
	xdgHome := buildVideoProfile(t, figlens.URL)

	stdout, combined, code := runVideoCmd(t, bin, xdgHome,
		"video", "export", "42",
		"--session-id", "s_integ",
		"--async",
		"--yes",
		"--output", "json",
	)

	if code != 0 {
		t.Fatalf("expected exit 0, got %d\ncombined:\n%s", code, combined)
	}

	// Stdout must be valid JSON.
	var result map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &result); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\nstdout:\n%s", err, stdout)
	}

	// export.export_task_id == "exp_7"
	exportObj, ok := result["export"].(map[string]any)
	if !ok {
		t.Fatalf("expected 'export' object in JSON, got:\n%s", stdout)
	}
	if got := exportObj["export_task_id"]; got != "exp_7" {
		t.Errorf("expected export_task_id=exp_7, got %v\nstdout:\n%s", got, stdout)
	}

	// export.status == "running" (derived from work.exporting==1, no ExportResult).
	if got := exportObj["status"]; got != "running" {
		t.Errorf("expected export.status=running, got %v\nstdout:\n%s", got, stdout)
	}

	// exportResultV2 must not have been called in --async mode.
	if n := atomic.LoadInt32(&resultCalled); n != 0 {
		t.Errorf("exportResultV2 was called %d times; expected 0 in --async mode", n)
	}
}

// TestExport_NDJSON verifies that `vk video export --output ndjson` emits
// progress events followed by a terminal snapshot line, all valid NDJSON.
func TestExport_NDJSON(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	var exportCallCount int32

	figlens := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/v1/agent2forVideo/exportRemoteV2":
			json.NewEncoder(w).Encode(map[string]any{
				"code": 0,
				"data": map[string]any{"task_id": "exp_ndjson"},
			})
		case r.URL.Path == "/v1/agent2forVideo/exportResultV2":
			n := atomic.AddInt32(&exportCallCount, 1)
			if n == 1 {
				json.NewEncoder(w).Encode(map[string]any{
					"code": 0,
					"data": map[string]any{
						"status":       "processing",
						"progress":     50,
						"progress_msg": "rendering",
					},
				})
			} else {
				json.NewEncoder(w).Encode(map[string]any{
					"code": 0,
					"data": map[string]any{
						"status":     "completed",
						"video_path": "final.mp4",
					},
				})
			}
		case r.URL.Path == "/v1/works/detailBySession":
			json.NewEncoder(w).Encode(map[string]any{
				"code": 0,
				"data": map[string]any{
					"id": 43, "title": "Integration Test Video",
					"video_path":  "final.mp4",
					"share_token": "tok_integ",
					"exporting":   0,
				},
			})
		default:
			w.WriteHeader(404)
		}
	}))
	defer figlens.Close()

	bin := build(t)
	xdgHome := buildVideoProfile(t, figlens.URL)

	stdout, combined, code := runVideoCmd(t, bin, xdgHome,
		"video", "export", "42",
		"--session-id", "s_integ",
		"--yes",
		"--output", "ndjson",
		"--poll-interval", "1ms",
	)

	if code != 0 {
		t.Fatalf("expected exit 0, got %d\ncombined:\n%s", code, combined)
	}

	// Every non-empty line must be a valid JSON object.
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	if len(lines) == 0 {
		t.Fatalf("no output lines; combined:\n%s", combined)
	}

	var objects []map[string]any
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var obj map[string]any
		if err := json.Unmarshal([]byte(line), &obj); err != nil {
			t.Fatalf("line %d is not valid JSON: %v\nline: %s\nfull stdout:\n%s", i+1, err, line, stdout)
		}
		objects = append(objects, obj)
	}

	if len(objects) == 0 {
		t.Fatalf("no JSON objects in stdout; combined:\n%s", combined)
	}

	// At least one line must be type=="export.progress" with progress==50.
	foundProgress := false
	for _, obj := range objects {
		if obj["type"] == "export.progress" {
			if prog, ok := obj["progress"]; ok {
				// JSON numbers unmarshal as float64.
				if pf, ok := prog.(float64); ok && int(pf) == 50 {
					foundProgress = true
					break
				}
			}
		}
	}
	if !foundProgress {
		t.Errorf("expected at least one line with type=export.progress and progress=50\nfull stdout:\n%s", stdout)
	}

	// The last object must be type=="snapshot" with export.status=="succeeded".
	last := objects[len(objects)-1]
	if last["type"] != "snapshot" {
		t.Errorf("expected last line type=snapshot, got %v\nfull stdout:\n%s", last["type"], stdout)
	}
	exportObj, ok := last["export"].(map[string]any)
	if !ok {
		t.Errorf("expected 'export' object in last snapshot line\nfull stdout:\n%s", stdout)
	} else if exportObj["status"] != "succeeded" {
		t.Errorf("expected export.status=succeeded in snapshot, got %v\nfull stdout:\n%s", exportObj["status"], stdout)
	}
}

// TestCreate_Export_PartialSuccess_Exits7 verifies that `vk create --export`
// exits 7 when the export poll returns a failed status, while still having
// emitted the preview snapshot to stdout beforehand.
func TestCreate_Export_PartialSuccess_Exits7(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	figlens := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/tasks/init":
			json.NewEncoder(w).Encode(map[string]any{
				"code": 0,
				"data": map[string]any{"task_id": 77, "session_id": "s_partial", "work_id": 78, "v": 3},
			})
		case "/v1/agent3forVideo/stream":
			w.Header().Set("Content-Type", "text/event-stream")
			flusher, _ := w.(http.Flusher)
			for _, ev := range []string{
				`data: {"code":200,"data":{"type":"aim_result","session_id":"s_partial"}}`,
				`data: [DONE]`,
			} {
				fmt.Fprintln(w, ev)
				fmt.Fprintln(w)
				if flusher != nil {
					flusher.Flush()
				}
			}
		case "/v1/works/detailBySession":
			json.NewEncoder(w).Encode(map[string]any{
				"code": 0,
				"data": map[string]any{
					"id": 78, "session_id": "s_partial", "title": "Partial",
					"share_token": "tok_7", "html_path": "w/x.html", "duration": 10000,
				},
			})
		case "/v1/agent2forVideo/exportRemoteV2":
			json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"task_id": "exp_partial"}})
		case "/v1/agent2forVideo/exportResultV2":
			json.NewEncoder(w).Encode(map[string]any{
				"code": 0,
				"data": map[string]any{"status": "failed", "error": "render died"},
			})
		default:
			w.WriteHeader(404)
		}
	}))
	defer figlens.Close()

	xdgHome := buildVideoProfile(t, figlens.URL)
	bin := build(t)

	cmd := exec.Command(bin, "create",
		"--from", "doc_abcdef12345678",
		"--export", "--yes", "--output", "json",
	)
	cmd.Env = append(os.Environ(),
		"VIBEKNOW_TOKEN=fake-token",
		"XDG_CONFIG_HOME="+xdgHome,
		"VIBEKNOW_EXPORT_TIMEOUT=10s",
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()

	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatalf("run: %v\nstderr: %s", err, stderr.String())
	}
	if code != 7 {
		t.Fatalf("expected exit 7, got %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	// Stdout should still contain the preview snapshot (share_url present).
	if !strings.Contains(stdout.String(), `"share_url"`) {
		t.Fatalf("expected share_url in stdout:\n%s", stdout.String())
	}
}
