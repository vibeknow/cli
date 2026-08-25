package output

import (
	"bytes"
	"encoding/json"
	"io"
	"time"
)

type ndjsonW struct {
	w      io.Writer
	prefix string
}

func NewNDJSON(w io.Writer) *ndjsonW { return &ndjsonW{w: w} }

// NewPrefixed writes the same stamped event lines behind a fixed prefix.
//
// It exists so structured events can share a stream with human text: on
// stdout an NDJSON line is the whole contract and needs no marker, but on
// stderr it sits next to progress prose, warnings and hints. A prefix lets
// a consumer pick the machine lines out by string match without having to
// attempt a JSON parse of every line it sees.
func NewPrefixed(w io.Writer, prefix string) *ndjsonW { return &ndjsonW{w: w, prefix: prefix} }

func (n *ndjsonW) Event(evt map[string]any) error {
	out := map[string]any{
		"schema_version": schemaVersion,
		"ts":             time.Now().UTC().Format(time.RFC3339Nano),
	}
	for k, v := range evt {
		out[k] = v
	}

	// Encoded into a buffer and written once, rather than straight through
	// an Encoder bound to w: an event must reach the stream as a single
	// write so two emitters can never interleave halves of a line, and the
	// prefix must not be separable from the payload it labels.
	var buf bytes.Buffer
	buf.WriteString(n.prefix)
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(out); err != nil {
		return err
	}
	_, err := n.w.Write(buf.Bytes())
	return err
}
