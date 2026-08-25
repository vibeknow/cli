package cmdutil

import (
	"context"

	"github.com/vibeknow/cli/client/figlens"
	"github.com/vibeknow/cli/internal/clerr"
	"github.com/vibeknow/cli/internal/errs"
	"github.com/vibeknow/cli/internal/video/snapshot"
)

// Delivery states for a run whose outcome the CLI could not observe.
//
// Exit 6 has always said "the state is unknown", which is true and not
// actionable: the caller's real question is whether re-running would create
// a second billed render or recover a lost one, and those need opposite
// answers. These three values answer that question directly.
const (
	// DeliverySubmitted: the backend has this run. Re-running the original
	// command would start a second one.
	DeliverySubmitted = "submitted"
	// DeliveryNotSubmitted: the backend has no record of it, so nothing was
	// billed and nothing is in flight. Safe to run again.
	DeliveryNotSubmitted = "not_submitted"
	// DeliveryIndeterminate: the CLI could not find out. Not safe to
	// re-run — check first.
	DeliveryIndeterminate = "indeterminate"
)

// UnknownRun is what the CLI can honestly say about a run it lost track of.
type UnknownRun struct {
	TaskID    int64
	SessionID string
	Delivery  string
	// BackendStatus is the work row's state when it could be read, as a
	// word rather than the wire integer.
	BackendStatus string
}

// ResendSafe reports whether starting the work over cannot double-bill.
//
// Only an outright "the backend has never heard of this" qualifies.
// Indeterminate deliberately answers false: the cost of waiting when a run
// was already lost is a delay, and the cost of resending when it was not is
// the user's money.
func (u UnknownRun) ResendSafe() bool { return u.Delivery == DeliveryNotSubmitted }

// workReader is the slice of figlens.Client this file needs, named locally
// so the probe can be tested without a backend.
type workReader interface {
	GetWorkBySession(ctx context.Context, sessionID string) (*figlens.Work, error)
}

// ProbeRun asks the backend what it knows about a run the CLI stopped
// observing.
//
// The CLI's own silence proves nothing: an empty event stream looks the
// same whether the task was never dispatched, the session id was wrong, or
// the connection dropped before the first message. One read of the work row
// separates those, and it is the difference between telling a caller to
// reattach and telling it to start over.
//
// It never fails. A probe that cannot reach the backend reports
// indeterminate, which is the honest answer and the safe one.
func ProbeRun(ctx context.Context, c workReader, taskID int64, sessionID string) UnknownRun {
	u := UnknownRun{TaskID: taskID, SessionID: sessionID, Delivery: DeliveryIndeterminate}
	if c == nil || sessionID == "" {
		return u
	}
	w, err := c.GetWorkBySession(ctx, sessionID)
	switch {
	case errs.HasCode(err, "not_found"):
		// The work row is created by task init, before anything runs. Its
		// absence means this session was never initialised, so there is
		// nothing in flight and nothing has been billed against it.
		u.Delivery = DeliveryNotSubmitted
	case err != nil:
		// Some other failure — auth, network, a 500. The run may well be
		// fine; we simply cannot see it.
		return u
	case w == nil || w.ID == 0:
		u.Delivery = DeliveryNotSubmitted
	default:
		u.Delivery = DeliverySubmitted
		u.BackendStatus = workStatusName(w.Status)
	}
	return u
}

// UnknownStateError builds the exit-6 error for u, carrying the delivery
// verdict as machine-readable detail alongside the human message.
//
// reattach is the command that resumes observation; it becomes both the
// hint and the single next_action, so a caller has one thing to run whether
// it reads prose or JSON.
func UnknownStateError(msg string, u UnknownRun, reattach string) *clerr.Error {
	detail := map[string]any{
		"delivery":    u.Delivery,
		"resend_safe": u.ResendSafe(),
	}
	if u.TaskID != 0 {
		detail["task_id"] = u.TaskID
	}
	if u.SessionID != "" {
		detail["session_id"] = u.SessionID
	}
	if u.BackendStatus != "" {
		detail["backend_status"] = u.BackendStatus
	}
	if reattach != "" {
		detail["next_actions"] = []map[string]string{{
			"command": reattach,
			"purpose": reattachPurpose(u),
		}}
	}
	e := clerr.New(msg).WithCode(6).WithDetail(detail)
	if reattach != "" {
		return e.WithHint(reattach)
	}
	return e
}

func reattachPurpose(u UnknownRun) string {
	switch u.Delivery {
	case DeliverySubmitted:
		return "Reattach to the run the backend still has; do not start a new one"
	case DeliveryNotSubmitted:
		return "Nothing is in flight — starting over will not double-bill"
	default:
		return "Establish what actually happened before deciding whether to re-run"
	}
}

// workStatusName renders the work row's status integer as the word the
// snapshot layer already uses, so one run is not described two ways.
func workStatusName(status int) string {
	switch status {
	case figlens.WorkStatusGenerating:
		return snapshot.StatusRunning
	case figlens.WorkStatusActive:
		return snapshot.StatusSucceeded
	case figlens.WorkStatusFailed:
		return snapshot.StatusFailed
	case figlens.WorkStatusDeleted:
		return "deleted"
	}
	return "unknown"
}
