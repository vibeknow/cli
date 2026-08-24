package integration

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// exportStub serves the four endpoints an export run touches, with the
// export result fixed to the given status/error.
func exportStub(t *testing.T, status, errMsg string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/agent2forVideo/exportRemoteV2":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 0,
				"data": map[string]any{"task_id": 909},
			})
		case "/v1/agent2forVideo/exportResultV2":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 0,
				"data": map[string]any{"status": status, "error": errMsg, "progress": 100},
			})
		case "/v1/works/detailBySession":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 0,
				"data": map[string]any{
					"id": 43, "session_id": "s_exp",
					"title": "Export Test", "share_token": "tok_exp",
					"status": 1, "exporting": 0,
				},
			})
		default:
			w.WriteHeader(404)
		}
	}))
}

// TestVideoExport_FailedRenderDoesNotExitZero pins the rule that a blocking
// command adopts the terminal state it waited for. exportpoll.PollExport
// returns (failedResult, nil) — "the poll succeeded" — and `vk video export`
// used to pass that straight through, printing export.status="failed" while
// exiting 0. An agent branching on the exit code would then call `vk video
// download` for an MP4 that was never rendered.
func TestVideoExport_FailedRenderDoesNotExitZero(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	srv := exportStub(t, "failed", "renderer out of memory")
	defer srv.Close()

	bin := build(t)
	configHome := buildVideoProfile(t, srv.URL)

	stdout, stderr, code := runVideoCmd(t, bin, configHome,
		"video", "export", "42", "--session-id", "s_exp", "--yes", "--output", "json",
	)

	if code != 7 {
		t.Fatalf("failed export should exit 7 (partial: preview exists, MP4 does not), got %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	// The snapshot still has to be emitted — the caller needs the reason and
	// the share URL even though the render failed.
	var out map[string]any
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("--output json should still emit the snapshot, got %q: %v", stdout, err)
	}
	exp, _ := out["export"].(map[string]any)
	if exp["status"] != "failed" {
		t.Fatalf("export.status = %v, want \"failed\" (body: %v)", exp["status"], out)
	}
	if !strings.Contains(stderr, "renderer out of memory") {
		t.Fatalf("stderr should carry the backend's failure reason, got: %s", stderr)
	}
}

// TestVideoExport_SucceededExitsZero is the companion half: the exit-7 path
// must not fire on a normal render.
func TestVideoExport_SucceededExitsZero(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	srv := exportStub(t, "completed", "")
	defer srv.Close()

	bin := build(t)
	configHome := buildVideoProfile(t, srv.URL)

	stdout, stderr, code := runVideoCmd(t, bin, configHome,
		"video", "export", "42", "--session-id", "s_exp", "--yes", "--output", "json",
	)
	if code != 0 {
		t.Fatalf("successful export should exit 0, got %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
}

// TestVideoExportStatus_FailedStillExitsZero pins the other side of the rule:
// a single-shot query succeeded at answering, so it reports the failure in
// the payload rather than in the exit code. Without this distinction an
// agent could not tell "the poll failed" from "the export failed".
func TestVideoExportStatus_FailedStillExitsZero(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	srv := exportStub(t, "failed", "renderer out of memory")
	defer srv.Close()

	bin := build(t)
	configHome := buildVideoProfile(t, srv.URL)

	stdout, stderr, code := runVideoCmd(t, bin, configHome,
		"video", "export-status", "909", "--session-id", "s_exp", "--output", "json",
	)
	if code != 0 {
		t.Fatalf("export-status is a query and should exit 0, got %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("bad json %q: %v", stdout, err)
	}
	exp, _ := out["export"].(map[string]any)
	if exp["status"] != "failed" {
		t.Fatalf("export.status = %v, want \"failed\"", exp["status"])
	}
}

// TestCreate_AsyncWithExportIsRejected pins the flag-combination fix.
// --export runs after the preview snapshot, which --async never reaches, so
// the two used to combine into "silently produce a preview and exit 0" —
// the caller asked for an MP4 and had no way to learn it was not coming.
// The rejection has to happen before any network call, so an accidental
// combination costs nothing.
func TestCreate_AsyncWithExportIsRejected(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.WriteHeader(500)
	}))
	defer srv.Close()

	bin := build(t)
	configHome := buildVideoProfile(t, srv.URL)

	stdout, stderr, code := runVideoCmd(t, bin, configHome,
		"create", "--from", "doc_abc12345", "--kb-id", "kb_test", "--async", "--export",
	)

	if code != 2 {
		t.Fatalf("--async --export should be rejected as a validation error (exit 2), got %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if hits != 0 {
		t.Fatalf("validation must run before any network call, but the backend saw %d requests", hits)
	}
	if !strings.Contains(stderr, "--async") || !strings.Contains(stderr, "--export") {
		t.Fatalf("the error should name both flags, got: %s", stderr)
	}
	// "Errors that teach": the message has to contain the sequence that does
	// work, or the agent's only option is to guess.
	if !strings.Contains(stderr, "vk video export") {
		t.Fatalf("the error should show the two-step alternative, got: %s", stderr)
	}
}
