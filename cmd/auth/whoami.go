package auth

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/shiliu-ai/vibeknow-cli/client/account"
	"github.com/shiliu-ai/vibeknow-cli/internal/config"
	"github.com/shiliu-ai/vibeknow-cli/internal/credential"
	"github.com/shiliu-ai/vibeknow-cli/internal/endpoints"
	"github.com/shiliu-ai/vibeknow-cli/internal/keychain"
)

var whoamiCmd = &cobra.Command{
	Use:   "whoami",
	Short: "print the current authenticated user",
	RunE: func(cmd *cobra.Command, args []string) error {
		p, tok, err := resolveProfileAndToken()
		if err != nil {
			return err
		}
		url, err := endpoints.Resolve(p, "account")
		if err != nil {
			return err
		}
		c := account.New(url, staticToken(tok))
		u, err := c.Whoami(context.Background())
		if err != nil {
			return err
		}
		fmt.Printf("uid: %s\nnickname: %s\nemail: %s\nphone: %s\n", u.UID, u.Nickname, u.Email, u.Phone)
		return nil
	},
}

type staticToken string

func (s staticToken) Token(ctx context.Context) (string, error) { return string(s), nil }

func resolveProfileAndToken() (config.Profile, string, error) {
	f, err := config.LoadProfiles()
	if err != nil {
		return config.Profile{}, "", err
	}
	if f.Current == "" {
		return config.Profile{}, "", fmt.Errorf("no active profile; set one with `vibeknow profile use <name>`")
	}
	var prof *config.Profile
	for i := range f.Profiles {
		if f.Profiles[i].Name == f.Current {
			prof = &f.Profiles[i]
			break
		}
	}
	if prof == nil {
		return config.Profile{}, "", fmt.Errorf("current profile %q not found in profiles list", f.Current)
	}
	r := credential.Resolver{
		Env: credential.EnvSource{Var: "VIBEKNOW_TOKEN"},
	}
	if prof.CredentialRef != "" {
		kc, err := keychain.OpenFor("vibeknow")
		if err == nil {
			r.Keychain = credential.KeychainSource{Keychain: kc, Entry: prof.CredentialRef}
		}
	}
	tok, _, err := r.Resolve()
	if err != nil {
		return *prof, "", fmt.Errorf("no credential available; set VIBEKNOW_TOKEN env var")
	}
	return *prof, tok, nil
}
