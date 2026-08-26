package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// streamServer serves a fixed list of raw SSE frames from the v3 stream
// endpoint, with the surrounding init/detail calls stubbed out.
func streamServer(t *testing.T, frames []string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/tasks/init":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 0,
				"data": map[string]any{"task_id": 5, "session_id": "s_life", "work_id": 6, "v": 3},
			})
		case "/v1/agent3forVideo/stream":
			w.Header().Set("Content-Type", "text/event-stream")
			flusher, _ := w.(http.Flusher)
			for _, e := range frames {
				fmt.Fprintln(w, e)
				fmt.Fprintln(w)
				if flusher != nil {
					flusher.Flush()
				}
			}
		case "/v1/works/detailBySession":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 0,
				"data": map[string]any{
					"id": 6, "session_id": "s_life", "title": "Lifecycle",
					"html_path": "works/life/index.html", "share_token": "tok_life",
					"exporting": 0, "status": 1,
				},
			})
		default:
			w.WriteHeader(404)
		}
	}))
}

func createArgs(extra ...string) []string {
	return append([]string{
		"create", "--from", "doc_lifecycle01", "--kb-id", "kb_test", "--prompt", "p",
	}, extra...)
}

// TestStream_EnvelopedDoneSentinel covers the terminator the v3 pipeline
// actually sends. The bare `data: [DONE]` line is the v2 shape; v3 wraps it
// like every other frame, as {"code":200,"data":{"msg":"[DONE]"}}. A client
// that only knows the bare form does not see the stream end — it waits for
// a frame that already went past, and the run hangs until something times
// out despite having completed successfully.
func TestStream_EnvelopedDoneSentinel(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	srv := streamServer(t, []string{
		`data: {"code":200,"data":{"type":"process","log":{"step_id":"script_writing","status":"success","message":"ok"}}}`,
		`data: {"code":200,"data":{"type":"aim_result","session_id":"s_life"}}`,
		`data: {"code":200,"data":{"msg":"[DONE]"}}`,
	})
	defer srv.Close()

	bin := build(t)
	configHome := buildVideoProfile(t, srv.URL)

	stdout, stderr, code := runVideoCmd(t, bin, configHome, createArgs()...)
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (enveloped [DONE] must end the stream)\nstdout: %s\nstderr: %s",
			code, stdout, stderr)
	}
}

