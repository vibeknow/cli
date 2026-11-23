package httpclient

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vibeknow/cli/internal/clerr"
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

type failingProvider struct{ err error }

func (f failingProvider) Token(context.Context) (string, error) { return "", f.err }

// A provider that already built a *clerr.Error has said what the user should
// do; the middleware must not swap its hint for "log in again".
func TestAuthMiddlewareKeepsTheProvidersOwnError(t *testing.T) {
	want := clerr.Auth("store is read-only").WithHint("run it outside the sandbox")
	chain := Chain(http.DefaultTransport, AuthMiddleware{Provider: failingProvider{err: want}})
	_, err := chain.RoundTrip(httptest.NewRequest("GET", "http://example.invalid/", nil))
	var ce *clerr.Error
	if !errors.As(err, &ce) || ce.Hint != want.Hint {
		t.Fatalf("err = %#v, want the provider's error with its hint", err)
	}
}

// Any other failure still becomes an auth error pointing at login.
func TestAuthMiddlewareWrapsPlainErrors(t *testing.T) {
	chain := Chain(http.DefaultTransport, AuthMiddleware{Provider: failingProvider{err: errors.New("no credential")}})
	_, err := chain.RoundTrip(httptest.NewRequest("GET", "http://example.invalid/", nil))
	var ce *clerr.Error
	if !errors.As(err, &ce) || ce.Code != clerr.ExitAuth || !strings.Contains(ce.Hint, "auth login") {
		t.Fatalf("err = %#v, want an auth error pointing at login", err)
	}
}
