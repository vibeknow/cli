package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "update the CLI (not implemented in P0)",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("update: not implemented in P0; use `npm update -g @vibeknow/cli`")
	},
}

func init() { rootCmd.AddCommand(updateCmd) }
