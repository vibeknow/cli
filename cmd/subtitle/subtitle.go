package subtitle

import "github.com/spf13/cobra"

// Cmd groups the read-only subtitle catalogs. The commands that *change*
// subtitles live on `vk video set`, because they need a work to change; these
// need nothing but an account, and exist so that `--subtitle-font` and
// `--subtitle-preset` have discoverable values rather than being free text
// the backend refuses.
var Cmd = &cobra.Command{
	Use: "subtitle",
	// NoArgs turns `vk subtitle <typo>` into cobra's "unknown command" error
	// (exit 2). Without it a group command with no Run falls back to
	// printing its help on stdout and exiting 0 — a malformed command
	// reported as success, with help text where a caller expected data.
	Args: cobra.NoArgs,
	// Present so the command counts as Runnable: cobra short-circuits a
	// non-runnable command straight to help WITHOUT validating args, which
	// is how `vk <group> <typo>` used to exit 0. With this, NoArgs runs
	// first and a bare `vk <group>` still prints help.
	RunE:  func(cmd *cobra.Command, args []string) error { return cmd.Help() },
	Short: "list the fonts and ready-made looks `vk video set` accepts for subtitles",
}

func init() {
	Cmd.AddCommand(fontsCmd)
	Cmd.AddCommand(presetsCmd)
}
