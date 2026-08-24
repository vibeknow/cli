package cmd

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/vibeknow/cli/internal/clerr"
	"github.com/vibeknow/cli/internal/cmdutil"
	"github.com/vibeknow/cli/internal/i18n"
	"github.com/vibeknow/cli/internal/update"
)

var updateCmd = &cobra.Command{
	// Takes no positional arguments. Without this cobra accepts and
	// silently discards them, so a stray argument looks like success.
	Args:  cobra.NoArgs,
	Use:   "update",
	Short: "check for a newer vibeknow-cli release",
	Long: `Contact the npm registry and report whether a newer vibeknow-cli
release is available. This command never installs the update itself; when a
new version is found it prints the recommended 'npm update -g' command.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Fprintln(cmd.ErrOrStderr(), i18n.T("update.checking"))

		info, ok := update.CheckLatest(version)
		if !ok {
			return clerr.Network(i18n.T("update.check_fail"))
		}
		payload := map[string]any{
			"current":          version,
			"update_available": info != nil,
		}
		if info != nil {
			payload["latest"] = info.Latest
		}
		return cmdutil.Emit(cmd, payload, "update.check", func(w io.Writer) {
			if info == nil {
				fmt.Fprintln(w, i18n.T("update.up_to_date", version))
				return
			}
			fmt.Fprintln(w, info.Message())
		})
	},
}

func init() { rootCmd.AddCommand(updateCmd) }
