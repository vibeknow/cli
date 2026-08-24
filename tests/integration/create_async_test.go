package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestCreate_Async_StartsTheRunAndDetaches is the regression test for the
// bug where --async created tasks that never ran.
//
// tasks/init only reserves the task/session/work rows — it is the stream
// request that dispatches the pipeline. --async used to return straight
// after init, so the run was never started, yet the work row was already
// stamped "generating" and looked alive forever.
//
// The stub therefore models the real shape: a long render that emits one
// progress event and then stays open. Passing requires both halves —
// the stream request must be made (proving the run starts) and the client
// must stop reading once it arrives (proving --async still detaches).
func TestCreate_Async_StartsTheRunAndDetaches(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	var mu sync.Mutex
	bodies := map[string]map[string]any{}
	var streamOpened atomic.Bool
	clientLeft := make(chan struct{}, 1)

	figlens := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/tasks/init":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 0,
				"data": map[string]any{"task_id": 42, "session_id": "s_async", "work_id": 43, "v": 3},
			})

		case "/v1/agent2forVideo/fastQueryOptimize":
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "data: {\"code\":200,\"data\":{\"type\":\"aim_result\",\"answer_done\":{\"text\":\"async prompt\"}}}\n\ndata: [DONE]\n\n")

		case "/v1/agent3forVideo/stream":
			streamOpened.Store(true)
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			mu.Lock()
			bodies["stream"] = body
			mu.Unlock()

			w.Header().Set("Content-Type", "text/event-stream")
			flusher, _ := w.(http.Flusher)
			fmt.Fprint(w, "data: {\"code\":200,\"data\":{\"type\":\"process\",\"log\":{\"step_id\":\"\",\"status\":\"start\",\"message\":\"working\"}}}\n\n")
			if flusher != nil {
				flusher.Flush()
			}
			// Hold the stream open the way a real multi-minute render
			// does. If --async did not detach, the CLI would block here
			// until this timeout and the test would fail on duration.
			select {
			case <-r.Context().Done():
				clientLeft <- struct{}{}
			case <-time.After(30 * time.Second):
			}

		default:
			w.WriteHeader(404)
		}
	}))
	defer figlens.Close()

	bin := build(t)
	configHome := buildVideoProfile(t, figlens.URL)

	start := time.Now()
	stdout, stderr, code := runVideoCmd(t, bin, configHome,
		"create", "--from", "doc_async12345", "--kb-id", "kb_test", "--async", "--output", "json",
	)
	elapsed := time.Since(start)

	if code != 0 {
		t.Fatalf("create --async exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}

	if !streamOpened.Load() {
		t.Fatal("--async never opened the stream: the run was never dispatched, which is the original bug")
	}

	select {
	case <-clientLeft:
	case <-time.After(5 * time.Second):
		t.Fatal("--async did not detach after the first event; it stayed attached to the render")
	}

	// Generous bound — the point is "returned while the render was still
	// running", not a latency budget.
	if elapsed > 20*time.Second {
		t.Fatalf("--async took %s; it should return once the run is confirmed, not wait for the render", elapsed)
	}

	mu.Lock()
	streamBody := bodies["stream"]
	mu.Unlock()
	if streamBody["task_id"] == nil || streamBody["query"] == "" {
		// A stream request without the generation parameters would not
		// start anything — it would be an observer reconnect.
		t.Fatalf("stream body must carry the generation parameters, got: %v", streamBody)
	}

	var out map[string]any
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("--output json should emit a JSON object, got %q: %v", stdout, err)
	}
	if out["session_id"] != "s_async" {
		t.Fatalf("session_id = %v, want s_async (body: %v)", out["session_id"], out)
	}
	if out["task_id"] != float64(42) {
		t.Fatalf("task_id = %v, want 42 (body: %v)", out["task_id"], out)
	}
}

// TestCreate_Async_ReportsImmediateFailure covers the other half of the
// contract: --async must not hand back a task_id for a run the backend
// just rejected. Before the fix it could not have noticed — it never read
// the stream at all.
func TestCreate_Async_ReportsImmediateFailure(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	figlens := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/tasks/init":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 0,
				"data": map[string]any{"task_id": 42, "session_id": "s_async", "work_id": 43, "v": 3},
			})

		case "/v1/agent2forVideo/fastQueryOptimize":
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "data: {\"code\":200,\"data\":{\"type\":\"aim_result\",\"answer_done\":{\"text\":\"p\"}}}\n\ndata: [DONE]\n\n")

		case "/v1/agent3forVideo/stream":
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "data: {\"code\":100002,\"data\":{\"type\":\"error\",\"message\":\"insufficient credits\"}}\n\ndata: [DONE]\n\n")

		default:
			w.WriteHeader(404)
		}
	}))
	defer figlens.Close()

	bin := build(t)
	configHome := buildVideoProfile(t, figlens.URL)

	stdout, stderr, code := runVideoCmd(t, bin, configHome,
		"create", "--from", "doc_async12345", "--kb-id", "kb_test", "--async",
	)

	if code == 0 {
		t.Fatalf("--async must not exit 0 when the run was rejected\nstdout: %s\nstderr: %s", stdout, stderr)
	}
	if strings.Contains(stdout, "task_id=") {
		t.Fatalf("--async must not print a task_id for a rejected run, got: %s", stdout)
	}
}

// TestVideoWait_NoEventsIsNotSuccess pins the companion fix: an empty
// stream used to exit 0 with no output, reporting success for a task
// whose state was entirely unknown — the failure mode that let the
// --async bug go unnoticed.
func TestVideoWait_NoEventsIsNotSuccess(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	figlens := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/agent3forVideo/stream" {
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "data: [DONE]\n\n")
			return
		}
		w.WriteHeader(404)
	}))
	defer figlens.Close()

	bin := build(t)
	configHome := buildVideoProfile(t, figlens.URL)

	stdout, stderr, code := runVideoCmd(t, bin, configHome,
		"video", "wait", "42", "--session-id", "s_zombie",
	)

	if code != 6 {
		t.Fatalf("wait on a task with no events should exit 6 (state unknown), got %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stderr, "no events") {
		t.Fatalf("stderr should explain that nothing was observable, got: %s", stderr)
	}
}
