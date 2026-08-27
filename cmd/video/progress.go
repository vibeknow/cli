package video

import (
	"strconv"
	"time"

	"github.com/vibeknow/cli/client/figlens"
	"github.com/vibeknow/cli/internal/clerr"
	"github.com/vibeknow/cli/internal/durfmt"
	"github.com/vibeknow/cli/internal/jobs"
	"github.com/vibeknow/cli/internal/stage"
)

// runProgress remembers where a run had got to, so a wait that stops on its
// own budget can say so instead of just failing to finish.
//
// It deliberately keeps only the latest position rather than a history: the
// consumer is a caller that will call again in a moment, and the thing it
// needs to tell a person is "where is this now", not "where has it been".
type runProgress struct {
	startedAt time.Time

	// stage/node are the last pipeline position seen. node is the backend's
	// wire name and gets sanitized through stage.DisplayName before it is
	// shown — the image line's step_ids carry an internal model codename.
	stage string
	node  string

	// nodeDone records whether that node had finished when it was last heard
	// from. Without it the last node is reported the same way whether the run
	// is inside it or long past it, and the difference is the whole answer
	// during a silent stretch: the hand-drawn line finishes script_writing
	// and then draws for minutes without emitting anything, so "outline /
	// script_writing" would be repeated the entire time the run was doing
	// something else.
	nodeDone bool

	// message is the last free-form progress line. The v=2 agent engine emits
	// nothing but these (no step_ids at all), so on that line it is the only
	// thing there is to report.
	message string
}

func (p *runProgress) observe(ev figlens.StreamEvent) {
	if ev.Node != "" {
		p.node = ev.Node
		// A warning does not move the run and does not end the node, so it
		// must not decide whether the node is still open.
		switch ev.Type {
		case "node.started":
			p.nodeDone = false
		case "node.succeeded":
			p.nodeDone = true
		}
	}
	if ev.Stage != "" {
		p.stage = ev.Stage
	}
	if ev.Type == "node.progress" && ev.Message != "" {
		p.message = ev.Message
	}
}

func (p *runProgress) elapsed() time.Duration {
	return time.Since(p.startedAt).Truncate(time.Second)
}

// describe renders the position as a phrase a caller can repeat to a person.
//
// mode is the run's video_kind as the local ledger recorded it, used only to
// explain an absence; "" when unknown, which is never treated as a claim.
func (p *runProgress) describe(mode string) string {
	switch {
	case p.node != "" && !p.nodeDone:
		if p.stage != "" {
			return p.stage + " / " + stage.DisplayName(p.node)
		}
		return stage.DisplayName(p.node)

	case p.node != "" && mode == figlens.VideoKindHandDraw:
		// The hand-drawn line's silent stretch sits between two nodes that do
		// report, so this is where a run spends most of its time. Naming the
		// node it has left says more than repeating it as though the run were
		// still inside it.
		return "past " + stage.DisplayName(p.node) + "; drawing (this line reports no progress until it finishes)"

	case p.node != "":
		// Between nodes on any line. Short in the usual case, but reporting a
		// finished node as the current one is how a run that has moved on
		// looks identical to one that has stopped.
		return "past " + stage.DisplayName(p.node)

	case p.stage != "":
		return p.stage
	case p.message != "":
		return p.message
	case mode == figlens.VideoKindHandDraw:
		// The hand-drawn line registers no step_ids at all, so its nodes emit
		// nothing between the run starting and the run finishing (go-figlens
		// emitNodeEvent returns early for an unregistered id). Reporting
		// "no stage reported yet" every ninety minutes would be accurate and
		// useless: it is the same sentence a stuck run produces, on the one
		// line where the wait is longest and a person is most likely to
		// conclude something has broken and start over — at full price.
		//
		// So the absence gets named as the expected thing it is. This says
		// only what is known: not that the run is healthy, which nothing here
		// can establish, but that silence is not evidence either way.
		return "drawing (this line reports no progress until it finishes)"
	default:
		// The first seconds of any run. Saying "starting up" would be a
		// guess; saying nothing specific is the honest report.
		return "no stage reported yet"
	}
}

// stillRunningError reports a wait that ended because its --for budget ran
// out, not because the run did.
//
// Exit 6 rather than 0: 0 on `video wait` means the task succeeded, and a
// caller that ran `wait && download` must not be told to download a video
// that is still being made. Exit 6 already means "not terminal, reattach
// rather than start over", which is exactly the right next move — the
// `reason` field separates this from the other two things 6 covers, both of
// which are states nobody asked for.
func stillRunningError(taskID int64, sessionID string, p runProgress, budget time.Duration) *clerr.Error {
	// The mode comes from the local ledger rather than a request: it is only
	// needed to explain silence, and a run whose ledger entry is missing is
	// exactly the run that should not be made to wait on another round trip
	// before being told anything at all.
	mode := ""
	if rec, found, err := jobs.Get(taskID); err == nil && found {
		mode = rec.Mode
	}
	detail := map[string]any{
		"status":     "running",
		"reason":     "wait_budget_expired",
		"task_id":    taskID,
		"session_id": sessionID,
		"stage":      p.describe(mode),
		"waited_ms":  p.elapsed().Milliseconds(),
		"next_actions": []map[string]string{{
			"command": "vk video wait " + strconv.FormatInt(taskID, 10) +
				" --session-id " + sessionID + " --for " + budget.String(),
			"purpose": "Keep waiting; the run is still going and no work is lost",
		}},
	}
	// `message` is the free-form line that accompanies a node-based stage. On
	// the agent engine there are no nodes, so `stage` already *is* that line
	// and emitting both puts one string in the payload twice — which reads as
	// two facts and invites a caller to look for a difference between them.
	if p.message != "" && p.message != detail["stage"] {
		detail["message"] = p.message
	}
	return clerr.Newf("still generating (%s) after %s — run again to keep waiting",
		p.describe(mode), durfmt.Short(p.elapsed())).
		WithCode(6).
		WithDetail(detail).
		WithHint("nothing failed and nothing was lost; this wait simply reached its --for budget")
}
