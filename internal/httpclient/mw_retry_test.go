package httpclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestRetryOn502ThenSucceeds(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&hits, 1) < 3 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()

	chain := Chain(http.DefaultTransport, RetryMiddleware{
		MaxAttempts: 4,
		BaseDelay:   10 * time.Millisecond,
	})
	c := New(srv.URL).WithTransport(chain)
	if err := c.Do(context.Background(), "GET", "/", nil, nil); err != nil {
		t.Fatalf("should have succeeded after retries: %v", err)
	}
	if atomic.LoadInt32(&hits) != 3 {
		t.Errorf("expected 3 hits, got %d", atomic.LoadInt32(&hits))
	}
}

func TestRetryGivesUp(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	chain := Chain(http.DefaultTransport, RetryMiddleware{
		MaxAttempts: 2,
		BaseDelay:   1 * time.Millisecond,
	})
	c := New(srv.URL).WithTransport(chain)
	err := c.Do(context.Background(), "GET", "/", nil, nil)
	if err == nil {
		t.Fatal("should have failed")
	}
	if atomic.LoadInt32(&hits) != 2 {
		t.Errorf("expected 2 hits, got %d", atomic.LoadInt32(&hits))
	}
}

func TestRetrySkipsNon5xx(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	chain := Chain(http.DefaultTransport, RetryMiddleware{MaxAttempts: 5, BaseDelay: 1 * time.Millisecond})
	c := New(srv.URL).WithTransport(chain)
	_ = c.Do(context.Background(), "GET", "/", nil, nil)
	if atomic.LoadInt32(&hits) != 1 {
		t.Errorf("404 should not retry, got %d hits", atomic.LoadInt32(&hits))
	}
}
