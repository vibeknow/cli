package auth

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/vibeknow/cli/internal/cliauth"
	"github.com/vibeknow/cli/internal/cmdutil"
	"github.com/vibeknow/cli/internal/keychain"
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
		cleared := kc.Delete(p.CredentialRef) == nil
		if !cleared {
			fmt.Fprintln(cmd.ErrOrStderr(), "note: credential was not present in keychain (may have been env-only)")
		}
		envTokenSet := os.Getenv("VIBEKNOW_TOKEN") != ""
		if envTokenSet {
			fmt.Fprintln(cmd.ErrOrStderr(), "note: VIBEKNOW_TOKEN env var is set and still overrides the keychain — unset it to complete logout")
		}
		// env_token_still_set is in the payload because logout does not
		// actually log you out while it is true, and the caller has no other
		// way to find that out from the exit code.
		return cmdutil.Emit(cmd, map[string]any{
			"profile":             p.Name,
			"credential_ref":      p.CredentialRef,
			"cleared":             cleared,
			"env_token_still_set": envTokenSet,
		}, "auth.logout", func(w io.Writer) {
			if cleared {
				fmt.Fprintf(w, "cleared keychain entry %q\n", p.CredentialRef)
			}
		})
	},
}