// TestStream_KeepaliveIsSilent pins that the heartbeat frames the backend
// sends during long silent stretches are consumed without being rendered.
// Surfacing them would fill an agent's transcript with content-free lines
// while nothing is happening.
func TestStream_KeepaliveIsSilent(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	srv := streamServer(t, []string{
		`data: {"code":200,"data":{"type":"keepalive"}}`,
		`data: {"code":200,"data":{"type":"keepalive"}}`,
		`data: {"code":200,"data":{"type":"process","log":{"step_id":"tts_generate","status":"success","message":"ok"}}}`,
		`data: {"code":200,"data":{"type":"aim_result","session_id":"s_life"}}`,
		`data: [DONE]`,
	})
	defer srv.Close()

	bin := build(t)
	configHome := buildVideoProfile(t, srv.URL)

	stdout, stderr, code := runVideoCmd(t, bin, configHome, append(createArgs(), "--output", "ndjson")...)
	if code != 0 {
		t.Fatalf("exit = %d, want 0\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if strings.Contains(stdout, "keepalive") {
		t.Errorf("keepalive frames leaked into the event stream:\n%s", stdout)
	}
}

// TestStream_Paused_Exits6 covers a run that stops without failing. A paused
// task has produced real work and can be resumed in the editor, so calling
// it a failure would be wrong — and exit 0 would be worse, since there is no
// finished video. Exit 6 is the non-terminal code, the same one an
// interrupted stream uses.
func TestStream_Paused_Exits6(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	srv := streamServer(t, []string{
		`data: {"code":200,"data":{"type":"process","log":{"step_id":"big_director","status":"start","message":"planning"}}}`,
		`data: {"code":200,"data":{"type":"paused","message":"paused by user"}}`,
		`data: [DONE]`,
	})
	defer srv.Close()

	bin := build(t)
	configHome := buildVideoProfile(t, srv.URL)

	stdout, stderr, code := runVideoCmd(t, bin, configHome, append(createArgs(), "--output", "ndjson")...)
	if code != 6 {
		t.Fatalf("exit = %d, want 6 (non-terminal)\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	ev := findEvent(t, stdout, "task.paused")
	if ev == nil {
		t.Fatalf("no task.paused event emitted:\n%s", stdout)
	}
}

// TestStream_UnmappedNodeDegrades keeps an unrecognised step_id from being
// dropped or fatal. The backend's node set moves, and a CLI that only
// forwards nodes it has a stage for would go silent on exactly the new work
// a user is most likely to ask about.
//
// The node's raw wire name is withheld on purpose: unrecognised step_ids are
// precisely the ones that may carry an internal codename, which is why known
// nodes get a sanitised display name in the first place. The backend's
// message is already user-facing, so that is what gets through.
func TestStream_UnmappedNodeDegrades(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	srv := streamServer(t, []string{
		`data: {"code":200,"data":{"type":"process","log":{"step_id":"some_future_node","status":"start","message":"new thing"}}}`,
		`data: {"code":200,"data":{"type":"aim_result","session_id":"s_life"}}`,
		`data: [DONE]`,
	})
	defer srv.Close()

	bin := build(t)
	configHome := buildVideoProfile(t, srv.URL)

	stdout, stderr, code := runVideoCmd(t, bin, configHome, append(createArgs(), "--output", "ndjson")...)
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (an unknown node is not a failure)\nstdout: %s\nstderr: %s",
			code, stdout, stderr)
	}
	progress := findEvent(t, stdout, "node.progress")
	if progress["message"] != "new thing" {
		t.Errorf("unmapped node was dropped instead of reported as progress: %+v", progress)
	}
	if strings.Contains(stdout, "some_future_node") {
		t.Errorf("raw wire name of an unknown node leaked to the user:\n%s", stdout)
	}
}

// TestCreate_BusinessCodes_ExitContract pins the exit code for each backend
// rejection an agent has to tell apart. The codes matter more than the
// messages: an agent decides whether to retry, ask the user to fix the
// input, or report the run dead purely from the exit code.
func TestCreate_BusinessCodes_ExitContract(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	cases := []struct {
		name     string
		envCode  int
		wantExit int
		wantCode string
	}{
		{"insufficient credits", 100001, 5, "insufficient_credits"},
		{"concurrent work limit", 100003, 4, "concurrent_work_limit"},
		{"script invalid", 100004, 2, "script_invalid"},
		{"replica invalid", 100005, 2, "replica_invalid"},
		{"knowledge unsupported", 100006, 2, "knowledge_unsupported"},
		{"image params invalid", 100007, 2, "image_invalid"},
		{"work being edited", 100008, 4, "work_edit_busy"},
		{"project quota exceeded", 100009, 5, "project_quota_exceeded"},
		{"project works full", 100010, 5, "project_works_full"},
		{"tts preview quota", 100011, 5, "tts_preview_quota_exceeded"},
	}

	bin := build(t)

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"code":    tc.envCode,
					"message": tc.name,
				})
			}))
			defer srv.Close()

			configHome := buildVideoProfile(t, srv.URL)
			stdout, stderr, code := runVideoCmd(t, bin, configHome,
				append(createArgs(), "--output", "ndjson")...)

			if code != tc.wantExit {
				t.Errorf("exit = %d, want %d\nstdout: %s\nstderr: %s", code, tc.wantExit, stdout, stderr)
			}
			failed := findEvent(t, stdout, "task.failed")
			if failed["code"] != tc.wantCode {
				t.Errorf("ndjson code = %v, want %q", failed["code"], tc.wantCode)
			}
			// retryable must agree with the exit code, or an agent reading
			// the event and an agent reading $? reach opposite conclusions.
			wantRetryable := tc.wantExit == 4
			if failed["retryable"] != wantRetryable {
				t.Errorf("retryable = %v, want %v (exit %d)", failed["retryable"], wantRetryable, tc.wantExit)
			}
		})
	}
}
