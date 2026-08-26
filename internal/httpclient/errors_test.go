package httpclient

import (
	"net/http"
	"testing"
)

func TestErrObjectAsErrsObject(t *testing.T) {
	e := &errObject{Code: "not_found", Message: "x", TraceID: "tx", Retryable: false}
	o := e.AsErrsObject()
	if o.Code != "not_found" || o.TraceID != "tx" || o.SchemaVersion != "1" {
		t.Errorf("bad conversion: %+v", o)
	}
}

func TestIsRetryableCode(t *testing.T) {
	tests := []struct {
		code string
		want bool
	}{
		// Transient: re-running the same request can succeed.
		{"rate_limited", true},
		{"internal_error", true},
		{"concurrent_work_limit", true},
		{"work_edit_busy", true},
		// Permanent: user must change something first.
		{"insufficient_credits", false},
		{"script_invalid", false},
		{"image_invalid", false},
		{"project_quota_exceeded", false},
		{"freeze_not_found", false},
		{"auth_required", false},
		{"business_error", false},
		// Empty / unknown: never promise retry will help.
		{"", false},
		{"unknown_label", false},
	}
	for _, tc := range tests {
		t.Run(tc.code, func(t *testing.T) {
			if got := IsRetryableCode(tc.code); got != tc.want {
				t.Errorf("IsRetryableCode(%q) = %v, want %v", tc.code, got, tc.want)
			}
		})
	}
}

func TestMapEnvelopeCode(t *testing.T) {
	tests := []struct {
		name       string
		envCode    int
		httpStatus int
		want       string
	}{
		// Account service auth domain (110xxx) — these three are
		// consumed by the OAuth token provider to decide whether to
		// purge the stored credential.
		{"account_disabled", 110004, http.StatusUnauthorized, CodeAccountDisabled},
		{"session_replaced", 110008, http.StatusUnauthorized, CodeSessionReplaced},
		{"account_pending_deletion", 110013, http.StatusUnprocessableEntity, CodeAccountPendingDeletion},
		// Other 110xxx codes stay as generic business_error.
		{"other_110xxx_bucket", 110001, http.StatusBadRequest, "business_error"},
		// Class-based buckets still work.
		{"auth_class", 40101, http.StatusUnauthorized, "auth_required"},
		{"rate_limited", 42901, http.StatusTooManyRequests, "rate_limited"},
		{"internal_error", 50001, http.StatusInternalServerError, "internal_error"},
		// Known business codes retain their specific labels.
		{"insufficient_credits", 100001, http.StatusPaymentRequired, "insufficient_credits"},
		{"script_invalid", 100004, http.StatusBadRequest, "script_invalid"},
		{"image_invalid", 100007, http.StatusBadRequest, "image_invalid"},
		{"work_edit_busy", 100008, http.StatusConflict, "work_edit_busy"},
		{"project_quota_exceeded", 100009, http.StatusForbidden, "project_quota_exceeded"},
		{"project_works_full", 100010, http.StatusForbidden, "project_works_full"},
		{"tts_preview_quota_exceeded", 100011, http.StatusTooManyRequests, "tts_preview_quota_exceeded"},
		// Unmapped 100xxx codes fall to the generic bucket, not an HTTP class.
		{"unknown_business", 100099, http.StatusBadRequest, "business_error"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := mapEnvelopeCode(tc.envCode, tc.httpStatus); got != tc.want {
				t.Errorf("mapEnvelopeCode(%d, %d) = %q, want %q", tc.envCode, tc.httpStatus, got, tc.want)
			}
		})
	}
}
