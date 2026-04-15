package httpclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestVersionMatchOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Vibeknow-Api-Version", "v1")
		w.WriteHeader(200)
	}))
	defer srv.Close()

	chain := Chain(http.DefaultTransport, VersionMiddleware{Expected: "v1"})
	c := New(srv.URL).WithTransport(chain)
	if err := c.Do(context.Background(), "GET", "/", nil, nil); err != nil {
		t.Fatal(err)
	}
}

func TestVersionMismatchErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Vibeknow-Api-Version", "v2")
		w.WriteHeader(200)
	}))
	defer srv.Close()

	chain := Chain(http.DefaultTransport, VersionMiddleware{Expected: "v1"})
	c := New(srv.URL).WithTransport(chain)
	err := c.Do(context.Background(), "GET", "/", nil, nil)
	if err == nil {
		t.Fatal("expected version mismatch")
	}
	eo, ok := err.(*errObject)
	if !ok || eo.Code != "version_mismatch" {
		t.Errorf("wrong error: %+v", err)
	}
}

func TestVersionMissingHeaderIsWarningOnly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	chain := Chain(http.DefaultTransport, VersionMiddleware{Expected: "v1"})
	c := New(srv.URL).WithTransport(chain)
	if err := c.Do(context.Background(), "GET", "/", nil, nil); err != nil {
		t.Errorf("missing header should not fail: %v", err)
	}
}
