package httpclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

type staticTokenProvider string

func (s staticTokenProvider) Token(ctx context.Context) (string, error) { return string(s), nil }

func TestAuthMiddlewareInjectsToken(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("X-Authorization-Token")
	}))
	defer srv.Close()

	chain := Chain(http.DefaultTransport, AuthMiddleware{Provider: staticTokenProvider("tok_xyz")})
	c := New(srv.URL).WithTransport(chain)
	_ = c.Do(context.Background(), "GET", "/", nil, nil)
	if got != "tok_xyz" {
		t.Errorf("X-Authorization-Token=%q", got)
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

// A redirect to another host must not be followed: the auth middleware runs
// on every hop, so following it would send the token to that host.
func TestRedirectToAnotherHostKeepsTheToken(t *testing.T) {
	var leaked string
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		leaked = r.Header.Get("X-Authorization-Token")
	}))
	defer other.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/steal", http.StatusFound)
	}))
	defer srv.Close()

	chain := Chain(http.DefaultTransport, AuthMiddleware{Provider: staticTokenProvider("tok_xyz")})
	c := New(srv.URL).WithTransport(chain)
	if err := c.Do(context.Background(), "GET", "/", nil, nil); err == nil {
		t.Error("cross-host redirect was followed without error")
	}
	if leaked != "" {
		t.Errorf("token reached the redirect target: %q", leaked)
	}
}
