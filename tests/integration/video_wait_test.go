package integration

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Pins the exit-code parity between `vk video wait` and `vk create`:
// when the stream's terminal task.failed carries retryable=true (mapped
// from a transient backend code like concurrent_work_limit), `vk video
// wait` must exit 4 — not 5 — so a downstream agent can reuse the same
// retry policy regardless of which command consumed the stream. Before
// 0.6.3, wait.go hard-coded exit 5 on any task.failed event.
func TestVideoWait_RetryableFailedExits4(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// wait.go streams via /v1/agent3forVideo/stream by default.
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		fmt.Fprintln(w, `data: {"code":100003,"data":{"message":"too many concurrent works"}}`)
		fmt.Fprintln(w)
		if flusher != nil {
			flusher.Flush()
		}
	}))
	defer srv.Close()

	configHome := buildProfile(t, map[string]string{"figlens": srv.URL})
	stdout, stderr, code := runVideoCmd(t, build(t), configHome,
		"video", "wait", "42", "--session-id", "s_wait_retry", "--output", "ndjson",
	)

	if code != 4 {
		t.Fatalf("exit code = %d, want 4 (retryable). stderr: %s", code, stderr)
	}
	failed := findEvent(t, stdout, "task.failed")
	if failed["code"] != "concurrent_work_limit" {
		t.Errorf("ndjson code = %v, want concurrent_work_limit", failed["code"])
	}
	if failed["retryable"] != true {
		t.Errorf("ndjson retryable = %v, want true", failed["retryable"])
	}
}

// `script_invalid` is an input error, not a task failure: re-running won't
// help. Both `vk create` and `vk video wait` exit 2 (validation), not 5,
// so a caller wrapping either command can branch the same way on bad input.
func TestVideoWait_ScriptInvalidExits2(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		fmt.Fprintln(w, `data: {"code":100004,"data":{"message":"讲稿超过 8000 字"}}`)
		fmt.Fprintln(w)
		if flusher != nil {
			flusher.Flush()
		}
	}))
	defer srv.Close()

	configHome := buildProfile(t, map[string]string{"figlens": srv.URL})
	_, stderr, code := runVideoCmd(t, build(t), configHome,
		"video", "wait", "42", "--session-id", "s_wait_script",
	)

	if code != 2 {
		t.Fatalf("exit code = %d, want 2 (validation/script_invalid). stderr: %s", code, stderr)
	}
}
