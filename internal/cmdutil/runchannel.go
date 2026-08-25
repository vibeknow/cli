package cmdutil

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/vibeknow/cli/internal/events"
	"github.com/vibeknow/cli/internal/preview"
)

// RunChannel bundles the two side channels a long-running command writes
// to besides its result: structured progress on stderr, and preview files
// on disk.
//
// They are built together because they answer the same question — "what is
// happening, and what can I show the user right now" — and because the
// preview deliverer announces itself through the event channel, so one
// cannot be configured without the other.
type RunChannel struct {
	// Events is never nil; a disabled channel is a no-op Emitter.
	Events *events.Emitter
	// Previews is nil when no --preview-dir was given. Its methods tolerate
	// a nil receiver, so callers do not branch.
	Previews *preview.Deliverer
}

// NewRunChannel wires both channels for cmd. previewDir is the raw
// --preview-dir value; empty disables preview delivery.
//
// A bad --preview-dir is the one failure here that is worth returning: the
// caller asked for files in a place that cannot hold them, and silently
// producing no files would look identical to a run that had none.
func NewRunChannel(cmd *cobra.Command, previewDir string) (*RunChannel, error) {
	format, _ := cmd.Flags().GetString("output")
	em := events.New(cmd.ErrOrStderr(), format, os.Getenv(events.EnvEvents))
	d, err := preview.New(previewDir, em.Emit)
	if err != nil {
		return nil, err
	}
	return &RunChannel{Events: em, Previews: d}, nil
}

// Structured reports whether progress is going out as machine events. When
// it is, commands skip the human prose they would otherwise write for the
// same event, so stderr does not carry both renderings of one fact.
func (c *RunChannel) Structured() bool { return c != nil && c.Events.Enabled() }

// Emit forwards one event to the structured channel.
func (c *RunChannel) Emit(fields map[string]any) {
	if c == nil {
		return
	}
	c.Events.Emit(fields)
}
