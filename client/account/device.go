package account

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/vibeknow/cli/internal/httpclient"
)

// NewUnauthenticated creates an account client with no auth middleware.
// Used for pre-login endpoints such as device code flow.
func NewUnauthenticated(baseURL string) *Client {
	return &Client{http: httpclient.New(baseURL).WithEnvelope()}
}

// DeviceCodeResponse holds the response from the device code initiation endpoint.
type DeviceCodeResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	// RFC 8628 optional field: the verification URI with the user code already
	// embedded (?user_code=…), so an opened page needs no copy-paste. Empty
	// when the account service predates it — callers fall back to synthesizing
	// the same shape from VerificationURI + UserCode.
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
}

// DeviceCode initiates the device authorization flow.
// POST /v1/auth/device/code
func (c *Client) DeviceCode(ctx context.Context) (*DeviceCodeResponse, error) {
	var resp DeviceCodeResponse
	body := map[string]string{
		"client_id": "vibeknow-cli",
		"scope":     "full",
	}
	if err := c.http.Do(ctx, http.MethodPost, "/v1/auth/device/code", body, &resp); err != nil {
		return nil, err
	}
	// Apply defaults if not returned by server.
	if resp.Interval == 0 {
		resp.Interval = 5
	}
	if resp.ExpiresIn == 0 {
		resp.ExpiresIn = 900
	}
	return &resp, nil
}

// DeviceTokenResponse holds tokens returned after successful device authorization.
type DeviceTokenResponse struct {
	AccessToken      string `json:"access_token"`
	RefreshToken     string `json:"refresh_token"`
	TokenType        string `json:"token_type"`
	ExpiresIn        int    `json:"expires_in"`
	RefreshExpiresIn int    `json:"refresh_expires_in"`
}

// Poll status constants returned as PollError.Status.
const (
	PollPending  = "authorization_pending"
	PollSlowDown = "slow_down"
	PollExpired  = "expired_token"
	PollDenied   = "access_denied"
)

// PollError is returned by DeviceToken for flow-control codes (40010-40013).
type PollError struct {
	Status  string
	Message string
}

func (e *PollError) Error() string { return fmt.Sprintf("device auth: %s", e.Status) }

// pollCodeToStatus maps backend envelope codes to PollError statuses.
var pollCodeToStatus = map[int]string{
	40010: PollPending,
	40011: PollSlowDown,
	40012: PollExpired,
	40013: PollDenied,
}

// rawEnvelope mirrors the envelope shape, used for manual parsing in DeviceToken.
type rawEnvelope struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

// DeviceToken polls the device token endpoint with the given device code.
// It bypasses the httpclient envelope handling so that flow-control codes
// (40010-40013) are returned as PollError rather than generic errors.
// POST /v1/auth/device/token
func (c *Client) DeviceToken(ctx context.Context, deviceCode string) (*DeviceTokenResponse, error) {
	body := map[string]string{
		"client_id":   "vibeknow-cli",
		"device_code": deviceCode,
		"grant_type":  "device_code",
	}
	buf, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("device token: marshal body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.http.BaseURL()+"/v1/auth/device/token", bytes.NewReader(buf))
	if err != nil {
		return nil, fmt.Errorf("device token: new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	// Use a plain http.Client with no middleware — we handle the raw envelope ourselves.
	httpCl := &http.Client{Timeout: 30 * time.Second}
	resp, err := httpCl.Do(req)
	if err != nil {
		return nil, fmt.Errorf("device token: request: %w", err)
	}
	defer resp.Body.Close()

	var env rawEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return nil, fmt.Errorf("device token: decode response: %w", err)
	}

	// Check for poll flow-control codes.
	if status, ok := pollCodeToStatus[env.Code]; ok {
		return nil, &PollError{Status: status, Message: env.Message}
	}

	if env.Code != 0 {
		return nil, fmt.Errorf("device token: server error %d: %s", env.Code, env.Message)
	}

	var tokenResp DeviceTokenResponse
	if len(env.Data) > 0 && string(env.Data) != "null" {
		if err := json.Unmarshal(env.Data, &tokenResp); err != nil {
			return nil, fmt.Errorf("device token: decode data: %w", err)
		}
	}
	return &tokenResp, nil
}
