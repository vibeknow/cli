package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestCreate_ModeReplica_WiresVideoKind(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	var mu sync.Mutex
	bodies := map[string]map[string]any{}

	figlens := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/tasks/init":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			mu.Lock()
			bodies["init"] = body
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 0,
				"data": map[string]any{"task_id": 42, "session_id": "s_replica", "work_id": 43, "v": 3},
			})

		case "/v1/agent2forVideo/fastQueryOptimize":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			mu.Lock()
			bodies["optimize"] = body
			mu.Unlock()
			w.Header().Set("Content-Type", "text/event-stream")
			flusher, _ := w.(http.Flusher)
			for _, e := range []string{
				`data: {"code":200,"data":{"type":"aim_result","answer_done":{"text":"replica prompt"}}}`,
				`data: [DONE]`,
			} {
				fmt.Fprintln(w, e)
				fmt.Fprintln(w)
				if flusher != nil {
					flusher.Flush()
				}
			}

		case "/v1/agent3forVideo/stream":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			mu.Lock()
			bodies["stream"] = body
			mu.Unlock()
			w.Header().Set("Content-Type", "text/event-stream")
			flusher, _ := w.(http.Flusher)
			events := []string{
				`data: {"code":200,"data":{"type":"process","log":{"step_id":"doc_replica_plan","status":"start","message":"go"}}}`,
				`data: {"code":200,"data":{"type":"process","log":{"step_id":"doc_replica_plan","status":"success","message":"ok"}}}`,
				`data: {"code":200,"data":{"type":"aim_result","session_id":"s_replica"}}`,
				`data: [DONE]`,
			}
			for _, e := range events {
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
					"id": 43, "session_id": "s_replica",
					"title":       "Replica Test",
					"html_path":   "works/replica/index.html",
					"share_token": "tok_replica",
					"exporting":   0,
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
		"create", "--mode", "replica", "--from", "doc_abc12345",
	)

	if code != 0 {
		t.Fatalf("create exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}

	mu.Lock()
	defer mu.Unlock()

	if bodies["init"]["video_kind"] != "replica" {
		t.Fatalf("init body video_kind = %v, want \"replica\"", bodies["init"]["video_kind"])
	}
	if _, ok := bodies["init"]["knowledge_id"]; ok {
		t.Fatalf("init body should not carry knowledge_id for non-script mode: %v", bodies["init"])
	}
	if _, ok := bodies["init"]["doc_id"]; ok {
		t.Fatalf("init body should not carry doc_id for non-script mode: %v", bodies["init"])
	}
	if bodies["stream"]["video_kind"] != "replica" {
		t.Fatalf("stream body video_kind = %v, want \"replica\"", bodies["stream"]["video_kind"])
	}
	out := stdout + stderr
	if !strings.Contains(out, "doc_replica_plan") {
		t.Fatalf("expected doc_replica_plan in output (proves stage map), got:\nstdout: %s\nstderr: %s", stdout, stderr)
	}
}
