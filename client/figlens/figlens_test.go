package figlens_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vibeknow/cli/client/figlens"
)

type staticToken string

func (s staticToken) Token(ctx context.Context) (string, error) { return string(s), nil }

func figlensResp(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": data})
}

func TestInitTask(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/tasks/init" || r.Method != "POST" {
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		figlensResp(w, map[string]any{
			"task_id": 123, "session_id": "s_abc", "work_id": 456, "v": 3,
		})
	}))
	defer srv.Close()

	c := figlens.New(srv.URL, staticToken("tok"))
	task, err := c.InitTask(context.Background(), figlens.InitTaskParams{
		KnowledgeID: "kb_1", DocID: "doc_1", VideoKind: "script_lock",
	})
	if err != nil {
		t.Fatalf("InitTask: %v", err)
	}
	if task.TaskID != 123 {
		t.Fatalf("task_id = %d", task.TaskID)
	}
	if gotBody["v"] != float64(3) {
		t.Fatalf("v = %v, want 3", gotBody["v"])
	}
	if gotBody["knowledge_id"] != "kb_1" {
		t.Fatalf("knowledge_id = %v", gotBody["knowledge_id"])
	}
	if gotBody["video_kind"] != "script_lock" {
		t.Fatalf("video_kind = %v", gotBody["video_kind"])
	}
}

func TestInitTask_OmitsEmptyFields(t *testing.T) {
	var raw []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ = io.ReadAll(r.Body)
		figlensResp(w, map[string]any{
			"task_id": 1, "session_id": "s_x", "work_id": 2, "v": 3,
		})
	}))
	defer srv.Close()

	c := figlens.New(srv.URL, staticToken("tok"))
	_, err := c.InitTask(context.Background(), figlens.InitTaskParams{})
	if err != nil {
		t.Fatalf("InitTask: %v", err)
	}
	body := string(raw)
	for _, f := range []string{"knowledge_id", "doc_id", "video_kind"} {
		if strings.Contains(body, f) {
			t.Fatalf("%s unexpectedly present in empty-params body: %s", f, body)
		}
	}
}

func TestGetWorkBySession(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("session_id") != "s_abc" {
			t.Fatalf("unexpected session_id query param")
		}
		figlensResp(w, map[string]any{
			"id": 456, "session_id": "s_abc", "title": "Test Video",
			"html_path": "works/foo/index.html",
			"video_path": "/videos/test.mp4",
			"cover_url": "https://cover.jpg",
			"share_token": "tok_xyz", "exporting": 1,
			"duration": 120,
			"engine":   "suite",
		})
	}))
	defer srv.Close()

	c := figlens.New(srv.URL, staticToken("tok"))
	work, err := c.GetWorkBySession(context.Background(), "s_abc")
	if err != nil {
		t.Fatalf("GetWorkBySession: %v", err)
	}
	if work.ID != 456 || work.Duration != 120 {
		t.Fatalf("work = %+v", work)
	}
	if work.SessionID != "s_abc" {
		t.Fatalf("session_id = %q", work.SessionID)
	}
	if work.HtmlPath != "works/foo/index.html" {
		t.Fatalf("html_path = %q", work.HtmlPath)
	}
	if work.ShareToken != "tok_xyz" {
		t.Fatalf("share_token = %q", work.ShareToken)
	}
	if work.Exporting != 1 {
		t.Fatalf("exporting = %d", work.Exporting)
	}
	if work.Engine != "suite" {
		t.Fatalf("engine = %q, want \"suite\"", work.Engine)
	}
}

func TestExportVideo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		figlensResp(w, map[string]any{"task_id": 424242})
	}))
	defer srv.Close()

	c := figlens.New(srv.URL, staticToken("tok"))
	exportID, err := c.ExportVideo(context.Background(), "s_abc")
	if err != nil {
		t.Fatalf("ExportVideo: %v", err)
	}
	if exportID != 424242 {
		t.Fatalf("export_id = %d", exportID)
	}
}

func TestGetExportResult(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		figlensResp(w, map[string]any{
			"status": "running", "progress": 42, "progress_msg": "rendering frames",
			"video_path": "", "error": "",
		})
	}))
	defer srv.Close()

	c := figlens.New(srv.URL, staticToken("tok"))
	result, err := c.GetExportResult(context.Background(), 424242)
	if err != nil {
		t.Fatalf("GetExportResult: %v", err)
	}
	if result.Status != "running" || result.Progress != 42 {
		t.Fatalf("result = %+v", result)
	}
	if result.ProgressMsg != "rendering frames" {
		t.Fatalf("progress_msg = %q", result.ProgressMsg)
	}
}

func TestSignedURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		figlensResp(w, map[string]any{"url": "https://signed.example.com/video.mp4"})
	}))
	defer srv.Close()

	c := figlens.New(srv.URL, staticToken("tok"))
	u, err := c.SignedURL(context.Background(), "/videos/test.mp4")
	if err != nil {
		t.Fatalf("SignedURL: %v", err)
	}
	if u != "https://signed.example.com/video.mp4" {
		t.Fatalf("url = %q", u)
	}
}

func TestFastQueryOptimize_SendsVideoKind(t *testing.T) {
	var gotBody map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, `data: {"code":200,"data":{"type":"aim_result","answer_done":{"text":"ok"}}}

data: [DONE]

`)
	}))
	defer srv.Close()

	c := figlens.New(srv.URL, staticToken("tok"))
	_, err := c.FastQueryOptimize(context.Background(), figlens.OptimizeParams{
		KnowledgeID: "kb_1", DocID: "doc_1", VideoKind: "script_lock",
	}, nil)
	if err != nil {
		t.Fatalf("FastQueryOptimize: %v", err)
	}
	if gotBody["video_kind"] != "script_lock" {
		t.Fatalf("video_kind on wire = %q, want %q", gotBody["video_kind"], "script_lock")
	}
}

