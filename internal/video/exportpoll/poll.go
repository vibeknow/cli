// Package exportpoll owns the polling loop that watches a figlens export
// task through to a terminal state. Extracted from the video command so
// that both cmd/video/export.go and cmd/create.go (--export chain) reuse
// the same backoff, jitter, and cancellation semantics.
package exportpoll

import (
	"context"
	"errors"
	"math/rand"
	"os"
	"time"

	"github.com/vibeknow/cli/client/figlens"
	"github.com/vibeknow/cli/internal/video/snapshot"
)

// DefaultTimeout returns the default timeout for sync-mode polling. Reads
// VIBEKNOW_EXPORT_TIMEOUT if set, else 15 minutes.
func DefaultTimeout() time.Duration {
	if v := os.Getenv("VIBEKNOW_EXPORT_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return 15 * time.Minute
}

// Event is emitted once per poll cycle. Status values mirror
// snapshot.StatusRunning / StatusSucceeded / StatusFailed.
type Event struct {
	Progress    int
	ProgressMsg string
	Status      string
}

// ErrTimeout is returned when the sync poll exceeds its deadline.
var ErrTimeout = errors.New("exportpoll: timeout")

// Client is the minimal figlens surface PollExport needs.
type Client interface {
	GetExportResult(ctx context.Context, exportTaskID string) (*figlens.ExportResult, error)
}

// PollExport polls the backend until the export reaches a terminal state
// (succeeded or failed), the deadline expires, or ctx is cancelled. It
// invokes onEvent after each status read (including the terminal one).
//
// Backoff: starts at 1s, multiplied by 1.5 per cycle, capped at 10s, with
// ±20% jitter. If fixedInterval > 0 the backoff is bypassed and that exact
// interval is used between polls.
func PollExport(
	ctx context.Context,
	c Client,
	exportTaskID string,
	timeout time.Duration,
	fixedInterval time.Duration,
	onEvent func(Event),
) (*figlens.ExportResult, error) {
	deadline := time.Now().Add(timeout)
	interval := time.Second
	for {
		if time.Now().After(deadline) {
			return nil, ErrTimeout
		}
		r, err := c.GetExportResult(ctx, exportTaskID)
		if err != nil {
			return nil, err
		}
		switch r.Status {
		case "completed", "success", "succeeded":
			onEvent(Event{Progress: 100, Status: snapshot.StatusSucceeded})
			return r, nil
		case "failed", "error":
			onEvent(Event{Status: snapshot.StatusFailed, ProgressMsg: r.Error})
			return r, nil
		default:
			onEvent(Event{
				Progress:    r.Progress,
				ProgressMsg: r.ProgressMsg,
				Status:      snapshot.StatusRunning,
			})
		}

		// Decide next wait interval.
		wait := interval
		if fixedInterval > 0 {
			wait = fixedInterval
		} else {
			// Jitter the current interval by ±20%, then grow for next cycle.
			wait = time.Duration(float64(interval) * (0.8 + 0.4*rand.Float64()))
			interval = time.Duration(float64(interval) * 1.5)
			if interval > 10*time.Second {
				interval = 10 * time.Second
			}
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(wait):
		}
	}
}
