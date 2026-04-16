package vectoria_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vibeknow/cli/client/vectoria"
)

func TestCreateKB(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/v1/knowledgebases" {
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("X-API-Key") != "test-key" {
			t.Fatalf("missing X-API-Key header")
		}
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		if body["name"] == "" {
			t.Fatal("expected name in body")
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"id": "kb_abc123"})
	}))
	defer srv.Close()

	c := vectoria.New(srv.URL, "test-key")
	id, err := c.CreateKB(context.Background(), "test-kb")
	if err != nil {
		t.Fatalf("CreateKB: %v", err)
	}
	if id != "kb_abc123" {
		t.Fatalf("kb_id = %q, want kb_abc123", id)
	}
}

func TestUploadDoc(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Fatalf("unexpected method %s", r.Method)
		}
		if !strings.HasPrefix(r.URL.Path, "/v1/knowledgebases/kb_1/documents/file") {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if r.Header.Get("X-API-Key") != "test-key" {
			t.Fatalf("missing X-API-Key header")
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"id": "doc_xyz", "status": "processing"})
	}))
	defer srv.Close()

	c := vectoria.New(srv.URL, "test-key")
	doc, err := c.UploadDoc(context.Background(), "kb_1", "test.pdf", strings.NewReader("pdf-data"))
	if err != nil {
		t.Fatalf("UploadDoc: %v", err)
	}
	if doc.ID != "doc_xyz" {
		t.Fatalf("doc_id = %q", doc.ID)
	}
}

func TestUploadURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/v1/knowledgebases/kb_1/documents/url") {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		if body["url"] != "https://example.com" {
			t.Fatalf("unexpected url %q", body["url"])
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"id": "doc_url1", "status": "processing"})
	}))
	defer srv.Close()

	c := vectoria.New(srv.URL, "test-key")
	doc, err := c.UploadURL(context.Background(), "kb_1", "https://example.com")
	if err != nil {
		t.Fatalf("UploadURL: %v", err)
	}
	if doc.ID != "doc_url1" {
		t.Fatalf("doc_id = %q", doc.ID)
	}
}

func TestGetDocStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"id": "doc_1", "status": "completed"})
	}))
	defer srv.Close()

	c := vectoria.New(srv.URL, "test-key")
	doc, err := c.GetDocStatus(context.Background(), "kb_1", "doc_1")
	if err != nil {
		t.Fatalf("GetDocStatus: %v", err)
	}
	if doc.Status != "completed" {
		t.Fatalf("status = %q", doc.Status)
	}
}

func TestDeleteDoc(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "DELETE" {
			t.Fatalf("unexpected method %s", r.Method)
		}
		w.WriteHeader(204)
	}))
	defer srv.Close()

	c := vectoria.New(srv.URL, "test-key")
	err := c.DeleteDoc(context.Background(), "kb_1", "doc_1")
	if err != nil {
		t.Fatalf("DeleteDoc: %v", err)
	}
}

func TestUploadDoc_FileContent(t *testing.T) {
	var gotContent string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseMultipartForm(32 << 20)
		f, _, _ := r.FormFile("file")
		if f != nil {
			data, _ := io.ReadAll(f)
			gotContent = string(data)
			f.Close()
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"id": "doc_1", "status": "processing"})
	}))
	defer srv.Close()

	c := vectoria.New(srv.URL, "test-key")
	c.UploadDoc(context.Background(), "kb_1", "test.txt", strings.NewReader("hello world"))
	if gotContent != "hello world" {
		t.Fatalf("file content = %q, want 'hello world'", gotContent)
	}
}
