package ip

import "github.com/spf13/cobra"

var Cmd = &cobra.Command{
	Use: "ip",
	// NoArgs turns `vk ip <typo>` into cobra's "unknown command" error
	// (exit 2) instead of help on stdout and exit 0; see cmd/avatar.
	Args:  cobra.NoArgs,
	RunE:  func(cmd *cobra.Command, args []string) error { return cmd.Help() },
	Short: "list IP characters for `vk create --mode image --ip-asset`",
}

func init() {
	Cmd.AddCommand(listCmd)
}
