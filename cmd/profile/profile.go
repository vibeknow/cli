// Package profile provides the `vibeknow profile` command tree.
package profile

import "github.com/spf13/cobra"

// Cmd is the parent "profile" command, added to root by cmd.init().
var Cmd = &cobra.Command{
	Use:   "profile",
	Short: "manage vibeknow profiles",
}

func init() {
	Cmd.AddCommand(addCmd)
	Cmd.AddCommand(listCmd)
	Cmd.AddCommand(useCmd)
	Cmd.AddCommand(removeCmd)
	Cmd.AddCommand(showCmd)
}
