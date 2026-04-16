// Package errs defines the canonical Error Object (spec §11.2).
package errs

import (
	"errors"
	"fmt"
)

type Object struct {
	SchemaVersion string         `json:"schema_version"`
	Code          string         `json:"code"`
	Message       string         `json:"message"`
	Details       map[string]any `json:"details,omitempty"`
	Retryable     bool           `json:"retryable"`
	TraceID       string         `json:"trace_id,omitempty"`
}

func (o *Object) Error() string {
	if o.TraceID != "" {
		return fmt.Sprintf("[%s] %s (trace=%s)", o.Code, o.Message, o.TraceID)
	}
	return fmt.Sprintf("[%s] %s", o.Code, o.Message)
}

func (o *Object) IsRetryable() bool { return o.Retryable }

func New(code, message string) *Object {
	return &Object{SchemaVersion: "1", Code: code, Message: message}
}

// HasCode reports whether err is an *Object with the given code.
func HasCode(err error, code string) bool {
	var o *Object
	if errors.As(err, &o) {
		return o.Code == code
	}
	return false
}
