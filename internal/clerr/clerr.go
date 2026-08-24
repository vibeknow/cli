// Package clerr provides CLI-level structured errors with exit codes and
// optional hints. Errors bubble to main.go which inspects the Code field
// for process exit and renders a user-facing message (text or JSON envelope).
package clerr

import (
	"errors"
	"fmt"
)

// Error types — communicated to scripts via the JSON envelope's "type" field.
// Exit codes are coarser (see Exit* constants below); additional types can
// be added here when a distinct category actually ships.
const (
	TypeValidation = "validation"
	TypeAuth       = "auth"
	TypeNetwork    = "network"
	TypeAPI        = "api"
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
	// Cause is the underlying error, if any. It is not rendered — Message
	// carries the user-facing text — but it stays reachable through
	// errors.Is/As so structured backend codes (errs.Object) survive being
	// wrapped in a CLI error.
	Cause error
}

func (e *Error) Error() string { return e.Message }

// Unwrap exposes Cause so errors.Is / errors.As / errs.HasCode can see
// through a clerr wrapper to the error it was built from.
func (e *Error) Unwrap() error { return e.Cause }

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
func (e *Error) WithCause(c error) *Error { e.Cause = c; return e }
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
