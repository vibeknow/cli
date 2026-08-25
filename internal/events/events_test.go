package events

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestEnabledFor(t *testing.T) {
	cases := []struct {
		name   string
		format string
		env    string
		want   bool
		why    string
	}{
		{"json is the default audience", "json", "", true,
			"json means a program is reading; the prose on stderr has no audience and the events do"},
		{"text stays human", "text", "", false,
			"stderr in text mode is a person's progress display; JSON interleaved into it helps nobody"},
		{"ndjson keeps its stream on stdout", "ndjson", "", false,
			"duplicating the released stdout stream onto stderr would double every event"},
		{"env forces on in text", "text", "1", true, ""},
		{"env forces on with a word", "text", "stderr", true, ""},
		{"env forces off in json", "json", "0", false, ""},
		{"env off by word", "json", "off", false, ""},
		{"unrecognised env falls back to the format", "json", "maybe", true, ""},
		{"env is trimmed and case-insensitive", "text", "  ON  ", true, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := EnabledFor(tc.format, tc.env); got != tc.want {
				t.Fatalf("EnabledFor(%q, %q) = %v, want %v. %s", tc.format, tc.env, got, tc.want, tc.why)
			}
		})
	}
}

func TestEmit_WritesOnePrefixedJSONLine(t *testing.T) {
	var buf bytes.Buffer
	New(&buf, "json", "").Emit(map[string]any{"type": "node.started", "node": "parse"})

	out := buf.String()
	if !strings.HasPrefix(out, Prefix) {
		t.Fatalf("event must carry the %q prefix so it is separable from human text: %q", Prefix, out)
	}
	if n := strings.Count(out, "\n"); n != 1 {
		t.Fatalf("one event must be one line, got %d newlines in %q", n, out)
	}

	var got map[string]any
	if err := json.Unmarshal([]byte(strings.TrimPrefix(out, Prefix)), &got); err != nil {
		t.Fatalf("everything after the prefix must parse as JSON: %v (%q)", err, out)
	}
	if got["type"] != "node.started" || got["node"] != "parse" {
		t.Fatalf("payload lost in transit: %v", got)
	}
	// schema_version and ts come from the shared writer; a consumer that
	// already handles the ndjson stream should not need a second shape.
	for _, k := range []string{"schema_version", "ts"} {
		if _, ok := got[k]; !ok {
			t.Fatalf("%q missing — stderr events must be the same shape as the stdout stream", k)
		}
	}
}

func TestEmit_DisabledWritesNothing(t *testing.T) {
	var buf bytes.Buffer
	e := New(&buf, "text", "")
	if e.Enabled() {
		t.Fatal("text mode should leave the channel off")
	}
	e.Emit(map[string]any{"type": "node.started"})
	if buf.Len() != 0 {
		t.Fatalf("a disabled channel must be silent, wrote %q", buf.String())
	}
}

// A nil Emitter is reachable from any caller that did not build one; it
// must not panic, because the alternative is a nil check at every emit site
// and one of them will be forgotten.
func TestEmit_NilIsSafe(t *testing.T) {
	var e *Emitter
	if e.Enabled() {
		t.Fatal("nil Emitter should report disabled")
	}
	e.Emit(map[string]any{"type": "x"})
}

// Events are advisory. A stream that cannot be written must not become the
// caller's problem, because the caller's only sane response would be to
// ignore it anyway.
func TestEmit_WriteFailureIsSwallowed(t *testing.T) {
	e := New(failWriter{}, "json", "")
	e.Emit(map[string]any{"type": "node.started"})
}

type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errFail }

var errFail = writeErr("stderr is closed")

type writeErr string

func (e writeErr) Error() string { return string(e) }
