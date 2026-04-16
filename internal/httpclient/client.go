// Package httpclient provides a shared HTTP client used by all service
// clients in client/*. See spec §4 and §11.2.
package httpclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const ClientAPIVersion = "v1"

type Client struct {
	baseURL string
	http    *http.Client
}

func New(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *Client) WithTransport(rt http.RoundTripper) *Client {
	nc := *c
	nc.http = &http.Client{Transport: rt, Timeout: c.http.Timeout}
	return &nc
}

// Transport returns the underlying RoundTripper so callers can issue raw requests
// that still traverse the middleware chain.
func (c *Client) Transport() http.RoundTripper { return c.http.Transport }

func (c *Client) Do(ctx context.Context, method, path string, body, out any) error {
	var reader io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("httpclient: marshal body: %w", err)
		}
		reader = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return fmt.Errorf("httpclient: new request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		var eo *errObject
		if errors.As(err, &eo) {
			return eo
		}
		return &errObject{Code: "network_error", Message: err.Error(), Retryable: true}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return parseBackendError(resp)
	}
	if out == nil || resp.StatusCode == http.StatusNoContent {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return &errObject{Code: "unknown", Message: "decode response: " + err.Error()}
	}
	return nil
}

// DoRaw sends a request through the middleware chain and returns the raw
// *http.Response with body still open. Caller MUST close resp.Body.
// Returns an error for HTTP >= 400 (body is read and closed in that case).
func (c *Client) DoRaw(ctx context.Context, method, path string, body any) (*http.Response, error) {
	var reader io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("httpclient: marshal body: %w", err)
		}
		reader = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return nil, fmt.Errorf("httpclient: new request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "text/event-stream")

	resp, err := c.http.Do(req)
	if err != nil {
		var eo *errObject
		if errors.As(err, &eo) {
			return nil, eo
		}
		return nil, &errObject{Code: "network_error", Message: err.Error(), Retryable: true}
	}
	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		return nil, parseBackendError(resp)
	}
	return resp, nil
}
