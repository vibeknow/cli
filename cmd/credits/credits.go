package credits

import "github.com/spf13/cobra"

var Cmd = &cobra.Command{
	Use: "credits",
	// NoArgs turns `vk credits <typo>` into cobra's "unknown command" error
	// (exit 2). Without it a group command with no Run falls back to
	// printing its help on stdout and exiting 0 — a malformed command
	// reported as success, with help text where a caller expected data.
	Args: cobra.NoArgs,
	// Present so the command counts as Runnable: cobra short-circuits a
	// non-runnable command straight to help WITHOUT validating args, which
	// is how `vk <group> <typo>` used to exit 0. With this, NoArgs runs
	// first and a bare `vk <group>` still prints help.
	RunE:  func(cmd *cobra.Command, args []string) error { return cmd.Help() },
	Short: "manage credits and billing",
}

func init() {
	Cmd.AddCommand(balanceCmd)
}
