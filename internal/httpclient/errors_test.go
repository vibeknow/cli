package httpclient

import "testing"

func TestErrObjectAsErrsObject(t *testing.T) {
	e := &errObject{Code: "not_found", Message: "x", TraceID: "tx", Retryable: false}
	o := e.AsErrsObject()
	if o.Code != "not_found" || o.TraceID != "tx" || o.SchemaVersion != "1" {
		t.Errorf("bad conversion: %+v", o)
	}
}
