package integration

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// pipelineControlServer stands in for figlens' pause/resume pair. status is
// the HTTP code to answer with, payload the body.
func pipelineControlServer(t *testing.T, path string, status int, payload map[string]any, seen *map[string]any) *httptest.Server {
	t.Helper()
	var mu sync.Mutex
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != path {
			w.WriteHeader(404)
			return
		}
		if seen != nil {
			mu.Lock()
			_ = json.NewDecoder(r.Body).Decode(seen)
			mu.Unlock()
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(payload)
	}))
}

// TestPause_StopsARunAndPointsAtResume covers the action that was missing
// entirely: an agent driven by natural language will start the wrong run, and
// until now nothing could stop it — the user paid for a video they did not
// want and could only wait for it.
func TestPause_StopsARunAndPointsAtResume(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	var body map[string]any
	srv := pipelineControlServer(t, "/v1/pipeline/pause", 200, map[string]any{
		"code": 0,
		"data": map[string]any{"session_id": "s_run", "status": "paused"},
	}, &body)
	defer srv.Close()

	bin := build(t)
	configHome := buildVideoProfile(t, srv.URL)

	stdout, stderr, code := runVideoCmd(t, bin, configHome,
		"video", "pause", "42", "--session-id", "s_run", "--output", "json")
	if code != 0 {
		t.Fatalf("pause exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if body["session_id"] != "s_run" {
		t.Errorf("session_id sent = %v, want s_run", body["session_id"])
	}

	var got map[string]any
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("decode %q: %v", stdout, err)
	}
	if got["status"] != "paused" {
		t.Errorf("status = %v, want paused: %+v", got["status"], got)
	}
	// Stopping is only half an answer; the caller has to be told the run is
	// recoverable, or "pause" reads as "throw away".
	actions, _ := got["next_actions"].([]any)
	if len(actions) == 0 {
		t.Fatalf("no next_actions: %+v", got)
	}
	first, _ := actions[0].(map[string]any)
	if cmd, _ := first["command"].(string); cmd == "" || !strings.Contains(cmd, "video resume") {
		t.Errorf("next action does not point at resume: %+v", first)
	}
}

// TestResume_FailedRunRetriesFromCheckpoint pins the distinction the mode
// field exists for. Retrying a failed run reuses its checkpoint and reopens
// the original bill; the alternative an agent would otherwise pick — create
// the video again — is a second full charge for work already paid for.
func TestResume_FailedRunRetriesFromCheckpoint(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	srv := pipelineControlServer(t, "/v1/pipeline/resume", 200, map[string]any{
		"code": 0,
		"data": map[string]any{
			"session_id": "s_run", "status": "resumed", "mode": "failed_checkpoint_retry",
		},
	}, nil)
	defer srv.Close()

	bin := build(t)
	configHome := buildVideoProfile(t, srv.URL)

	stdout, stderr, code := runVideoCmd(t, bin, configHome,
		"video", "resume", "42", "--session-id", "s_run", "--output", "json")
	if code != 0 {
		t.Fatalf("resume exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("decode %q: %v", stdout, err)
	}
	if got["mode"] != "failed_checkpoint_retry" {
		t.Errorf("mode = %v, want failed_checkpoint_retry — a caller that cannot tell "+
			"this from a plain resume cannot tell the user what it just paid for: %+v", got["mode"], got)
	}
}

// TestPipelineControl_PermanentRefusalIsExit5 is the reason these commands
// classify their own errors.
//
// The backend answers every refusal with HTTP 400 and a sentence, which lands
// on the catch-all exit 1 — the code whose reasonable response is to try
// again. But "this engine keeps no checkpoint" is settled: retrying produces
// the identical sentence forever. Exit 5 says the request was fine and the
// run still cannot proceed, so the answer is to tell the user why.
func TestPipelineControl_PermanentRefusalIsExit5(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	srv := pipelineControlServer(t, "/v1/pipeline/resume", 400, map[string]any{
		"code":    400,
		"message": `resume: work engine="agent" has no pipeline checkpoint, cannot be resumed`,
	}, nil)
	defer srv.Close()

	bin := build(t)
	configHome := buildVideoProfile(t, srv.URL)

	stdout, stderr, code := runVideoCmd(t, bin, configHome,
		"video", "resume", "42", "--session-id", "s_run", "--output", "json")
	if code != 5 {
		t.Fatalf("resume of an agent-engine run: exit %d, want 5 (permanent)\nstdout: %s\nstderr: %s",
			code, stdout, stderr)
	}
	// The reason has to survive: exit 5 tells the agent to stop, the message
	// is what it repeats to the user.
	if !strings.Contains(stderr+stdout, "no pipeline checkpoint") {
		t.Errorf("the backend's reason was dropped; nothing to tell the user\nstdout: %s\nstderr: %s", stdout, stderr)
	}
}

// TestPipelineControl_BusySessionIsExit4 is the carve-out. Two pause/resume
// calls racing on one session is the single refusal here that a retry does
// clear, and it must not be swept into the permanent bucket with the rest.
func TestPipelineControl_BusySessionIsExit4(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	srv := pipelineControlServer(t, "/v1/pipeline/pause", 400, map[string]any{
		"code":    400,
		"message": "session busy (pausing or resuming), please retry later",
	}, nil)
	defer srv.Close()

	bin := build(t)
	configHome := buildVideoProfile(t, srv.URL)

	stdout, stderr, code := runVideoCmd(t, bin, configHome,
		"video", "pause", "42", "--session-id", "s_run", "--output", "json")
	if code != 4 {
		t.Fatalf("busy session: exit %d, want 4 (retryable)\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
}
