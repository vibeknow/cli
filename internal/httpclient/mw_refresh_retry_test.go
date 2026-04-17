package httpclient

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

type mockRefreshableProvider struct {
	token        string
	tokenType    string
	refreshCalls int32
	refreshToken string
}

func (m *mockRefreshableProvider) Token(_ context.Context) (string, error) {
	return m.token, nil
}

func (m *mockRefreshableProvider) TokenType() string {
	return m.tokenType
}

func (m *mockRefreshableProvider) ForceRefresh(_ context.Context) (string, error) {
	atomic.AddInt32(&m.refreshCalls, 1)
	return m.refreshToken, nil
}

// plainProvider implements only TokenProvider (not RefreshableTokenProvider).
type plainProvider struct{}

func (p *plainProvider) Token(_ context.Context) (string, error) { return "plain-token", nil }

func TestRefreshRetryMiddleware_401_OAuth(t *testing.T) {
	provider := &mockRefreshableProvider{
		token:        "old-token",
		tokenType:    "oauth",
		refreshToken: "new-token",
	}

	callCount := 0
	backend := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		callCount++
		if callCount == 1 {
			return &http.Response{
				StatusCode: http.StatusUnauthorized,
				Body:       io.NopCloser(strings.NewReader("")),
				Header:     make(http.Header),
			}, nil
		}
		// Second call: verify new token header
		if got := r.Header.Get("X-Authorization-Token"); got != "new-token" {
			t.Errorf("expected X-Authorization-Token=new-token on retry, got %q", got)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("ok")),
			Header:     make(http.Header),
		}, nil
	})

	mw := RefreshRetryMiddleware{Provider: provider}
	rt := mw.Wrap(backend)

	req, _ := http.NewRequest(http.MethodGet, "http://example.com/", nil)
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if got := atomic.LoadInt32(&provider.refreshCalls); got != 1 {
		t.Errorf("expected 1 refresh call, got %d", got)
	}
	if callCount != 2 {
		t.Errorf("expected 2 backend calls, got %d", callCount)
	}
}

func TestRefreshRetryMiddleware_401_PAT_NoRetry(t *testing.T) {
	provider := &mockRefreshableProvider{
		token:        "pat-token",
		tokenType:    "pat",
		refreshToken: "should-not-be-used",
	}

	callCount := 0
	backend := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		callCount++
		return &http.Response{
			StatusCode: http.StatusUnauthorized,
			Body:       io.NopCloser(strings.NewReader("")),
			Header:     make(http.Header),
		}, nil
	})

	mw := RefreshRetryMiddleware{Provider: provider}
	rt := mw.Wrap(backend)

	req, _ := http.NewRequest(http.MethodGet, "http://example.com/", nil)
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}
	if got := atomic.LoadInt32(&provider.refreshCalls); got != 0 {
		t.Errorf("expected 0 refresh calls, got %d", got)
	}
	if callCount != 1 {
		t.Errorf("expected 1 backend call, got %d", callCount)
	}
}

func TestRefreshRetryMiddleware_NonRefreshable_NoRetry(t *testing.T) {
	provider := &plainProvider{}

	callCount := 0
	backend := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		callCount++
		return &http.Response{
			StatusCode: http.StatusUnauthorized,
			Body:       io.NopCloser(strings.NewReader("")),
			Header:     make(http.Header),
		}, nil
	})

	mw := RefreshRetryMiddleware{Provider: provider}
	rt := mw.Wrap(backend)

	req, _ := http.NewRequest(http.MethodGet, "http://example.com/", nil)
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}
	if callCount != 1 {
		t.Errorf("expected 1 backend call, got %d", callCount)
	}
}

func TestRefreshRetryMiddleware_200_NoRetry(t *testing.T) {
	provider := &mockRefreshableProvider{
		token:        "oauth-token",
		tokenType:    "oauth",
		refreshToken: "should-not-be-used",
	}

	callCount := 0
	backend := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		callCount++
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("ok")),
			Header:     make(http.Header),
		}, nil
	})

	mw := RefreshRetryMiddleware{Provider: provider}
	rt := mw.Wrap(backend)

	req, _ := http.NewRequest(http.MethodGet, "http://example.com/", nil)
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if got := atomic.LoadInt32(&provider.refreshCalls); got != 0 {
		t.Errorf("expected 0 refresh calls, got %d", got)
	}
	if callCount != 1 {
		t.Errorf("expected 1 backend call, got %d", callCount)
	}
}
