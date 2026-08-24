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

// TestStreamChat_SendsScriptLockAlongsideVideoKind pins the split that
// 原稿锁定 went through: it is a boolean carried *next to* video_kind, not a
// video_kind value. The pipeline entry dispatches the graph on video_kind
// and reads the lock off this boolean; a request that spells the lock as
// video_kind="script_lock" matches no graph, falls through to the standard
// line with the lock off, and silently rewrites the user's script.
func TestStreamChat_SendsScriptLockAlongsideVideoKind(t *testing.T) {
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
		VideoKind: figlens.VideoKindImage2, ScriptLock: true,
	}, func(figlens.StreamEvent) {})
	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}
	if gotBody["script_lock"] != true {
		t.Errorf("script_lock = %v, want true", gotBody["script_lock"])
	}
	// The two axes must both survive: locking the script does not change
	// which line renders it.
	if gotBody["video_kind"] != "image2" {
		t.Errorf("video_kind = %v, want \"image2\"", gotBody["video_kind"])
	}
}

func TestStreamChat_SendsHandDrawVideoKind(t *testing.T) {
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
		VideoKind: figlens.VideoKindHandDraw,
	}, func(figlens.StreamEvent) {})
	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}
	// Hyphen, not underscore — the backend compares the literal string.
	if gotBody["video_kind"] != "hand-draw" {
		t.Fatalf("video_kind = %v, want \"hand-draw\"", gotBody["video_kind"])
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

// Current go-figlens (since the 2026-05/06 pipeline rework) nests the
// aim_result payload under `answer_done`: the playable URL lives at
// answer_done.html_path (v=3) and the metadata bag at answer_done.data.
// The flat top-level html_path/text/data shape is the legacy contract;
// both must parse.
func TestStreamChat_AimResultNestedAnswerDone(t *testing.T) {
	sseBody := `data: {"code":200,"data":{"type":"aim_result","session_id":"s_v3","answer_done":{"text":"视频已生成","html_path":"https://cdn.example.com/works/s_v3/index.html","data":{"duration_ms":30000,"watermark":true}}}}

`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseBody)
	}))
	defer srv.Close()

	c := figlens.New(srv.URL, staticToken("tok"))
	var events []figlens.StreamEvent
	err := c.StreamChat(context.Background(), figlens.StreamParams{TaskID: 1, SessionID: "s_v3", Query: "q"}, func(ev figlens.StreamEvent) {
		events = append(events, ev)
	})
	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}
	if len(events) == 0 || events[0].Type != "task.succeeded" {
		t.Fatalf("expected task.succeeded, got %v", events)
	}
	if events[0].VideoURL != "https://cdn.example.com/works/s_v3/index.html" {
		t.Fatalf("VideoURL = %q, want nested answer_done.html_path", events[0].VideoURL)
	}
	if events[0].DurationMs != 30000 {
		t.Fatalf("DurationMs = %d, want 30000 from answer_done.data", events[0].DurationMs)
	}
}

// v=2 agent puts the playable URL in answer_done.text (it has no html_path).
func TestStreamChat_AimResultNestedAgentTextURL(t *testing.T) {
	sseBody := `data: {"code":200,"data":{"type":"aim_result","session_id":"s_v2","answer_done":{"text":"https://cdn.example.com/works/s_v2/index.html"}}}

`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseBody)
	}))
	defer srv.Close()

	c := figlens.New(srv.URL, staticToken("tok"))
	var events []figlens.StreamEvent
	err := c.StreamChat(context.Background(), figlens.StreamParams{TaskID: 1, SessionID: "s_v2", Query: "q"}, func(ev figlens.StreamEvent) {
		events = append(events, ev)
	})
	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}
	if len(events) == 0 || events[0].VideoURL != "https://cdn.example.com/works/s_v2/index.html" {
		t.Fatalf("expected VideoURL from answer_done.text, got %+v", events)
	}
}

