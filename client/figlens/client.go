// Package figlens is the CLI client for the go-figlens video pipeline service.
package figlens

import (
	"github.com/shiliu-ai/vibeknow-cli/internal/httpclient"
)

type Client struct {
	http *httpclient.Client
}

func New(baseURL string, tokenProvider httpclient.TokenProvider) *Client {
	return &Client{http: httpclient.New(baseURL).WithEnvelope().WithTransport(httpclient.StandardChain(tokenProvider, nil))}
}
