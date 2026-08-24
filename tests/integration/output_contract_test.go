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

// runCLIEnv is runVideoCmd with extra environment entries, for the cases
// that need to set VIBEKNOW_OUTPUT.
func runCLIEnv(t *testing.T, bin, configHome string, extraEnv []string, args ...string) (string, string, int) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Env = append(os.Environ(),
		"VIBEKNOW_TOKEN=fake-token",
		"VIBEKNOW_CONFIG_HOME="+configHome,
	)
	cmd.Env = append(cmd.Env, extraEnv...)
	var stdoutBuf, stderrBuf strings.Builder
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf
	code := 0
	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		if ok := asExitError(err, &ee); ok {
			code = ee.ExitCode()
		} else {
			t.Fatalf("run %v: %v", args, err)
		}
	}
	return stdoutBuf.String(), stderrBuf.String(), code
}

func asExitError(err error, out **exec.ExitError) bool {
	ee, ok := err.(*exec.ExitError)
	if ok {
		*out = ee
	}
	return ok
}

// TestOutputJSON_IsHonoredByLocalCommands walks the commands that need no
// backend and asserts each one actually produces JSON when asked.
//
// Before this, ~19 of 33 commands accepted `--output json`, ignored it, and
// printed human prose on stdout — exit 0, no warning. A caller piping into
// jq got a parse error and no way to tell a broken command from one that
// simply never implemented the flag.
func TestOutputJSON_IsHonoredByLocalCommands(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	bin := build(t)
	configHome := buildProfile(t, map[string]string{"figlens": "http://127.0.0.1:1"})

	cases := []struct {
		name string
		args []string
		key  string // a field that must be present in the object
	}{
		{"version", []string{"version"}, "version"},
		{"profile list", []string{"profile", "list"}, "profiles"},
		{"profile show", []string{"profile", "show"}, "name"},
		{"config list", []string{"config", "list"}, "config"},
		{"config set", []string{"config", "set", "k", "v"}, "key"},
		{"config get", []string{"config", "get", "k"}, "value"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			args := append(append([]string{}, tc.args...), "--output", "json")
			stdout, stderr, code := runVideoCmd(t, bin, configHome, args...)
			if code != 0 {
				t.Fatalf("exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
			}
			var obj map[string]any
			if err := json.Unmarshal([]byte(stdout), &obj); err != nil {
				t.Fatalf("--output json must emit a JSON object on stdout, got %q: %v", stdout, err)
			}
			if _, ok := obj[tc.key]; !ok {
				t.Fatalf("payload is missing %q: %v", tc.key, obj)
			}
			if obj["schema_version"] == nil {
				t.Fatalf("every structured payload carries schema_version: %v", obj)
			}
		})
	}
}

// TestOutputFormat_UnknownValueIsRejected pins the other half: an
// unrecognized format is an error, not a silent fall-through to text.
// `--output jsonl` used to print prose and exit 0.
func TestOutputFormat_UnknownValueIsRejected(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	bin := build(t)
	configHome := buildProfile(t, map[string]string{})

	stdout, stderr, code := runVideoCmd(t, bin, configHome, "version", "--output", "jsonl")
	if code != 2 {
		t.Fatalf("unknown --output should be a validation error (exit 2), got %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stderr, "text, json, ndjson") {
		t.Fatalf("the error must list the valid formats, got: %s", stderr)
	}
	if strings.Contains(stdout, "dev") {
		t.Fatalf("nothing should have been printed on stdout, got: %q", stdout)
	}
}

// TestOutputFormat_PathValueHintsAtDest covers the flag collision that the
// rename created a migration path for: `vk video download --output out.mp4`
// is valid released syntax that now means "format = out.mp4". The error has
// to name --dest, or every existing script and skill file breaks with an
// error that does not say how to fix it.
func TestOutputFormat_PathValueHintsAtDest(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	bin := build(t)
	configHome := buildProfile(t, map[string]string{})

	_, stderr, code := runVideoCmd(t, bin, configHome,
		"video", "download", "--session-id", "s_x", "--output", "out.mp4")
	if code != 2 {
		t.Fatalf("expected validation exit 2, got %d\nstderr: %s", code, stderr)
	}
	if !strings.Contains(stderr, "--dest") {
		t.Fatalf("the error must point at --dest, got: %s", stderr)
	}
}

// TestVibeknowOutputEnv_SetsTheDefault covers the env-var precedence chain:
// VIBEKNOW_OUTPUT sets the default so a non-interactive caller does not have
// to thread --output through every invocation, and an explicit --output
// still wins over it.
func TestVibeknowOutputEnv_SetsTheDefault(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	bin := build(t)
	configHome := buildProfile(t, map[string]string{})

	stdout, stderr, code := runCLIEnv(t, bin, configHome, []string{"VIBEKNOW_OUTPUT=json"}, "version")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(stdout), &obj); err != nil {
		t.Fatalf("VIBEKNOW_OUTPUT=json should make json the default, got %q: %v", stdout, err)
	}

	// Explicit flag beats the env var.
	stdout, stderr, code = runCLIEnv(t, bin, configHome, []string{"VIBEKNOW_OUTPUT=json"}, "version", "--output", "text")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if strings.HasPrefix(strings.TrimSpace(stdout), "{") {
		t.Fatalf("--output text must override VIBEKNOW_OUTPUT, got: %q", stdout)
	}

	// A bad env value is rejected the same way a bad flag is — otherwise a
	// typo in a CI environment silently reverts every command to text.
	_, stderr, code = runCLIEnv(t, bin, configHome, []string{"VIBEKNOW_OUTPUT=yaml"}, "version")
	if code != 2 {
		t.Fatalf("a bad VIBEKNOW_OUTPUT should exit 2, got %d\nstderr: %s", code, stderr)
	}
	if !strings.Contains(stderr, "VIBEKNOW_OUTPUT") {
		t.Fatalf("the error must name the env var so the caller knows where the value came from, got: %s", stderr)
	}
}

