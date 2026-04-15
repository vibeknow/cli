package auth

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/shiliu-ai/vibeknow-cli/internal/config"
	"github.com/shiliu-ai/vibeknow-cli/internal/keychain"
)

var logoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "clear stored credential for the current profile",
	RunE: func(cmd *cobra.Command, args []string) error {
		f, err := config.LoadProfiles()
		if err != nil {
			return err
		}
		if f.Current == "" {
			return fmt.Errorf("no active profile")
		}
		var prof *config.Profile
		for i := range f.Profiles {
			if f.Profiles[i].Name == f.Current {
				prof = &f.Profiles[i]
				break
			}
		}
		if prof == nil || prof.CredentialRef == "" {
			return fmt.Errorf("current profile has no credential_ref configured")
		}
		kc, err := keychain.OpenFor("vibeknow")
		if err != nil {
			return err
		}
		if err := kc.Delete(prof.CredentialRef); err != nil {
			fmt.Fprintln(cmd.ErrOrStderr(), "note: credential was not present in keychain (may have been env-only)")
		} else {
			fmt.Printf("cleared keychain entry %q\n", prof.CredentialRef)
		}
		fmt.Fprintln(cmd.ErrOrStderr(), "note: VIBEKNOW_TOKEN env var (if set) remains — unset it manually to complete logout")
		return nil
	},
}
