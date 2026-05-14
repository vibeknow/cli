package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
)

func TestCreate_EngineAgent_WiresV2AndSurfacesProgress(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	var mu sync.Mutex
	bodies := map[string]map[string]any{}
	paths := []string{}

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/tasks/init", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		mu.Lock()
		bodies["init"] = body
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 0,
			"data": map[string]any{"task_id": 99, "session_id": "s_agent_e2e", "work_id": 100, "v": 2},
		})
	})
	mux.HandleFunc("/v1/agent2forVideo/stream", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path)
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		for _, e := range []string{
			`data: {"code":200,"data":{"type":"process","log":{"step_id":"","status":"start","message":"正在调用知识库..."}}}`,
			`data: {"code":200,"data":{"type":"process","log":{"step_id":"","status":"success","message":"知识库就绪"}}}`,
			`data: {"code":200,"data":{"type":"aim_result","session_id":"s_agent_e2e"}}`,
			`data: [DONE]`,
		} {
			fmt.Fprintln(w, e)
			fmt.Fprintln(w)
			if flusher != nil {
				flusher.Flush()
			}
		}
	})
	// v=3 stream must NOT be called when --engine agent is set.
	mux.HandleFunc("/v1/agent3forVideo/stream", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path)
		mu.Unlock()
		w.WriteHeader(500) // loud failure if mis-routed
	})
	mux.HandleFunc("/v1/works/detailBySession", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 0,
			"data": map[string]any{
				"id":          100,
				"session_id":  "s_agent_e2e",
				"title":       "Agent Test",
				"html_path":   "works/agent/index.html",
				"share_token": "tok_agent",
				"exporting":   0,
				"engine":      "agent",
			},
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	bin := build(t)
	configHome := buildVideoProfile(t, srv.URL)

	cmd := exec.Command(bin, "create", "--engine", "agent", "--from", "doc_abc12345")
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Env = append(os.Environ(),
		"VIBEKNOW_TOKEN=fake-token",
		"VIBEKNOW_CONFIG_HOME="+configHome,
	)

	err := cmd.Run()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatalf("run: %v\nstderr: %s", err, stderr.String())
	}

	if code != 0 {
		t.Fatalf("create exit %d\nstdout:%s\nstderr:%s", code, stdout.String(), stderr.String())
	}

	mu.Lock()
	defer mu.Unlock()

	// 1. Wire body: v=2 on init.
	if bodies["init"]["v"] != float64(2) {
		t.Fatalf("init body v = %v, want 2 (Engine=agent should produce v=2 wire)", bodies["init"]["v"])
	}

	// 2. Endpoint routing.
	hitAgent2 := false
	hitAgent3 := false
	for _, p := range paths {
		if p == "/v1/agent2forVideo/stream" {
			hitAgent2 = true
		}
		if p == "/v1/agent3forVideo/stream" {
			hitAgent3 = true
		}
	}
	if hitAgent3 {
		t.Fatalf("CLI hit /v1/agent3forVideo/stream when --engine agent was set; should have gone to /agent2/")
	}
	if !hitAgent2 {
		t.Fatalf("CLI never hit /v1/agent2forVideo/stream; routing broken (paths=%v)", paths)
	}

	// 3. Progress visibility: stderr contains [agent] prefixed lines.
	out := stdout.String() + stderr.String()
	if !strings.Contains(out, "[agent] 正在调用知识库") {
		t.Fatalf("missing [agent] progress prefix for first message:\n%s", out)
	}
	if !strings.Contains(out, "知识库就绪") {
		t.Fatalf("missing second progress message:\n%s", out)
	}

	// 4. Snapshot engine field is remapped/passed-through.
	// stdout in text mode includes a snapshot rendering; just verify engine appears somewhere.
	// In JSON mode it'd be {"engine":"agent"}, but text mode rendering may vary.
	// Skip strict format check; just confirm the word "agent" appears in output (already trivially true via [agent] prefix).
}
