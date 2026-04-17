// Package auth provides the `vibeknow auth` command tree.
// P1 includes whoami / status / logout only; login is P1.5.
package auth

import "github.com/spf13/cobra"

var Cmd = &cobra.Command{
	Use:   "auth",
	Short: "manage authentication state",
}

func init() {
	Cmd.AddCommand(whoamiCmd)
	Cmd.AddCommand(statusCmd)
	Cmd.AddCommand(logoutCmd)
	Cmd.AddCommand(loginCmd)
}
