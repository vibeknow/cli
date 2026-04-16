// Package vibeknow is the CLI client for the go-vibeknow service
// (billing, voice clone, credits).
package vibeknow

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/shiliu-ai/vibeknow-cli/internal/httpclient"
)

type Client struct {
	http *httpclient.Client
}

func New(baseURL string, tokenProvider httpclient.TokenProvider) *Client {
	return &Client{http: httpclient.New(baseURL).WithTransport(httpclient.StandardChain(tokenProvider, nil))}
}

type envelope struct {
	Code int             `json:"code"`
	Data json.RawMessage `json:"data"`
}

func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var env envelope
	if err := c.http.Do(ctx, method, path, body, &env); err != nil {
		return err
	}
	if out != nil && len(env.Data) > 0 {
		if err := json.Unmarshal(env.Data, out); err != nil {
			return fmt.Errorf("vibeknow: decode data: %w", err)
		}
	}
	return nil
}
