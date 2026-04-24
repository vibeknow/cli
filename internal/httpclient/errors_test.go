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
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := mapEnvelopeCode(tc.envCode, tc.httpStatus); got != tc.want {
				t.Errorf("mapEnvelopeCode(%d, %d) = %q, want %q", tc.envCode, tc.httpStatus, got, tc.want)
			}
		})
	}
}
