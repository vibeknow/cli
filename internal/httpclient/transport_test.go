package httpclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

type header struct{ key, value string }

func (h header) Wrap(next http.RoundTripper) http.RoundTripper {
	return roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		r.Header.Set(h.key, h.value)
		return next.RoundTrip(r)
	})
}

func TestChainAppliesInOrder(t *testing.T) {
	var got http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
	}))
	defer srv.Close()

	chain := Chain(http.DefaultTransport,
		header{"X-A", "1"},
		header{"X-B", "2"},
	)
	c := New(srv.URL).WithTransport(chain)
	_ = c.Do(context.Background(), "GET", "/", nil, nil)
	if got.Get("X-A") != "1" || got.Get("X-B") != "2" {
		t.Errorf("headers not applied: %+v", got)
	}
}

func TestStandardChainRetriesAndInjectsAuth(t *testing.T) {
	var hits int32
	var authSeen string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&hits, 1) < 2 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		authSeen = r.Header.Get("X-Authorization-Token")
		w.Header().Set("X-Vibeknow-Api-Version", "v1")
		w.WriteHeader(200)
	}))
	defer srv.Close()

	chain := StandardChain(staticTokenProvider("abc"), nil)
	c := New(srv.URL).WithTransport(chain)
	if err := c.Do(context.Background(), "GET", "/", nil, nil); err != nil {
		t.Fatal(err)
	}
	if authSeen != "abc" {
		t.Errorf("auth not injected: %q", authSeen)
	}
	if atomic.LoadInt32(&hits) != 2 {
		t.Errorf("retry didn't happen: hits=%d", atomic.LoadInt32(&hits))
	}
}
