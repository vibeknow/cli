package integration

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Pins the end-to-end shape of `vk create --output ndjson` so the
// task.succeeded terminal event carries video_url + duration_ms from the
// backend aim_result payload all the way to stdout. Regression-prevents
// the long-standing bug where agent consumers could not get the video
// URL out of NDJSON because the CLI only forwarded session_id.
func TestCreateNDJSON_TaskSucceededIncludesVideoURLAndDuration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	figlens := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/tasks/init":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"code": 0,
				"data": map[string]any{"task_id": 7, "session_id": "s_ndj", "work_id": 8, "v": 3},
			})
		case "/v1/agent3forVideo/stream":
			w.Header().Set("Content-Type", "text/event-stream")
			flusher, _ := w.(http.Flusher)
			for _, e := range []string{
				`data: {"code":200,"data":{"type":"aim_result","session_id":"s_ndj","html_path":"https://cdn.example.com/v/s_ndj.html","data":{"duration_ms":30000}}}`,
				`data: [DONE]`,
			} {
				fmt.Fprintln(w, e)
				fmt.Fprintln(w)
				if flusher != nil {
					flusher.Flush()
				}
			}
		case "/v1/works/detailBySession":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"code": 0,
				"data": map[string]any{"id": 8, "title": "t", "video_path": "/x.mp4", "duration": 30, "share_token": "tok", "status": 1},
			})
		default:
			w.WriteHeader(404)
		}
	}))
	defer figlens.Close()

	configHome := buildProfile(t, map[string]string{"figlens": figlens.URL})
	stdout, stderr, code := runVideoCmd(t, build(t), configHome,
		"create", "--from", "doc_smoke12345", "--output", "ndjson")

	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr)
	}

	succeeded := findEvent(t, stdout, "task.succeeded")
	if succeeded["video_url"] != "https://cdn.example.com/v/s_ndj.html" {
		t.Errorf("video_url = %v, want backend html_path", succeeded["video_url"])
	}
	if got, want := jsonNumberInt(t, succeeded["duration_ms"]), int64(30000); got != want {
		t.Errorf("duration_ms = %d, want %d", got, want)
	}
}

// Pins task.failed shape: retryable must be present so consumers can
// branch on it, and the CLI must select exit 4 (retryable) instead of 5
// when the backend code is one we map as transient. concurrent_work_limit
// (100003) is the production canary for that path.
func TestCreateNDJSON_TaskFailedRetryableExitsFour(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	figlens := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/tasks/init":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"code": 0,
				"data": map[string]any{"task_id": 9, "session_id": "s_retry", "work_id": 10, "v": 3},
			})
		case "/v1/agent3forVideo/stream":
			w.Header().Set("Content-Type", "text/event-stream")
			flusher, _ := w.(http.Flusher)
			fmt.Fprintln(w, `data: {"code":100003,"data":{"message":"too many concurrent works"}}`)
			fmt.Fprintln(w)
			if flusher != nil {
				flusher.Flush()
			}
		default:
			w.WriteHeader(404)
		}
	}))
	defer figlens.Close()

	configHome := buildProfile(t, map[string]string{"figlens": figlens.URL})
	stdout, stderr, code := runVideoCmd(t, build(t), configHome,
		"create", "--from", "doc_smoke12345", "--output", "ndjson")

	if code != 4 {
		t.Fatalf("exit=%d want 4 (retryable). stderr=%s", code, stderr)
	}

	failed := findEvent(t, stdout, "task.failed")
	if failed["code"] != "concurrent_work_limit" {
		t.Errorf("code = %v, want concurrent_work_limit", failed["code"])
	}
	if failed["retryable"] != true {
		t.Errorf("retryable = %v, want true", failed["retryable"])
	}
}

func findEvent(t *testing.T, stdout, typ string) map[string]any {
	t.Helper()
	sc := bufio.NewScanner(strings.NewReader(stdout))
	for sc.Scan() {
		var m map[string]any
		if err := json.Unmarshal(sc.Bytes(), &m); err != nil {
			continue
		}
		if m["type"] == typ {
			return m
		}
	}
	t.Fatalf("event %q not found in stdout:\n%s", typ, stdout)
	return nil
}

// json.Unmarshal decodes numbers into float64 by default; cast safely.
func jsonNumberInt(t *testing.T, v any) int64 {
	t.Helper()
	f, ok := v.(float64)
	if !ok {
		t.Fatalf("expected numeric value, got %T (%v)", v, v)
	}
	return int64(f)
}
