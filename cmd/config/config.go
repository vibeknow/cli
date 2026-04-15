// Package config implements the `vibeknow config` command tree.
package config

import "github.com/spf13/cobra"

// Cmd is the parent "config" command.
var Cmd = &cobra.Command{
	Use:   "config",
	Short: "manage vibeknow global config",
}

func init() {
	Cmd.AddCommand(getCmd)
	Cmd.AddCommand(setCmd)
	Cmd.AddCommand(listCmd)
}
