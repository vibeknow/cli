package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/vibeknow/cli/internal/clerr"
	"github.com/vibeknow/cli/internal/i18n"
	"github.com/vibeknow/cli/internal/update"
)

var updateCmd = &cobra.Command{
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
		if info == nil {
			fmt.Fprintln(cmd.OutOrStdout(), i18n.T("update.up_to_date", version))
			return nil
		}
		fmt.Fprintln(cmd.OutOrStdout(), info.Message())
		return nil
	},
}

func init() { rootCmd.AddCommand(updateCmd) }
