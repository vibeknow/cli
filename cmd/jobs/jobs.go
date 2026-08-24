// Package jobs exposes the local run ledger as `vk jobs`.
package jobs

import (
	"github.com/spf13/cobra"
)

// Cmd is the `vk jobs` command group.
var Cmd = &cobra.Command{
	Use:   "jobs",
	Short: "inspect the local ledger of video runs",
	Long: `jobs reads the local record of every run started by ` + "`vk create`" + `.

Each figlens run is addressed by a (task_id, session_id) pair. The ledger
remembers that pair so a disconnected or restarted caller can find a run
that is still in flight instead of starting a second one, and so the video
subcommands can resolve --session-id on their own.

The ledger is local and advisory: the backend owns run state. An entry
missing from it means "not recorded here", never "does not exist".`,
}

func init() {
	Cmd.AddCommand(listCmd)
	Cmd.AddCommand(getCmd)
	Cmd.AddCommand(pruneCmd)
}
