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
	baseURL  string
	http     *http.Client
	envelope bool // if true, unwrap {"code":0,"data":{...}} envelope (Go services)
}

// envelope is the common response wrapper used by Go backend services
// (account, vibeknow, figlens). vectoria does NOT use this.
type envelope struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

func New(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

// WithEnvelope enables {"code":0,"data":{...}} unwrapping for Go backend services.
func (c *Client) WithEnvelope() *Client {
	nc := *c
	nc.envelope = true
	return &nc
}

func (c *Client) WithTransport(rt http.RoundTripper) *Client {
	nc := *c
	nc.http = &http.Client{Transport: rt, Timeout: c.http.Timeout}
	return &nc
}

// Transport returns the underlying RoundTripper so callers can issue raw requests
// that still traverse the middleware chain.
func (c *Client) Transport() http.RoundTripper { return c.http.Transport }

// BaseURL returns the base URL for this client.
func (c *Client) BaseURL() string { return c.baseURL }

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

	if c.envelope {
		// Go services wrap responses in {"code":0,"message":"...","data":{...}}
		var env envelope
		if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
			return &errObject{Code: "unknown", Message: "decode envelope: " + err.Error()}
		}
		if env.Code != 0 {
			return &errObject{
				Code:    mapEnvelopeCode(env.Code, resp.StatusCode),
				Message: env.Message,
			}
		}
		if len(env.Data) == 0 || string(env.Data) == "null" {
			return nil
		}
		if err := json.Unmarshal(env.Data, out); err != nil {
			return &errObject{Code: "unknown", Message: "decode envelope data: " + err.Error()}
		}
		return nil
	}

	// No envelope (e.g., vectoria) — decode body directly.
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return &errObject{Code: "unknown", Message: "decode response: " + err.Error()}
	}
	return nil
}

// DoRaw sends a request through the middleware chain and returns the raw
// *http.Response with body still open. Caller MUST close resp.Body.
// Returns an error for HTTP >= 400 (body is read and closed in that case).
// Uses no timeout on the HTTP client (SSE streams can last minutes); rely on
// ctx for cancellation.
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

	// Use a separate http.Client with no timeout for streaming responses.
	// The standard c.http has Timeout=30s which kills SSE streams.
	streamClient := &http.Client{Transport: c.http.Transport}
	resp, err := streamClient.Do(req)
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
