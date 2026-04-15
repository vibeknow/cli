package httpclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDoSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"hello": "world"})
	}))
	defer srv.Close()

	c := New(srv.URL)
	var out map[string]string
	if err := c.Do(context.Background(), "GET", "/ping", nil, &out); err != nil {
		t.Fatal(err)
	}
	if out["hello"] != "world" {
		t.Errorf("got %+v", out)
	}
}

func TestDoBackendErrorMapping(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":      40401,
			"message":   "document not found",
			"trace_id":  "tx_abc",
			"retryable": false,
		})
	}))
	defer srv.Close()

	c := New(srv.URL)
	err := c.Do(context.Background(), "GET", "/docs/x", nil, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	eo, ok := err.(*errObject)
	if !ok {
		t.Fatalf("want *errObject, got %T", err)
	}
	if eo.Code != "not_found" || eo.Message != "document not found" || eo.TraceID != "tx_abc" {
		t.Errorf("unexpected mapping: %+v", eo)
	}
}

func TestDo5xxMappingRetryable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":      50200,
			"message":   "upstream down",
			"retryable": true,
		})
	}))
	defer srv.Close()

	c := New(srv.URL)
	err := c.Do(context.Background(), "GET", "/x", nil, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	eo := err.(*errObject)
	if eo.Code != "internal_error" || !eo.Retryable {
		t.Errorf("5xx with retryable should map correctly: %+v", eo)
	}
}
