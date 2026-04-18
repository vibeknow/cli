// Package vectoria is the CLI client for the vectoria document/RAG service.
//
// Auth: JWT via X-Authorization-Token, same mechanism as figlens/account.
// The backend also accepts X-API-Key as a fallback, but the CLI does not
// expose it — credentials flow through VIBEKNOW_TOKEN or the OS keychain.
package vectoria

import (
	"net/http"

	"github.com/vibeknow/cli/internal/httpclient"
)

type Client struct {
	http *httpclient.Client
}

// New creates a vectoria client using the same JWT-based auth chain as other
// CLI services. Pass nil for tp only in tests where the server ignores auth.
func New(baseURL string, tp httpclient.TokenProvider) *Client {
	chain := httpclient.Chain(http.DefaultTransport,
		httpclient.AuthMiddleware{Provider: tp},
		httpclient.RefreshRetryMiddleware{Provider: tp},
	)
	return &Client{http: httpclient.New(baseURL).WithTransport(chain)}
}
