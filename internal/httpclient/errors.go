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

// IsRetryableCode reports whether the operation that produced this code
// is likely to succeed if the caller retries the same request unchanged.
// Used by transports that cannot read a server-supplied retryable flag —
// in particular the SSE stream path, where the backend currently emits
// neither `code` nor `retryable` on its terminal `error` event, so the
// CLI must derive the answer from the code label alone.
//
// Codes not listed return false: the safer default is to not promise a
// retry will help when we cannot prove it.
func IsRetryableCode(code string) bool {
	switch code {
	case "rate_limited", "internal_error", "concurrent_work_limit":
		return true
	}
	return false
}

// IsUserFixableCode reports whether the code labels a rejected user input
// (bad script, wrong doc type, unsupported document): retrying unchanged
// will never help, but fixing the input will. Consumers map these to exit
// code 2. Kept next to MapBusinessCode so a new preflight code is added
// once and every exit-code site (create init-time, create stream-time,
// video wait) stays in sync.
func IsUserFixableCode(code string) bool {
	switch code {
	case "script_invalid", "replica_invalid", "knowledge_unsupported":
		return true
	}
	return false
}

// MapBusinessCode maps a 100xxx-range backend envelope code to a stable
// CLI error code label. Returns ok=false for codes outside that range so
// callers can fall back to their own mapping (e.g. HTTP-class for HTTP,
// "business_error" for SSE). Shared by mapEnvelopeCode (HTTP path) and
// client/figlens.mapSSECode (SSE path) so a new business code only needs
// to be added once.
func MapBusinessCode(envCode int) (string, bool) {
	switch envCode {
	case 100001:
		return "insufficient_credits", true
	case 100002:
		return "freeze_not_found", true
	case 100003:
		return "concurrent_work_limit", true
	case 100004:
		return "script_invalid", true
	case 100005:
		// Replica (PPT 讲解) preflight rejection: doc is not a PPT-style
		// PDF or exceeds the content limit. User-fixable input error.
		return "replica_invalid", true
	case 100006:
		// Knowledge document unsupported: parsed to completion but empty
		// content (e.g. image-only PDF, unfetchable link). User-fixable.
		return "knowledge_unsupported", true
	}
	if envCode >= 100000 {
		return "business_error", true
	}
	return "", false
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
	// Account service auth domain (110xxx) — named so the token provider
	// can distinguish "session is permanently dead" from transient refresh
	// failures and purge the stored credential.
	case envCode == 110004:
		return CodeAccountDisabled
	case envCode == 110008:
		return CodeSessionReplaced
	case envCode == 110013:
		return CodeAccountPendingDeletion
	}
	if label, ok := MapBusinessCode(envCode); ok {
		return label
	}
	return mapHTTPCode(httpStatus)
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
