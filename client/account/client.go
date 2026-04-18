// Package account is the CLI client for the Account service.
package account

import (
	"github.com/vibeknow/cli/internal/httpclient"
)

type Client struct {
	http *httpclient.Client
}

// New constructs an account client with the standard middleware chain.
func New(baseURL string, tokenProvider httpclient.TokenProvider) *Client {
	return &Client{http: httpclient.New(baseURL).WithEnvelope().WithTransport(httpclient.StandardChain(tokenProvider, nil))}
}
