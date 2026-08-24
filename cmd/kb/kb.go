// Package kb implements the `vk kb` subcommand family: list, delete, prune.
package kb

import (
	"github.com/spf13/cobra"

	"github.com/vibeknow/cli/internal/i18n"
)

var Cmd = &cobra.Command{
	Use: "kb",
	// NoArgs turns `vk kb <typo>` into cobra's "unknown command" error
	// (exit 2). Without it a group command with no Run falls back to
	// printing its help on stdout and exiting 0 — a malformed command
	// reported as success, with help text where a caller expected data.
	Args: cobra.NoArgs,
	// Present so the command counts as Runnable: cobra short-circuits a
	// non-runnable command straight to help WITHOUT validating args, which
	// is how `vk <group> <typo>` used to exit 0. With this, NoArgs runs
	// first and a bare `vk <group>` still prints help.
	RunE:  func(cmd *cobra.Command, args []string) error { return cmd.Help() },
	Short: i18n.T("kb.short"),
}
