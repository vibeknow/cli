package voice

import "github.com/spf13/cobra"

var Cmd = &cobra.Command{
	Use:   "voice",
	Short: "manage voice templates",
}

func init() {
	Cmd.AddCommand(listCmd)
}
