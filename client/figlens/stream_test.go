package figlens_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/vibeknow/cli/client/figlens"
)

func TestStreamChat_ProcessEvents(t *testing.T) {
	sseBody := `data: {"code":200,"data":{"type":"process","log":{"step_id":"prepare","status":"start","message":"Starting parse"}}}

data: {"code":200,"data":{"type":"process","log":{"step_id":"prepare","status":"success","message":"Parse done"}}}

data: {"code":200,"data":{"type":"aim_result","session_id":"s_abc"}}

data: [DONE]

`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		fmt.Fprint(w, sseBody)
	}))
	defer srv.Close()

	c := figlens.New(srv.URL, staticToken("tok"))
	params := figlens.StreamParams{
		TaskID: 123, SessionID: "s_abc", Query: "test query",
		KnowledgeID: "kb_1", DocID: "doc_1", VoiceID: "v_1",
	}

	var events []figlens.StreamEvent
	err := c.StreamChat(context.Background(), params, func(ev figlens.StreamEvent) {
		events = append(events, ev)
	})
	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}
	if len(events) < 3 {
		t.Fatalf("expected >= 3 events, got %d", len(events))
	}
	if events[0].Type != "node.started" || events[0].Node != "prepare" {
		t.Fatalf("event[0] = %+v", events[0])
	}
	if events[1].Type != "node.succeeded" || events[1].Node != "prepare" {
		t.Fatalf("event[1] = %+v", events[1])
	}
	if events[2].Type != "task.succeeded" || events[2].SessionID != "s_abc" {
		t.Fatalf("event[2] = %+v", events[2])
	}
}

func TestStreamChat_ErrorEvent(t *testing.T) {
	sseBody := `data: {"code":200,"data":{"type":"error","message":"pipeline failed"}}

`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		fmt.Fprint(w, sseBody)
	}))
	defer srv.Close()

	c := figlens.New(srv.URL, staticToken("tok"))
	params := figlens.StreamParams{TaskID: 1, SessionID: "s_1", Query: "test"}

	var events []figlens.StreamEvent
	err := c.StreamChat(context.Background(), params, func(ev figlens.StreamEvent) {
		events = append(events, ev)
	})
	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}
	if len(events) == 0 || events[0].Type != "task.failed" {
		t.Fatalf("expected task.failed event, got %v", events)
	}
}

func TestStreamChat_SendsVideoKindAndAspect(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, `data: {"code":200,"data":{"type":"aim_result","session_id":"s_abc"}}

data: [DONE]

`)
	}))
	defer srv.Close()

	c := figlens.New(srv.URL, staticToken("tok"))
	err := c.StreamChat(context.Background(), figlens.StreamParams{
		TaskID: 1, SessionID: "s_abc", Query: "q",
		VideoKind: "replica", Aspect: "vertical", BGMEnabled: true,
	}, func(figlens.StreamEvent) {})
	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}
	if gotBody["video_kind"] != "replica" {
		t.Fatalf("video_kind = %v, want \"replica\"", gotBody["video_kind"])
	}
	if gotBody["aspect"] != "vertical" {
		t.Fatalf("aspect = %v, want \"vertical\"", gotBody["aspect"])
	}
	if gotBody["bgm_enabled"] != true {
		t.Fatalf("bgm_enabled = %v, want true", gotBody["bgm_enabled"])
	}
}

func TestStreamChat_ScriptInvalidCode(t *testing.T) {
	sseBody := `data: {"code":100004,"data":{"message":"讲稿超过 8000 字"}}

`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseBody)
	}))
	defer srv.Close()

	c := figlens.New(srv.URL, staticToken("tok"))
	var events []figlens.StreamEvent
	err := c.StreamChat(context.Background(), figlens.StreamParams{TaskID: 1, SessionID: "s"}, func(ev figlens.StreamEvent) {
		events = append(events, ev)
	})
	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}
	if len(events) == 0 || events[0].Type != "task.failed" {
		t.Fatalf("expected task.failed, got %v", events)
	}
	if events[0].Code != "script_invalid" {
		t.Fatalf("expected Code=script_invalid, got %q", events[0].Code)
	}
	if events[0].Message != "讲稿超过 8000 字" {
		t.Fatalf("expected backend message verbatim, got %q", events[0].Message)
	}
	if events[0].Retryable {
		t.Fatalf("script_invalid must not be retryable (it's a permanent input error)")
	}
}

// concurrent_work_limit is the canonical transient business code: same
// request can succeed once the user's other tasks finish, so the CLI must
// surface Retryable=true here.
func TestStreamChat_ConcurrentLimitIsRetryable(t *testing.T) {
	sseBody := `data: {"code":100003,"data":{"message":"too many concurrent works"}}

`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseBody)
	}))
	defer srv.Close()

	c := figlens.New(srv.URL, staticToken("tok"))
	var events []figlens.StreamEvent
	err := c.StreamChat(context.Background(), figlens.StreamParams{TaskID: 1, SessionID: "s"}, func(ev figlens.StreamEvent) {
		events = append(events, ev)
	})
	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}
	if len(events) == 0 || events[0].Code != "concurrent_work_limit" {
		t.Fatalf("expected Code=concurrent_work_limit, got %+v", events)
	}
	if !events[0].Retryable {
		t.Fatalf("concurrent_work_limit must be retryable")
	}
}

