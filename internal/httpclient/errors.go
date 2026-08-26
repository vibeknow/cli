package httpclient

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/vibeknow/cli/internal/clerr"
	"github.com/vibeknow/cli/internal/errs"
)

// classifyTransportError converts an error from http.Client.Do into the
// error the CLI should surface. It exists because http.Client wraps every
// RoundTripper failure in *url.Error, which hides the fact that some of
// those failures never touched the network at all.
//
// The case that matters is AuthMiddleware: when no credential is stored, or
// the stored one is dead, the middleware aborts the request with a
// clerr.Auth carrying the "run `vibeknow auth login`" instruction. Flattening
// that into network_error was wrong twice over — the process exited 1
// instead of 3, so an agent could not tell "the user must log in" from "the
// API misbehaved", and the error claimed Retryable, so the honest response
// to it was to retry a request that cannot ever succeed without the user.
func classifyTransportError(err error) error {
	var eo *errObject
	if errors.As(err, &eo) {
		return eo
	}
	// A middleware that already classified the failure outranks any guess
	// this layer could make from a transport error.
	var ce *clerr.Error
	if errors.As(err, &ce) {
		return ce
	}
	// Same for a structured backend error that reached us through a
	// RoundTripper rather than a response body — RefreshRetryMiddleware
	// returns one when a forced refresh finds the session permanently gone.
	// Rebuilding that as network_error told the user their connection was
	// flaky when their session had actually been killed.
	var o *errs.Object
	if errors.As(err, &o) {
		return &errObject{
			Code:      o.Code,
			Message:   o.Message,
			TraceID:   o.TraceID,
			Retryable: o.Retryable,
		}
	}
	return &errObject{Code: "network_error", Message: err.Error(), Retryable: true}
}

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
	// CodeAuthRequired is the generic 401. On an ordinary request it means
	// the access token is stale and a refresh will fix it; on the refresh
	// endpoint itself it means the refresh token was rejected — expired,
	// revoked, or signed with a key the server no longer holds — and no
	// amount of retrying will change that.
	CodeAuthRequired = "auth_required"
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
// ExitCodeForCode maps a stable CLI error code to the process exit code the
// documented contract promises for it. Returns 0 when the code carries no
// specific meaning, so the caller keeps its own default.
//
// Without this, only `vk create` translated backend codes into exit codes and
// every other command exited 1 for everything. An expired token — by far the
// most common recoverable failure, and the one the docs tell agents to handle
// by re-authenticating on exit 3 — was indistinguishable from a bug.
//
// Deliberately conservative: codes whose right response is genuinely ambiguous
// (permission_denied, not_found, conflict, business_error) are left at the
// generic exit 1 rather than guessing. permission_denied in particular must
// not become 3, or an agent would re-authenticate in a loop over a resource it
// will never be allowed to touch.
func ExitCodeForCode(code string) int {
	switch {
	case code == CodeAuthRequired,
		code == CodeSessionReplaced,
		code == CodeAccountDisabled,
		code == CodeAccountPendingDeletion,
		code == "session_expired":
		return 3 // credential missing / invalid / expired → re-authenticate
	case IsRetryableCode(code):
		return 4 // rate_limited, internal_error, concurrent_work_limit
	case code == "insufficient_credits", isQuotaExhaustedCode(code):
		return 5 // business failure; retrying the same command cannot help
	case IsUserFixableCode(code), code == "invalid_args":
		return 2 // the caller's input is wrong and can be corrected
	}
	return 0
}

// isQuotaExhaustedCode reports whether the code means an allowance ran out:
// too many projects for the tier, a project already at its work limit, the
// rolling TTS preview budget spent.
//
// These share their shape with insufficient_credits and are mapped the same
// way. Left at the generic exit 1 they read as "something went wrong", and
// an agent's reasonable response to that is to try again — which cannot
// work, because nothing about the request is wrong. Exit 5 says the run
// failed for a reason the CLI cannot fix, so the answer is to tell the user
// what ran out.
func isQuotaExhaustedCode(code string) bool {
	switch code {
	case "project_quota_exceeded", "project_works_full", "tts_preview_quota_exceeded":
		return true
	}
	return false
}

func IsRetryableCode(code string) bool {
	switch code {
	case "rate_limited", "internal_error", "concurrent_work_limit", "work_edit_busy":
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
	case "script_invalid", "replica_invalid", "knowledge_unsupported", "image_invalid":
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
	case 100007:
		// Image-mode preflight rejection: page count infeasible for the
		// document (word count < pages × 50, or more mandatory images than
		// pages). User-fixable input error. The label deliberately says
		// "image", not the wire kind, which carries an internal codename.
		return "image_invalid", true
	case 100008:
		// The work is being edited (scene-edit distributed lock held).
		// Transient by construction — the lock clears when the edit ends.
		return "work_edit_busy", true
	case 100009:
		// Project count at the membership tier's cap.
		return "project_quota_exceeded", true
	case 100010:
		// Single project is full (max works per project).
		return "project_works_full", true
	case 100011:
		// Per-work rolling TTS preview quota exhausted.
		return "tts_preview_quota_exceeded", true
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
		return CodeAuthRequired
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
		return CodeAuthRequired
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
