// Package output provides format-aware writers for CLI command results.
// Supported formats per spec §8.10: text, json, ndjson.
package output

import (
	"fmt"
	"io"
	"strings"
)

type Writer interface {
	Format() string
}

// Select resolves a user-supplied --output flag value with TTY and streaming
// context into a concrete format. See §8.10.
func Select(flag string, isTTY, streaming bool) string {
	if flag != "" {
		return flag
	}
	if isTTY {
		return "text"
	}
	if streaming {
		return "ndjson"
	}
	return "json"
}

// New creates a writer for the given format. Returns an error for
// unsupported formats so the CLI can exit 2 with a clear message.
func New(format string, w io.Writer, isTTY, streaming bool) (Writer, error) {
	switch strings.ToLower(format) {
	case "text":
		return NewText(w), nil
	case "json":
		return NewJSON(w), nil
	case "ndjson":
		return NewNDJSON(w), nil
	case "yaml", "table":
		return nil, fmt.Errorf("output format %q is not implemented yet; supported: text, json, ndjson", format)
	default:
		return nil, fmt.Errorf("unknown output format %q; supported: text, json, ndjson", format)
	}
}

// schemaVersion is stamped on every structured output per spec §11.
const schemaVersion = "1"
