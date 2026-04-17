package clerr

import "fmt"

// Error is a CLI error with an optional hint for the user.
type Error struct {
	Message string
	Hint    string
	Code    int // exit code: 0=default, 1=general, 3=auth, 5=internal
}

func (e *Error) Error() string { return e.Message }

// New creates a CLI error with a message.
func New(msg string) *Error {
	return &Error{Message: msg, Code: 1}
}

// Newf creates a formatted CLI error.
func Newf(format string, args ...any) *Error {
	return &Error{Message: fmt.Sprintf(format, args...), Code: 1}
}

// WithHint adds an actionable hint.
func (e *Error) WithHint(hint string) *Error {
	e.Hint = hint
	return e
}

// WithCode sets the exit code.
func (e *Error) WithCode(code int) *Error {
	e.Code = code
	return e
}

// Auth creates an auth error (exit code 3).
func Auth(msg string) *Error {
	return &Error{Message: msg, Code: 3}
}

// Authf creates a formatted auth error.
func Authf(format string, args ...any) *Error {
	return &Error{Message: fmt.Sprintf(format, args...), Code: 3}
}
