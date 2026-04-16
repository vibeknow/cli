package httpclient

import (
	"encoding/json"
	"net/http"

	"github.com/shiliu-ai/vibeknow-cli/internal/errs"
)

type errObject struct {
	Code      string
	Message   string
	TraceID   string
	Retryable bool
	HTTPCode  int
}

func (e *errObject) Error() string {
	if e.TraceID != "" {
		return "[" + e.Code + "] " + e.Message + " (trace=" + e.TraceID + ")"
	}
	return "[" + e.Code + "] " + e.Message
}

func (e *errObject) IsRetryable() bool { return e.Retryable }

// AsErrsObject converts to the canonical user-facing Error Object (spec §11.2).
func (e *errObject) AsErrsObject() *errs.Object {
	return &errs.Object{
		SchemaVersion: "1",
		Code:          e.Code,
		Message:       e.Message,
		TraceID:       e.TraceID,
		Retryable:     e.Retryable,
	}
}

type backendBody struct {
	Code      int             `json:"code"`
	Message   string          `json:"message"`
	Data      json.RawMessage `json:"data,omitempty"`
	TraceID   string          `json:"trace_id,omitempty"`
	Retryable bool            `json:"retryable,omitempty"`
}

func parseBackendError(resp *http.Response) error {
	var body backendBody
	_ = json.NewDecoder(resp.Body).Decode(&body)
	return &errObject{
		Code:      mapHTTPCode(resp.StatusCode),
		Message:   body.Message,
		TraceID:   body.TraceID,
		Retryable: body.Retryable || is5xx(resp.StatusCode),
		HTTPCode:  resp.StatusCode,
	}
}

// mapEnvelopeCode maps a backend envelope code + HTTP status to a CLI error code.
// Backend aether codes: 40xxx = 4xx class, 50xxx = 5xx class, 100xxx+ = business errors.
func mapEnvelopeCode(envCode, httpStatus int) string {
	switch {
	case envCode >= 40100 && envCode < 40200:
		return "auth_required"
	case envCode >= 40300 && envCode < 40400:
		return "permission_denied"
	case envCode >= 40400 && envCode < 40500:
		return "not_found"
	case envCode >= 42900 && envCode < 43000:
		return "rate_limited"
	case envCode >= 50000 && envCode < 60000:
		return "internal_error"
	case envCode >= 100000:
		return "business_error"
	default:
		return mapHTTPCode(httpStatus)
	}
}

func mapHTTPCode(status int) string {
	switch {
	case status == http.StatusUnauthorized:
		return "auth_required"
	case status == http.StatusForbidden:
		return "permission_denied"
	case status == http.StatusNotFound:
		return "not_found"
	case status == http.StatusConflict:
		return "conflict"
	case status == http.StatusTooManyRequests:
		return "rate_limited"
	case status >= 500 && status < 600:
		return "internal_error"
	case status >= 400 && status < 500:
		return "invalid_args"
	default:
		return "unknown"
	}
}

func is5xx(status int) bool { return status >= 500 && status < 600 }
