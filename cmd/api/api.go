// Package api provides the `vibeknow api` subtree for raw backend calls.
package api

import "github.com/spf13/cobra"

var Cmd = &cobra.Command{
	Use:   "api",
	Short: "raw backend API access (escape hatch)",
}

func init() {
	Cmd.AddCommand(callCmd)
}
