package httpclient

import (
	"context"
	"net/http"
	"net/http/httptest"
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
