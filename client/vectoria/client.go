// Package vectoria is the CLI client for the vectoria document/RAG service.
// Auth uses X-API-Key header (not JWT Bearer).
package vectoria

import (
	"net/http"

	"github.com/vibeknow/cli/internal/httpclient"
)

type Client struct {
	http *httpclient.Client
}

func New(baseURL, apiKey string) *Client {
	chain := httpclient.Chain(http.DefaultTransport,
		apiKeyMiddleware{key: apiKey},
	)
	return &Client{http: httpclient.New(baseURL).WithTransport(chain)}
}

type apiKeyMiddleware struct{ key string }

func (m apiKeyMiddleware) Wrap(next http.RoundTripper) http.RoundTripper {
	return httpclient.RoundTripperFunc(func(r *http.Request) (*http.Response, error) {
		if m.key != "" {
			r.Header.Set("X-API-Key", m.key)
		}
		return next.RoundTrip(r)
	})
}
