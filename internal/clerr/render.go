package clerr

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"golang.org/x/term"
)

const (
	colorRed    = "\033[31m"
	colorYellow = "\033[33m"
	colorReset  = "\033[0m"
)

func isTTY() bool {
	return term.IsTerminal(int(os.Stderr.Fd()))
}

type ErrDetail struct {
	Type    string `json:"type"`
	Code    int    `json:"code,omitempty"`
	Message string `json:"message"`
	Hint    string `json:"hint,omitempty"`
	Detail  any    `json:"detail,omitempty"`
}

type ErrorEnvelope struct {
	OK       bool                   `json:"ok"`
	Identity string                 `json:"identity,omitempty"`
	Error    *ErrDetail             `json:"error"`
	Notice   map[string]interface{} `json:"_notice,omitempty"`
}

// PendingNotice, when set, supplies the _notice field for JSON error
// envelopes (e.g. upgrade prompts). Returns nil when there is nothing to
// report. Wired by cmd/root.go.
var PendingNotice func() map[string]interface{}

func Render(w io.Writer, err error) {
	RenderAs(w, err, "text", "")
}

// RenderAs writes err to w using the given format ("text" or "json"). identity
// is included in the JSON envelope when non-empty.
func RenderAs(w io.Writer, err error, format, identity string) {
	if format == "json" {
		renderJSON(w, err, identity)
		return
	}
	renderText(w, err)
}

func renderText(w io.Writer, err error) {
	var cl *Error
	if !errors.As(err, &cl) {
		fmt.Fprintf(w, "Error: %s\n", err)
		return
	}
	if isTTY() {
		fmt.Fprintf(w, "%sError%s: %s\n", colorRed, colorReset, cl.Message)
		if cl.Hint != "" {
			fmt.Fprintf(w, "%sHint%s:  %s\n", colorYellow, colorReset, cl.Hint)
		}
		return
	}
	fmt.Fprintf(w, "Error: %s\n", cl.Message)
	if cl.Hint != "" {
		fmt.Fprintf(w, "Hint: %s\n", cl.Hint)
	}
}

func renderJSON(w io.Writer, err error, identity string) {
	detail := &ErrDetail{Message: err.Error()}
	var cl *Error
	if errors.As(err, &cl) {
		detail.Type = cl.Type
		if detail.Type == "" {
			detail.Type = TypeAPI
		}
		detail.Message = cl.Message
		detail.Hint = cl.Hint
		detail.Detail = cl.Detail
	} else {
		detail.Type = TypeAPI
	}
	env := &ErrorEnvelope{
		OK:       false,
		Identity: identity,
		Error:    detail,
	}
	if PendingNotice != nil {
		env.Notice = PendingNotice()
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if encErr := enc.Encode(env); encErr != nil {
		fmt.Fprintf(w, "Error: %s\n", err)
		return
	}
	_, _ = buf.WriteTo(w)
}
