// Package clerr provides CLI-level structured errors with exit codes and
// optional hints. Errors bubble to main.go which inspects the Code field
// for process exit and renders a user-facing message (text or JSON envelope).
package clerr

import (
	"errors"
	"fmt"
)

// Error types — communicated to scripts via the JSON envelope's "type" field.
// Exit codes are coarser (see Exit* constants below); the type field lets
// callers distinguish e.g. permission vs. rate_limit without needing more
// exit codes.
const (
	TypePermission = "permission"
	TypeValidation = "validation"
	TypeAuth       = "auth"
	TypeNetwork    = "network"
	TypeAPI        = "api"
	TypeNotFound   = "not_found"
	TypeRateLimit  = "rate_limit"
	TypeInternal   = "internal"
)

// Exit codes. Fine-grained error semantics (permission, not_found, …) travel
// on the envelope's "type" field, not on the exit code.
const (
	ExitOK         = 0
	ExitAPI        = 1 // generic API / catch-all
	ExitValidation = 2 // argument / flag validation failed
	ExitAuth       = 3 // token missing / invalid / expired
	ExitNetwork    = 4 // DNS / connect / timeout
	ExitInternal   = 5 // bug — should not happen
)

type Error struct {
	Type    string
	Code    int
	Message string
	Hint    string
	Detail  any
}

func (e *Error) Error() string { return e.Message }

func newTyped(typ string, code int, msg string) *Error {
	return &Error{Type: typ, Code: code, Message: msg}
}

func New(msg string) *Error        { return newTyped(TypeAPI, ExitAPI, msg) }
func Auth(msg string) *Error       { return newTyped(TypeAuth, ExitAuth, msg) }
func Validation(msg string) *Error { return newTyped(TypeValidation, ExitValidation, msg) }
func Network(msg string) *Error    { return newTyped(TypeNetwork, ExitNetwork, msg) }
func Internal(msg string) *Error   { return newTyped(TypeInternal, ExitInternal, msg) }

func Newf(format string, args ...any) *Error        { return New(fmt.Sprintf(format, args...)) }
func Authf(format string, args ...any) *Error       { return Auth(fmt.Sprintf(format, args...)) }
func Validationf(format string, args ...any) *Error { return Validation(fmt.Sprintf(format, args...)) }
func Networkf(format string, args ...any) *Error    { return Network(fmt.Sprintf(format, args...)) }
func Internalf(format string, args ...any) *Error   { return Internal(fmt.Sprintf(format, args...)) }

func (e *Error) WithHint(hint string) *Error { e.Hint = hint; return e }
func (e *Error) WithHintf(format string, args ...any) *Error {
	e.Hint = fmt.Sprintf(format, args...)
	return e
}
func (e *Error) WithCode(code int) *Error  { e.Code = code; return e }
func (e *Error) WithType(t string) *Error  { e.Type = t; return e }
func (e *Error) WithDetail(d any) *Error   { e.Detail = d; return e }

// ExitCodeFor returns the exit code for err. Unwraps via errors.As so wrapped
// *Error values are honored; non-*Error errors exit 1.
func ExitCodeFor(err error) int {
	if err == nil {
		return ExitOK
	}
	var e *Error
	if errors.As(err, &e) {
		if e.Code == 0 {
			return ExitAPI
		}
		return e.Code
	}
	return ExitAPI
}
