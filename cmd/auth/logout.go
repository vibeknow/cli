package auth

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/shiliu-ai/vibeknow-cli/internal/cliauth"
	"github.com/shiliu-ai/vibeknow-cli/internal/keychain"
)

var logoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "clear stored credential for the current profile",
	RunE: func(cmd *cobra.Command, args []string) error {
		p, err := cliauth.CurrentProfile()
		if err != nil {
			return err
		}
		if p.CredentialRef == "" {
			return fmt.Errorf("current profile has no credential_ref configured")
		}
		kc, err := keychain.OpenFor("vibeknow")
		if err != nil {
			return err
		}
		if err := kc.Delete(p.CredentialRef); err != nil {
			fmt.Fprintln(cmd.ErrOrStderr(), "note: credential was not present in keychain (may have been env-only)")
		} else {
			fmt.Printf("cleared keychain entry %q\n", p.CredentialRef)
		}
		fmt.Fprintln(cmd.ErrOrStderr(), "note: VIBEKNOW_TOKEN env var (if set) remains — unset it manually to complete logout")
		return nil
	},
}