func TestFastQueryOptimize_OmitsVideoKindWhenEmpty(t *testing.T) {
	var raw []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, `data: {"code":200,"data":{"type":"aim_result","answer_done":{"text":"ok"}}}

data: [DONE]

`)
	}))
	defer srv.Close()

	c := figlens.New(srv.URL, staticToken("tok"))
	_, _ = c.FastQueryOptimize(context.Background(), figlens.OptimizeParams{
		KnowledgeID: "kb_1", DocID: "doc_1",
	}, nil)
	if strings.Contains(string(raw), "video_kind") {
		t.Fatalf("video_kind unexpectedly present in wire body: %s", raw)
	}
}

func TestInitTask_AgentEngineSendsV2(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		figlensResp(w, map[string]any{
			"task_id": 1, "session_id": "s_x", "work_id": 2, "v": 2,
		})
	}))
	defer srv.Close()

	c := figlens.New(srv.URL, staticToken("tok"))
	_, err := c.InitTask(context.Background(), figlens.InitTaskParams{
		Engine: figlens.EngineAgent,
	})
	if err != nil {
		t.Fatalf("InitTask: %v", err)
	}
	if gotBody["v"] != float64(2) {
		t.Fatalf("v on wire = %v, want 2", gotBody["v"])
	}
}

func TestExtractDocImages(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/task/extractDocImages" {
			t.Errorf("path = %s", r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"code":0,"data":{"images":[{"image_index":1,"url":"https://cdn/x.png","description":"图表A","type":"image","context":"第5页"},{"image_index":2,"url":"https://cdn/y.png","description":"","type":"image"}]}}`)
	}))
	defer srv.Close()

	c := figlens.New(srv.URL, staticToken("tok"))
	images, err := c.ExtractDocImages(context.Background(), "kb_1", "doc_1", "")
	if err != nil {
		t.Fatalf("ExtractDocImages: %v", err)
	}
	if gotBody["knowledge_id"] != "kb_1" || gotBody["doc_id"] != "doc_1" {
		t.Fatalf("request body = %v", gotBody)
	}
	if len(images) != 2 || images[0].ImageIndex != 1 || images[0].Description != "图表A" {
		t.Fatalf("images = %+v", images)
	}
}

// TestInitTask_SendsScriptLock guards the preflight, not the render: the
// backend's script-quality check (length, character set, LLM suitability)
// is gated on this field alone. Omit it and an unusable script sails past
// init and only fails after a full billed pipeline run.
func TestInitTask_SendsScriptLock(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"code":0,"data":{"task_id":1,"session_id":"s","work_id":2,"v":3}}`)
	}))
	defer srv.Close()

	c := figlens.New(srv.URL, staticToken("tok"))
	_, err := c.InitTask(context.Background(), figlens.InitTaskParams{
		Engine: figlens.EnginePipeline, ScriptLock: true,
		KnowledgeID: "kb_1", DocID: "doc_1",
	})
	if err != nil {
		t.Fatalf("InitTask: %v", err)
	}
	if gotBody["script_lock"] != true {
		t.Errorf("script_lock on wire = %v, want true", gotBody["script_lock"])
	}
	// 原稿锁定 on the freeform line means no video_kind at all; the
	// preflight still needs the kb/doc pair to have travelled.
	if _, present := gotBody["video_kind"]; present {
		t.Errorf("video_kind should be absent, got %v", gotBody["video_kind"])
	}
	if gotBody["knowledge_id"] != "kb_1" || gotBody["doc_id"] != "doc_1" {
		t.Errorf("kb/doc pair missing from body: %v", gotBody)
	}
}

func TestInitTask_OmitsScriptLockWhenOff(t *testing.T) {
	var raw []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"code":0,"data":{"task_id":1,"session_id":"s","work_id":2,"v":3}}`)
	}))
	defer srv.Close()

	c := figlens.New(srv.URL, staticToken("tok"))
	_, _ = c.InitTask(context.Background(), figlens.InitTaskParams{
		Engine: figlens.EnginePipeline, KnowledgeID: "kb_1", DocID: "doc_1",
	})
	if strings.Contains(string(raw), "script_lock") {
		t.Fatalf("script_lock unexpectedly present in wire body: %s", raw)
	}
}

func TestInitTask_SendsSelectedImageIndexes(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"code":0,"data":{"task_id":1,"session_id":"s","work_id":2,"v":3}}`)
	}))
	defer srv.Close()

	c := figlens.New(srv.URL, staticToken("tok"))
	_, err := c.InitTask(context.Background(), figlens.InitTaskParams{
		Engine: figlens.EnginePipeline, VideoKind: figlens.VideoKindImage2,
		KnowledgeID: "kb_1", DocID: "doc_1", SelectedImageIndexes: []int{2, 4},
	})
	if err != nil {
		t.Fatalf("InitTask: %v", err)
	}
	if gotBody["v"] != float64(3) || gotBody["video_kind"] != "image2" {
		t.Fatalf("body = %v", gotBody)
	}
	idx, _ := gotBody["selected_image_indexes"].([]any)
	if len(idx) != 2 || idx[0] != float64(2) {
		t.Fatalf("selected_image_indexes = %v", gotBody["selected_image_indexes"])
	}
}
