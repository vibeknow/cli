package output

import (
	"encoding/json"
	"io"
	"time"
)

type ndjsonW struct{ enc *json.Encoder }

func NewNDJSON(w io.Writer) *ndjsonW {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return &ndjsonW{enc: enc}
}

func (n *ndjsonW) Format() string { return "ndjson" }

func (n *ndjsonW) Event(evt map[string]any) error {
	out := map[string]any{
		"schema_version": schemaVersion,
		"ts":             time.Now().UTC().Format(time.RFC3339Nano),
	}
	for k, v := range evt {
		out[k] = v
	}
	return n.enc.Encode(out)
}
