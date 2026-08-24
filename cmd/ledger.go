package cmd

import (
	"fmt"
	"os"

	"github.com/vibeknow/cli/internal/jobs"
)

// recordJob and updateJob wrap the ledger writes `vk create` makes.
//
// Both are best-effort by design: the ledger is a local convenience index,
// and a run that generated a video successfully must not be reported as a
// failure because a JSONL line could not be appended. The failure is still
// announced on stderr rather than swallowed — a ledger that has silently
// stopped recording is worse than one that says so, since the whole point
// is being able to find a run later.
func recordJob(r jobs.Record) {
	if err := jobs.Append(r); err != nil {
		warnLedger(err)
	}
}

func updateJob(taskID int64, sessionID string, mutate func(*jobs.Record)) {
	if err := jobs.Update(taskID, sessionID, mutate); err != nil {
		warnLedger(err)
	}
}

func warnLedger(err error) {
	fmt.Fprintf(os.Stderr,
		"warning: could not write the local run ledger (%v); `vk jobs` will not see this run — keep the task_id/session_id above\n", err)
}
