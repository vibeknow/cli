package httpclient

import (
	"encoding/json"
	"net/http"

	"github.com/vibeknow/cli/internal/errs"
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

// As supports errors.As conversion to *errs.Object.
func (e *errObject) As(target any) bool {
	if t, ok := target.(**errs.Object); ok {
		*t = &errs.Object{
			SchemaVersion: "1",
			Code:          e.Code,
			Message:       e.Message,
			TraceID:       e.TraceID,
			Retryable:     e.Retryable,
		}
		return true
	}
	return false
}

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
	// Prefer the envelope business code when present (e.g. 110008
	// session_replaced on HTTP 401) so callers can distinguish permanent
	// session-dead errors from generic auth_required. Fall back to the
	// HTTP-class mapping for bodies that don't carry an envelope code.
	var code string
	if body.Code != 0 {
		code = mapEnvelopeCode(body.Code, resp.StatusCode)
	} else {
		code = mapHTTPCode(resp.StatusCode)
	}
	return &errObject{
		Code:      code,
		Message:   body.Message,
		TraceID:   body.TraceID,
		Retryable: body.Retryable || is5xx(resp.StatusCode),
		HTTPCode:  resp.StatusCode,
	}
}

// Backend envelope codes that the CLI distinguishes by name. Keep the string
// values stable — callers match on them via errs.HasCode.
const (
	// CodeSessionReplaced (backend 110008): issued when the single-device
	// middleware sees a newer token_version, OR when a device-flow session's
	// underlying user row has been soft-deleted.
	CodeSessionReplaced = "session_replaced"
	// CodeAccountDisabled (backend 110004): user.status == disabled.
	CodeAccountDisabled = "account_disabled"
	// CodeAccountPendingDeletion (backend 110013): account in the deletion
	// cooling period; user must restore or re-login after cooling.
	CodeAccountPendingDeletion = "account_pending_deletion"
)

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
	// Account service auth domain (110xxx) — named so the token provider
	// can distinguish "session is permanently dead" from transient refresh
	// failures and purge the stored credential.
	case envCode == 110004:
		return CodeAccountDisabled
	case envCode == 110008:
		return CodeSessionReplaced
	case envCode == 110013:
		return CodeAccountPendingDeletion
	// Business errors (100xxx).
	case envCode == 100001:
		return "insufficient_credits"
	case envCode == 100002:
		return "freeze_not_found"
	case envCode == 100003:
		return "concurrent_work_limit"
	case envCode == 100004:
		return "script_invalid"
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
