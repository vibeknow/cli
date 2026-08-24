package jobs

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/vibeknow/cli/internal/clerr"
	"github.com/vibeknow/cli/internal/cmdutil"
	"github.com/vibeknow/cli/internal/durfmt"
	"github.com/vibeknow/cli/internal/jobs"
)

var (
	flagPruneOlderThan string
	flagPruneTerminal  bool
	flagPruneAll       bool
)

var pruneCmd = &cobra.Command{
	// Takes no positional arguments. Without this cobra accepts and
	// silently discards them, so a stray argument looks like success.
	Args:  cobra.NoArgs,
	Use:   "prune",
	Short: "drop recorded runs from the local ledger",
	Long: `prune removes entries from the local ledger. It never touches the
backend: pruning a run does not cancel it, delete the work, or refund
credits — it only forgets the pointer to it.

At least one filter is required, so a bare ` + "`vk jobs prune`" + ` cannot
silently discard the pointer to a run still in flight.`,
	Example: `  vk jobs prune --older-than 30d
  vk jobs prune --terminal
  vk jobs prune --all --output json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if flagPruneOlderThan == "" && !flagPruneTerminal && !flagPruneAll {
			return clerr.Validation("--older-than, --terminal, or --all is required; refusing to prune the whole ledger by default").
				WithHint("`vk jobs prune --terminal` drops finished runs and keeps anything still in flight")
		}
		opts := jobs.PruneOptions{TerminalOnly: flagPruneTerminal, All: flagPruneAll}
		if flagPruneOlderThan != "" {
			d, err := durfmt.ParseAge(flagPruneOlderThan)
			if err != nil {
				return clerr.Validationf("--older-than: %v (expected a form like 30d, 24h, 90m)", err)
			}
			opts.OlderThan = d
		}

		removed, kept, err := jobs.Prune(opts)
		if err != nil {
			return err
		}
		return cmdutil.Emit(cmd, map[string]any{
			"removed":   removed,
			"remaining": len(kept),
		}, "jobs.pruned", func(w io.Writer) {
			fmt.Fprintf(w, "removed=%d remaining=%d\n", removed, len(kept))
		})
	},
}

func init() {
	pruneCmd.Flags().StringVar(&flagPruneOlderThan, "older-than", "", "drop runs not updated within this window (e.g. 30d, 24h)")
	pruneCmd.Flags().BoolVar(&flagPruneTerminal, "terminal", false, "drop runs that already succeeded or failed")
	pruneCmd.Flags().BoolVar(&flagPruneAll, "all", false, "drop every recorded run")
}
