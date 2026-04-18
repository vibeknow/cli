package output

import (
	"encoding/json"
	"io"
)

type jsonW struct {
	enc *json.Encoder
	w   io.Writer
}

func NewJSON(w io.Writer) *jsonW {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return &jsonW{enc: enc, w: w}
}

func (j *jsonW) Object(payload map[string]any) error {
	out := map[string]any{"schema_version": schemaVersion}
	for k, v := range payload {
		out[k] = v
	}
	return j.enc.Encode(out)
}
