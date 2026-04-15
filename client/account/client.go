// Package account is the CLI client for go-account.
// P1 implements only Whoami; login endpoints are P1.5 scope.
package account

import (
	"net/http"

	"github.com/shiliu-ai/vibeknow-cli/internal/httpclient"
)

type Client struct {
	http *httpclient.Client
}

// New constructs an account client with auth + trace-id + version middleware.
func New(baseURL string, tokenProvider httpclient.TokenProvider) *Client {
	chain := httpclient.Chain(http.DefaultTransport,
		httpclient.AuthMiddleware{Provider: tokenProvider},
		httpclient.TraceIDMiddleware{},
		httpclient.VersionMiddleware{Expected: httpclient.ClientAPIVersion},
	)
	return &Client{http: httpclient.New(baseURL).WithTransport(chain)}
}