// Plain `error` SSE (no envelope code) is the v=2 agent failure shape.
// Backend sends no retryable flag and no code, so the CLI must default
// Retryable=false rather than guess true.
func TestStreamChat_PlainErrorEventIsNotRetryable(t *testing.T) {
	sseBody := `data: {"code":200,"data":{"type":"error","message":"agent crashed"}}

`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseBody)
	}))
	defer srv.Close()

	c := figlens.New(srv.URL, staticToken("tok"))
	var events []figlens.StreamEvent
	err := c.StreamChat(context.Background(), figlens.StreamParams{TaskID: 1, SessionID: "s"}, func(ev figlens.StreamEvent) {
		events = append(events, ev)
	})
	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}
	if len(events) == 0 || events[0].Type != "task.failed" {
		t.Fatalf("expected task.failed, got %v", events)
	}
	if events[0].Retryable {
		t.Fatalf("plain error SSE has no code — must not be marked retryable")
	}
}

// aim_result on v=3 pipeline: video URL in `html_path`, duration_ms in
// the metadata bag. Both must surface on the task.succeeded event so
// agent consumers can act on the result.
func TestStreamChat_AimResultV3HasVideoURLAndDuration(t *testing.T) {
	sseBody := `data: {"code":200,"data":{"type":"aim_result","session_id":"s_v3","html_path":"https://cdn.example.com/v/s_v3.html","data":{"duration_ms":42500,"fps":30}}}

data: [DONE]

`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseBody)
	}))
	defer srv.Close()

	c := figlens.New(srv.URL, staticToken("tok"))
	var events []figlens.StreamEvent
	err := c.StreamChat(context.Background(), figlens.StreamParams{TaskID: 1, SessionID: "s_v3"}, func(ev figlens.StreamEvent) {
		events = append(events, ev)
	})
	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}
	if len(events) == 0 || events[0].Type != "task.succeeded" {
		t.Fatalf("expected task.succeeded, got %+v", events)
	}
	if events[0].VideoURL != "https://cdn.example.com/v/s_v3.html" {
		t.Fatalf("VideoURL = %q, want html_path value", events[0].VideoURL)
	}
	if events[0].DurationMs != 42500 {
		t.Fatalf("DurationMs = %d, want 42500", events[0].DurationMs)
	}
}

// aim_result on v=2 agent: video URL lives in `text`; no duration_ms.
// Falling back to text — and not promising a duration we don't have —
// is the contract this case pins.
func TestStreamChat_AimResultV2UsesTextForVideoURL(t *testing.T) {
	sseBody := `data: {"code":200,"data":{"type":"aim_result","session_id":"s_v2","text":"https://cdn.example.com/v/s_v2.html"}}

data: [DONE]

`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseBody)
	}))
	defer srv.Close()

	c := figlens.New(srv.URL, staticToken("tok"))
	var events []figlens.StreamEvent
	err := c.StreamChat(context.Background(), figlens.StreamParams{TaskID: 1, SessionID: "s_v2"}, func(ev figlens.StreamEvent) {
		events = append(events, ev)
	})
	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}
	if len(events) == 0 || events[0].VideoURL != "https://cdn.example.com/v/s_v2.html" {
		t.Fatalf("expected VideoURL from text field, got %+v", events)
	}
	if events[0].DurationMs != 0 {
		t.Fatalf("v=2 has no duration_ms — DurationMs must be 0, got %d", events[0].DurationMs)
	}
}

func TestStreamChat_AgentEngineUsesAgent2Path(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"code\":200,\"data\":{\"type\":\"aim_result\",\"session_id\":\"s\"}}\n\ndata: [DONE]\n\n")
	}))
	defer srv.Close()

	c := figlens.New(srv.URL, staticToken("tok"))
	err := c.StreamChat(context.Background(), figlens.StreamParams{
		TaskID: 1, SessionID: "s", Query: "q", Engine: figlens.EngineAgent,
	}, func(figlens.StreamEvent) {})
	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}
	if gotPath != "/v1/agent2forVideo/stream" {
		t.Fatalf("path = %q, want /v1/agent2forVideo/stream", gotPath)
	}
}

func TestStreamChat_AgentProgressEvents(t *testing.T) {
	// v=2 events have empty step_id and a human-readable message.
	sseBody := `data: {"code":200,"data":{"type":"process","log":{"step_id":"","status":"start","message":"正在调用知识库..."}}}

data: {"code":200,"data":{"type":"process","log":{"step_id":"","status":"success","message":"知识库就绪"}}}

data: {"code":200,"data":{"type":"aim_result","session_id":"s_agent"}}

data: [DONE]

`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseBody)
	}))
	defer srv.Close()

	c := figlens.New(srv.URL, staticToken("tok"))
	var events []figlens.StreamEvent
	err := c.StreamChat(context.Background(), figlens.StreamParams{
		TaskID: 1, SessionID: "s_agent", Engine: figlens.EngineAgent,
	}, func(ev figlens.StreamEvent) {
		events = append(events, ev)
	})
	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("len(events) = %d, want 3; events=%+v", len(events), events)
	}
	if events[0].Type != "node.progress" || events[0].Status != "start" || events[0].Message != "正在调用知识库..." {
		t.Fatalf("event[0] = %+v", events[0])
	}
	if events[1].Type != "node.progress" || events[1].Status != "success" || events[1].Message != "知识库就绪" {
		t.Fatalf("event[1] = %+v", events[1])
	}
	if events[2].Type != "task.succeeded" {
		t.Fatalf("event[2] = %+v", events[2])
	}
}
