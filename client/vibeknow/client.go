// Package vibeknow is the CLI client for the go-vibeknow service
// (billing, voice clone, credits).
package vibeknow

import (
	"github.com/vibeknow/cli/internal/httpclient"
)

type Client struct {
	http *httpclient.Client
}

func New(baseURL string, tokenProvider httpclient.TokenProvider) *Client {
	return &Client{http: httpclient.New(baseURL).WithEnvelope().WithTransport(httpclient.StandardChain(tokenProvider, nil))}
}
