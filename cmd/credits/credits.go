package credits

import "github.com/spf13/cobra"

var Cmd = &cobra.Command{
	Use:   "credits",
	Short: "manage credits and billing",
}

func init() {
	Cmd.AddCommand(balanceCmd)
}