// TestDoctor_JSONReportSurvivesFailure checks that doctor renders its report
// before returning the failure error. A caller running `vk doctor --output
// json` to find out *what* is broken would otherwise get an empty stdout
// precisely when something is.
func TestDoctor_JSONReportSurvivesFailure(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	bin := build(t)
	// Port 1 is unreachable, so every endpoint probe fails.
	configHome := buildProfile(t, map[string]string{
		"account": "http://127.0.0.1:1", "vectoria": "http://127.0.0.1:1",
		"figlens": "http://127.0.0.1:1", "vibeknow": "http://127.0.0.1:1",
	})

	stdout, stderr, code := runVideoCmd(t, bin, configHome, "doctor", "--output", "json")
	if code == 0 {
		t.Fatalf("doctor with unreachable endpoints must not exit 0\nstdout: %s", stdout)
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(stdout), &obj); err != nil {
		t.Fatalf("the report must still be on stdout when checks fail, got %q: %v\nstderr: %s", stdout, err, stderr)
	}
	if obj["ok"] != false {
		t.Fatalf("ok should be false: %v", obj)
	}
	checks, _ := obj["checks"].([]any)
	if len(checks) == 0 {
		t.Fatalf("checks[] should be populated: %v", obj)
	}
}

// vectoriaUploadStub serves the create-kb → upload → poll sequence that
// `vk doc upload` walks, with the document already completed.
func vectoriaUploadStub(t *testing.T) *httptest.Server {
	t.Helper()
	// vectoria is the one backend that does not use the {"code","data"}
	// envelope (see internal/httpclient/client.go:117), so these bodies are
	// bare objects.
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/v1/knowledgebases" && r.Method == http.MethodPost:
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "kb_stub"})
		case strings.Contains(r.URL.Path, "/documents"):
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "doc_stub123", "status": "completed"})
		default:
			w.WriteHeader(404)
		}
	}))
}

// TestDocUpload_ProgressStaysOnStderr pins the stdout=data discipline for
// the one command an agent runs first: kb_id/doc_id have to be readable
// from stdout alone, with no narration mixed in.
func TestDocUpload_ProgressStaysOnStderr(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	srv := vectoriaUploadStub(t)
	defer srv.Close()

	bin := build(t)
	configHome := buildProfile(t, map[string]string{"vectoria": srv.URL})

	f := filepath.Join(t.TempDir(), "note.txt")
	if err := os.WriteFile(f, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runVideoCmd(t, bin, configHome, "doc", "upload", f, "--output", "json")
	if code != 0 {
		t.Fatalf("exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(stdout), &obj); err != nil {
		t.Fatalf("stdout must be exactly one JSON object, got %q: %v", stdout, err)
	}
	if obj["kb_id"] == "" || obj["doc_id"] == "" {
		t.Fatalf("kb_id/doc_id missing: %v", obj)
	}
	// The narration ("creating knowledge base…", "uploading…") is what used
	// to share stdout with the result.
	if !strings.Contains(stderr, "uploading") {
		t.Fatalf("progress narration should be on stderr, got: %s", stderr)
	}
}
