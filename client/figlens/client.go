// Package figlens is the CLI client for the Figlens video pipeline service.
package figlens

import (
	"github.com/vibeknow/cli/internal/httpclient"
)

type Client struct {
	http *httpclient.Client
}

func New(baseURL string, tokenProvider httpclient.TokenProvider) *Client {
	return &Client{http: httpclient.New(baseURL).WithEnvelope().WithTransport(httpclient.StandardChain(tokenProvider, nil))}
}
