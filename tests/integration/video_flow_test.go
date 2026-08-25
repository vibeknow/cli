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
	"sort"
	"strings"
	"sync/atomic"
	"testing"
)

// buildProfile writes a minimal profiles.yaml wiring the given endpoint
// name→URL pairs and returns the config-home path to pass to the binary
// via VIBEKNOW_CONFIG_HOME. Using that var (rather than XDG_CONFIG_HOME)
// keeps tests cross-platform — on Windows the CLI resolves its config
// dir from %AppData% and ignores XDG_CONFIG_HOME entirely.
func buildProfile(t *testing.T, endpoints map[string]string) string {
	t.Helper()
	configDir := filepath.Join(t.TempDir(), "vibeknow")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("mkdir config: %v", err)
	}
	// Sort keys for stable output (helps debugging).
	names := make([]string, 0, len(endpoints))
	for k := range endpoints {
		names = append(names, k)
	}
	sort.Strings(names)
	var endpointsBlock strings.Builder
	for _, name := range names {
		fmt.Fprintf(&endpointsBlock, "      %s: %s\n", name, endpoints[name])
	}
	profileYAML := fmt.Sprintf(`schema_version: "2"
current: test
profiles:
  - name: test
    endpoints:
%s    credential_ref: test
    trust: dev
    is_production: false
`, endpointsBlock.String())
	if err := os.WriteFile(filepath.Join(configDir, "profiles.yaml"), []byte(profileYAML), 0644); err != nil {
		t.Fatalf("write profiles.yaml: %v", err)
	}
	return configDir
}

// buildVideoProfile is a convenience wrapper around buildProfile for tests
// that only need a figlens endpoint. Existing callers stay unchanged.
func buildVideoProfile(t *testing.T, figlensURL string) string {
	return buildProfile(t, map[string]string{"figlens": figlensURL})
}

// runVideoCmd runs the binary with the given args, capturing stdout and
// stderr separately. Returns stdout, stderr, and exit code. Also used by
// non-video tests (kb prune, create-credits, create-mode, create-engine)
// despite the legacy name; rename to runCLI is a follow-up.
//
// Sets test-friendly env vars: VIBEKNOW_TOKEN=fake-token (most mock
// backends ignore auth), VIBEKNOW_CONFIG_HOME from the caller, and
// VIBEKNOW_EXPORT_TIMEOUT=10s so tests that don't care about export
// polling don't get stuck on the default 5min timeout.
func runVideoCmd(t *testing.T, bin, configHome string, args ...string) (string, string, int) {
	t.Helper()
	return runCmdEnv(t, bin, configHome, nil, args...)
}

// runCmdEnv is runVideoCmd with extra environment entries appended, for the
// handful of behaviours that are only reachable through an env var
// (VIBEKNOW_EVENTS, VIBEKNOW_ASSUME_YES).
func runCmdEnv(t *testing.T, bin, configHome string, extraEnv []string, args ...string) (string, string, int) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Env = append(os.Environ(),
		"VIBEKNOW_TOKEN=fake-token",
		"VIBEKNOW_CONFIG_HOME="+configHome,
		"VIBEKNOW_EXPORT_TIMEOUT=10s",
	)
	cmd.Env = append(cmd.Env, extraEnv...)
	var stdoutBuf, stderrBuf strings.Builder
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	err := cmd.Run()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	}
	return stdoutBuf.String(), stderrBuf.String(), code
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
					"status":      1,
				},
			})
		default:
			w.WriteHeader(404)
		}
	}))
	defer figlens.Close()

	bin := build(t)
	configHome := buildVideoProfile(t, figlens.URL)

	stdout, stderr, code := runVideoCmd(t, bin, configHome,
		"video", "download", "42", "--session-id", "s_integ",
	)
	combined := stdout + stderr

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
				"data": map[string]any{"task_id": 77007},
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
					"status":      1,
				},
			})
		default:
			w.WriteHeader(404)
		}
	}))
	defer figlens.Close()

	bin := build(t)
	configHome := buildVideoProfile(t, figlens.URL)

	stdout, stderr, code := runVideoCmd(t, bin, configHome,
		"video", "export", "42",
		"--session-id", "s_integ",
		"--async",
		"--yes",
		"--output", "json",
	)
	combined := stdout + stderr

	if code != 0 {
		t.Fatalf("expected exit 0, got %d\ncombined:\n%s", code, combined)
	}

	// Stdout must be valid JSON.
	var result map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &result); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\nstdout:\n%s", err, stdout)
	}

	// export.export_task_id == 77007
	exportObj, ok := result["export"].(map[string]any)
	if !ok {
		t.Fatalf("expected 'export' object in JSON, got:\n%s", stdout)
	}
	if got, _ := exportObj["export_task_id"].(float64); int64(got) != 77007 {
		t.Errorf("expected export_task_id=77007, got %v\nstdout:\n%s", exportObj["export_task_id"], stdout)
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
				"data": map[string]any{"task_id": 77008},
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
					"status":      1,
				},
			})
		default:
			w.WriteHeader(404)
		}
	}))
	defer figlens.Close()

	bin := build(t)
	configHome := buildVideoProfile(t, figlens.URL)

	stdout, stderr, code := runVideoCmd(t, bin, configHome,
		"video", "export", "42",
		"--session-id", "s_integ",
		"--yes",
		"--output", "ndjson",
		"--poll-interval", "1ms",
	)
	combined := stdout + stderr

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
					"status": 1,
				},
			})
		case "/v1/agent2forVideo/exportRemoteV2":
			json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"task_id": 77009}})
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

	configHome := buildVideoProfile(t, figlens.URL)
	bin := build(t)

	cmd := exec.Command(bin, "create",
		"--from", "doc_abcdef12345678", "--kb-id", "kb_test",
		"--export", "--yes", "--output", "json",
	)
	cmd.Env = append(os.Environ(),
		"VIBEKNOW_TOKEN=fake-token",
		"VIBEKNOW_CONFIG_HOME="+configHome,
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