// v=3 answer_done.text is a human-readable completion message, not a URL.
// When html_path is absent the CLI must leave VideoURL empty rather than
// leak the message into a URL field.
func TestStreamChat_AimResultNestedTextNotURL(t *testing.T) {
	sseBody := `data: {"code":200,"data":{"type":"aim_result","session_id":"s_v3","answer_done":{"text":"视频已生成"}}}

`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseBody)
	}))
	defer srv.Close()

	c := figlens.New(srv.URL, staticToken("tok"))
	var events []figlens.StreamEvent
	err := c.StreamChat(context.Background(), figlens.StreamParams{TaskID: 1, SessionID: "s_v3", Query: "q"}, func(ev figlens.StreamEvent) {
		events = append(events, ev)
	})
	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}
	if len(events) == 0 || events[0].Type != "task.succeeded" {
		t.Fatalf("expected task.succeeded, got %v", events)
	}
	if events[0].VideoURL != "" {
		t.Fatalf("VideoURL = %q, want empty (text is a message, not a URL)", events[0].VideoURL)
	}
}

// Current backend nests failure details under `error`: {"type":"error",
// "error":{"message":"..."}}. The CLI must surface error.message instead
// of dumping the raw JSON payload.
func TestStreamChat_ErrorEventNestedMessage(t *testing.T) {
	sseBody := `data: {"code":200,"data":{"type":"error","session_id":"s_1","error":{"message":"分镜生成失败"}}}

`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseBody)
	}))
	defer srv.Close()

	c := figlens.New(srv.URL, staticToken("tok"))
	var events []figlens.StreamEvent
	err := c.StreamChat(context.Background(), figlens.StreamParams{TaskID: 1, SessionID: "s_1"}, func(ev figlens.StreamEvent) {
		events = append(events, ev)
	})
	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}
	if len(events) == 0 || events[0].Type != "task.failed" {
		t.Fatalf("expected task.failed, got %v", events)
	}
	if events[0].Message != "分镜生成失败" {
		t.Fatalf("Message = %q, want nested error.message", events[0].Message)
	}
}

// image2 degrades per-page instead of failing the task (placeholder image,
// style fallback). The backend reports these as process logs with
// status="warning"; the CLI must surface them as node.warning rather than
// dropping them on the floor.
func TestStreamChat_WarningProcessEvent(t *testing.T) {
	sseBody := `data: {"code":200,"data":{"type":"process","session_id":"s_w","log":{"step_id":"image2_gen","status":"warning","message":"第5页配图失败，已使用占位图"}}}

`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseBody)
	}))
	defer srv.Close()

	c := figlens.New(srv.URL, staticToken("tok"))
	var events []figlens.StreamEvent
	err := c.StreamChat(context.Background(), figlens.StreamParams{TaskID: 1, SessionID: "s_w", Query: "q"}, func(ev figlens.StreamEvent) {
		events = append(events, ev)
	})
	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}
	if len(events) == 0 || events[0].Type != "node.warning" {
		t.Fatalf("expected node.warning, got %v", events)
	}
	// Display name is sanitized: the wire step_id carries an internal
	// model codename that must not surface in user-facing output.
	if events[0].Node != "image_gen" || events[0].Message == "" {
		t.Fatalf("node.warning lost node/message or leaked codename: %+v", events[0])
	}
}

// Pins the image2 request additions: page_count and selected_image_indexes
// must reach the wire body (and be omitted when zero-valued).
func TestStreamChat_SendsImage2Params(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	c := figlens.New(srv.URL, staticToken("tok"))
	err := c.StreamChat(context.Background(), figlens.StreamParams{
		TaskID: 1, SessionID: "s", Query: "q",
		VideoKind: figlens.VideoKindImage2, PageCount: 8,
		SelectedImageIndexes: []int{1, 3},
	}, func(figlens.StreamEvent) {})
	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}
	if gotBody["video_kind"] != "image2" {
		t.Fatalf("video_kind = %v, want image2", gotBody["video_kind"])
	}
	if gotBody["page_count"] != float64(8) {
		t.Fatalf("page_count = %v, want 8", gotBody["page_count"])
	}
	idx, _ := gotBody["selected_image_indexes"].([]any)
	if len(idx) != 2 || idx[0] != float64(1) || idx[1] != float64(3) {
		t.Fatalf("selected_image_indexes = %v, want [1 3]", gotBody["selected_image_indexes"])
	}
}
