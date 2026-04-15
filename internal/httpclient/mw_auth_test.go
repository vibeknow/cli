package httpclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

type staticTokenProvider string

func (s staticTokenProvider) Token(ctx context.Context) (string, error) { return string(s), nil }

func TestAuthMiddlewareInjectsBearer(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("Authorization")
	}))
	defer srv.Close()

	chain := Chain(http.DefaultTransport, AuthMiddleware{Provider: staticTokenProvider("tok_xyz")})
	c := New(srv.URL).WithTransport(chain)
	_ = c.Do(context.Background(), "GET", "/", nil, nil)
	if got != "Bearer tok_xyz" {
		t.Errorf("Authorization=%q", got)
	}
}

func TestTraceIDMiddlewareInjectsHeader(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("X-Trace-Id")
	}))
	defer srv.Close()

	chain := Chain(http.DefaultTransport, TraceIDMiddleware{})
	c := New(srv.URL).WithTransport(chain)
	_ = c.Do(context.Background(), "GET", "/", nil, nil)
	if got == "" {
		t.Error("X-Trace-Id missing")
	}
}
