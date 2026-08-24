package cmdutil

import (
	"io"

	"github.com/spf13/cobra"

	"github.com/vibeknow/cli/internal/output"
)

// Emit renders one command result in whatever format the caller asked for.
//
// It exists because most commands need the same three-way switch and the
// ones that skipped it simply had no JSON at all: `--output json` was
// accepted, ignored, and human prose came out on stdout — parseable by
// nobody and indistinguishable from success-with-structured-output.
//
// eventType is the "type" field stamped on the NDJSON line (NDJSON is a
// stream of typed events; a bare object would not fit that contract).
// Pass "" to fall back to the JSON object shape for ndjson too.
//
// text writes the human form and is called only for the text format, so
// callers can keep whatever bespoke layout they already had.
func Emit(cmd *cobra.Command, payload map[string]any, eventType string, text func(w io.Writer)) error {
	format, _ := cmd.Flags().GetString("output")
	stdout := cmd.OutOrStdout()
	switch format {
	case output.FormatJSON:
		return output.NewJSON(stdout).Object(payload)
	case output.FormatNDJSON:
		if eventType != "" {
			// Copy so a caller reusing payload afterwards doesn't observe
			// the injected field.
			ev := make(map[string]any, len(payload)+1)
			for k, v := range payload {
				ev[k] = v
			}
			ev["type"] = eventType
			return output.NewNDJSON(stdout).Event(ev)
		}
		return output.NewNDJSON(stdout).Event(payload)
	default:
		if text != nil {
			text(stdout)
		}
		return nil
	}
}
