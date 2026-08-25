// Package events is the structured progress channel on stderr.
//
// The CLI used to make a caller choose between watching a run and getting a
// parseable result: `--output json` printed nothing until the end, and
// `--output ndjson` put the progress stream on stdout, where it displaced
// the final answer — a consumer had to read the whole stream and work out
// for itself which line was terminal.
//
// The split here removes the choice. stdout carries the answer and nothing
// else; stderr carries the run as it happens, as `vk_event={...}` lines
// mixed in with whatever human text the command already wrote. A caller
// that wants progress greps stderr for the prefix; a caller that only wants
// the result reads stdout and never has to care.
//
// The events are advisory. Losing one changes nothing about the outcome, so
// nothing here returns an error to its caller: an unwritable stderr must not
// turn a finished render into a failed command.
package events

import (
	"io"
	"strings"
	"sync"

	"github.com/vibeknow/cli/internal/output"
)

// Prefix marks a structured line on an otherwise human-readable stream.
const Prefix = "vk_event="

// EnvEvents forces the channel on or off regardless of --output.
//
// Accepted: 1/on/true/stderr to enable, 0/off/false to disable. Anything
// else (including unset) leaves the format-derived default in place.
const EnvEvents = "VIBEKNOW_EVENTS"

// Emitter writes structured progress lines. The zero value and a nil
// pointer are both usable and do nothing, so callers never need a branch
// around a disabled channel.
type Emitter struct {
	mu sync.Mutex
	w  eventWriter
}

// eventWriter is the narrow slice of the output package this needs. Named
// locally because output's writers are unexported concrete types.
type eventWriter interface {
	Event(map[string]any) error
}

// New returns an Emitter for the given format, honouring EnvEvents.
// A disabled channel yields a non-nil Emitter whose Emit is a no-op.
func New(w io.Writer, format, env string) *Emitter {
	if !EnabledFor(format, env) {
		return &Emitter{}
	}
	return &Emitter{w: output.NewPrefixed(w, Prefix)}
}

// EnabledFor reports whether the structured channel is on.
//
// The default is tied to --output json rather than being always-on: in text
// mode stderr is a person's progress display, and interleaving JSON into it
// would trade a working human interface for one nobody asked for. json mode
// already means "a program is reading this", so the prose on stderr has no
// audience and the events do.
//
// ndjson keeps its stream on stdout: that shape is released and callers
// parse it, so duplicating it onto stderr would double every event for
// anyone reading both.
func EnabledFor(format, env string) bool {
	switch strings.ToLower(strings.TrimSpace(env)) {
	case "1", "on", "true", "stderr":
		return true
	case "0", "off", "false", "none":
		return false
	}
	return format == output.FormatJSON
}

// Enabled reports whether Emit will write anything. Callers use it to skip
// work that only exists to feed the channel, never to decide whether to
// call Emit.
func (e *Emitter) Enabled() bool { return e != nil && e.w != nil }

// Emit writes one event. Errors are dropped: this channel is advisory, and
// a caller that has to handle "the progress line did not write" is a caller
// that will get it wrong.
func (e *Emitter) Emit(fields map[string]any) {
	if !e.Enabled() {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	_ = e.w.Event(fields)
}
