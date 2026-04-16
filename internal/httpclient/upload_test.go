package httpclient_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/shiliu-ai/vibeknow-cli/internal/httpclient"
)

func TestDoUpload(t *testing.T) {
	var gotContentType string
	var gotBody string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			t.Fatalf("parse multipart: %v", err)
		}
		f, _, err := r.FormFile("file")
		if err != nil {
			t.Fatalf("form file: %v", err)
		}
		defer f.Close()
		data, _ := io.ReadAll(f)
		gotBody = string(data)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"doc_abc","status":"processing"}`))
	}))
	defer srv.Close()

	c := httpclient.New(srv.URL)

	var out struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	err := c.DoUpload(context.Background(), "/upload", "file", "test.pdf",
		strings.NewReader("fake-pdf-content"), &out)
	if err != nil {
		t.Fatalf("DoUpload: %v", err)
	}
	if !strings.HasPrefix(gotContentType, "multipart/form-data") {
		t.Fatalf("content-type = %q, want multipart/form-data", gotContentType)
	}
	if gotBody != "fake-pdf-content" {
		t.Fatalf("file body = %q", gotBody)
	}
	if out.ID != "doc_abc" {
		t.Fatalf("response id = %q", out.ID)
	}
}
