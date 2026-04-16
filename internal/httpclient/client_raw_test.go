package httpclient_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/vibeknow/cli/internal/httpclient"
)

func TestDoRaw_ReturnsOpenBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		w.Write([]byte("data: hello\n\n"))
	}))
	defer srv.Close()

	c := httpclient.New(srv.URL)
	resp, err := c.DoRaw(context.Background(), "POST", "/stream", map[string]any{"q": "test"})
	if err != nil {
		t.Fatalf("DoRaw: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "data: hello\n\n" {
		t.Fatalf("body = %q", body)
	}
}

func TestDoRaw_Returns4xxAsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(401)
		w.Write([]byte(`{"code":40101,"message":"unauthorized"}`))
	}))
	defer srv.Close()

	c := httpclient.New(srv.URL)
	_, err := c.DoRaw(context.Background(), "POST", "/stream", nil)
	if err == nil {
		t.Fatal("expected error for 401")
	}
}
