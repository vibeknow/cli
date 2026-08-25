package video

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"github.com/vibeknow/cli/client/figlens"
	"github.com/vibeknow/cli/internal/clerr"
	"github.com/vibeknow/cli/internal/cmdutil"
	"github.com/vibeknow/cli/internal/jobs"
)

func newFiglensClient() (*figlens.Client, error) {
	_, url, tp, err := cmdutil.Default().Service("figlens")
	if err != nil {
		return nil, err
	}
	return figlens.New(url, tp), nil
}

// noteJob records an observed state change, best-effort. The ledger is an
// index, not the source of truth: failing to update it must never turn a
// finished run into a reported failure.
func noteJob(taskID int64, sessionID string, mutate func(*jobs.Record)) {
	if taskID == 0 && sessionID == "" {
		return
	}
	if err := jobs.Update(taskID, sessionID, mutate); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not update the local run ledger: %v\n", err)
	}
}

// resolveTarget turns the optional [task_id] argument and the --session-id
// flag into the (task_id, session_id) pair every video subcommand needs.
//
// Both used to be the caller's problem: --session-id was mandatory, so a
// caller who had lost it — an agent whose context was trimmed, a user who
// closed the terminal — could not reach their own run at all. The local
// ledger now supplies the missing half:
//
//	vk video wait                 → the most recent recorded run
//	vk video wait 42              → session_id looked up for task 42
//	vk video wait 42 --session-id → the flag wins; no ledger read
//
// An explicit --session-id is always authoritative. The ledger only fills
// gaps, so a stale entry can never override what the caller passed.
//
// When the ledger cannot fill the gap either, the backend gets the last
// word. The ledger is per-machine: an agent that moved hosts, a container
// that was rebuilt, a colleague picking the work up — all of them have a
// run they can reach through the account and no local file that mentions
// it. "This machine has no note of it" is not "it does not exist", and the
// command should not act as if it were.
func resolveTarget(ctx context.Context, args []string, sessionFlag string) (int64, string, error) {
	var taskID int64
	if len(args) == 1 {
		parsed, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return 0, "", clerr.Validationf("task_id must be an integer: %v", err)
		}
		taskID = parsed
	}
	if sessionFlag != "" {
		return taskID, sessionFlag, nil
	}

	var (
		rec   jobs.Record
		found bool
		err   error
	)
	if taskID != 0 {
		rec, found, err = jobs.Get(taskID)
	} else {
		rec, found, err = jobs.Latest()
	}
	if err != nil {
		// A broken ledger must not become a broken command: fall through to
		// the backend, then to the same "pass --session-id" instruction.
		fmt.Fprintf(os.Stderr, "warning: could not read the local run ledger: %v\n", err)
		found = false
	}
	if found && rec.SessionID != "" {
		if taskID == 0 {
			taskID = rec.TaskID
		}
		fmt.Fprintf(os.Stderr, "using recorded run: task_id=%d session_id=%s\n", taskID, rec.SessionID)
		return taskID, rec.SessionID, nil
	}

	if taskID != 0 {
		// A task_id cannot be resolved remotely: the backend's work list is
		// keyed by work_id and session_id and carries no task_id, so there
		// is nothing to match against. Say what can actually be done rather
		// than pretending to look.
		return 0, "", clerr.Validationf("no session_id recorded for task %d", taskID).
			WithHint("pass --session-id explicitly, or run `vk video list` to find the session — the backend list is keyed by session, not task")
	}

	if sess, ok := latestRemoteSession(ctx); ok {
		fmt.Fprintf(os.Stderr, "no local record; using the most recent run on this account: session_id=%s\n", sess)
		return 0, sess, nil
	}

	return 0, "", clerr.Validation("no task_id given, and neither the local ledger nor this account has a run to fall back on").
		WithHint("pass a task_id and --session-id, or run `vk create` first — `vk jobs list` shows local records and `vk video list` shows the account's")
}

// latestRemoteSession returns the newest work's session_id from the
// backend. Failure is silent by design: this is the fallback path, and the
// caller's next move on a miss is the same instruction it would have got
// anyway.
func latestRemoteSession(ctx context.Context) (string, bool) {
	c, err := newFiglensClient()
	if err != nil {
		return "", false
	}
	works, _, err := c.ListWorks(ctx, 1, 1)
	if err != nil || len(works) == 0 || works[0].SessionID == "" {
		return "", false
	}
	return works[0].SessionID, true
}
