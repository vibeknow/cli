package auth

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/vibeknow/cli/internal/cliauth"
	"github.com/vibeknow/cli/internal/config"
	"github.com/vibeknow/cli/internal/credential"
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
			if p, err := cliauth.CurrentProfile(); err == nil {
				r = cliauth.ResolverFor(p)
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
