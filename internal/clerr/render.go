package clerr

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"golang.org/x/term"

	"github.com/vibeknow/cli/internal/charcheck"
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

// RenderAs writes err to w using the given format ("text", "json" or
// "ndjson"). identity is included in the JSON envelope when non-empty.
// ndjson gets the same envelope on a single line: a caller reading one JSON
// value per line cannot parse an indented one, and text would hide the
// error type it branches on.
func RenderAs(w io.Writer, err error, format, identity string) {
	// A silent error carries an exit code and nothing else; the command has
	// already written its own complete output.
	var silent *Error
	if errors.As(err, &silent) && silent.Silent {
		return
	}
	if format == "json" || format == "ndjson" {
		renderJSON(w, err, identity, format == "json")
		return
	}
	renderText(w, err)
}

func renderText(w io.Writer, err error) {
	// Messages and hints carry backend text verbatim (a refusal reason, a
	// title); a terminal must not interpret any of it. Strip the values, not
	// the writer, so the colours below survive.
	var cl *Error
	if !errors.As(err, &cl) {
		fmt.Fprintf(w, "Error: %s\n", charcheck.Strip(err.Error()))
		return
	}
	msg, hint := charcheck.Strip(cl.Message), charcheck.Strip(cl.Hint)
	if isTTY() {
		fmt.Fprintf(w, "%sError%s: %s\n", colorRed, colorReset, msg)
		if hint != "" {
			fmt.Fprintf(w, "%sHint%s:  %s\n", colorYellow, colorReset, hint)
		}
		return
	}
	fmt.Fprintf(w, "Error: %s\n", msg)
	if hint != "" {
		fmt.Fprintf(w, "Hint: %s\n", hint)
	}
}

func renderJSON(w io.Writer, err error, identity string, indent bool) {
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
	if indent {
		enc.SetIndent("", "  ")
	}
	if encErr := enc.Encode(env); encErr != nil {
		fmt.Fprintf(w, "Error: %s\n", err)
		return
	}
	_, _ = buf.WriteTo(w)
}
