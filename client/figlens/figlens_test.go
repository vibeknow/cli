package figlens_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/tasks/init" || r.Method != "POST" {
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
		figlensResp(w, map[string]any{
			"task_id": 123, "session_id": "s_abc", "work_id": 456, "v": 3,
		})
	}))
	defer srv.Close()

	c := figlens.New(srv.URL, staticToken("tok"))
	task, err := c.InitTask(context.Background())
	if err != nil {
		t.Fatalf("InitTask: %v", err)
	}
	if task.TaskID != 123 {
		t.Fatalf("task_id = %d", task.TaskID)
	}
	if task.SessionID != "s_abc" {
		t.Fatalf("session_id = %q", task.SessionID)
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
