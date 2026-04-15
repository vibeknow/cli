package errs

import "testing"

func TestObjectImplementsError(t *testing.T) {
	var _ error = (*Object)(nil)
	o := &Object{Code: "not_found", Message: "x"}
	if o.Error() == "" {
		t.Error("Error() empty")
	}
}

func TestIsRetryable(t *testing.T) {
	r := &Object{Code: "rate_limited", Retryable: true}
	if !r.IsRetryable() {
		t.Error("should be retryable")
	}
}
