package auth

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/shiliu-ai/vibeknow-cli/internal/config"
	"github.com/shiliu-ai/vibeknow-cli/internal/credential"
	"github.com/shiliu-ai/vibeknow-cli/internal/keychain"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "show credential source and active profile",
	RunE: func(cmd *cobra.Command, args []string) error {
		f, err := config.LoadProfiles()
		if err != nil {
			return err
		}
		fmt.Printf("active profile: %s\n", orNone(f.Current))
		r := credential.Resolver{Env: credential.EnvSource{Var: "VIBEKNOW_TOKEN"}}
		if f.Current != "" {
			for _, p := range f.Profiles {
				if p.Name == f.Current && p.CredentialRef != "" {
					if kc, err := keychain.OpenFor("vibeknow"); err == nil {
						r.Keychain = credential.KeychainSource{Keychain: kc, Entry: p.CredentialRef}
					}
					break
				}
			}
		}
		_, src, err := r.Resolve()
		if err != nil {
			fmt.Println("credential: none (set VIBEKNOW_TOKEN or configure credential_ref)")
			return nil
		}
		fmt.Printf("credential source: %s\n", src)
		return nil
	},
}

func orNone(s string) string {
	if s == "" {
		return "(none)"
	}
	return s
}
