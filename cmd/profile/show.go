package profile

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/vibeknow/cli/internal/cmdutil"
	"github.com/vibeknow/cli/internal/config"
)

var showCmd = &cobra.Command{
	Use:   "show [NAME]",
	Short: "show profile details (default: current)",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		f, err := config.LoadProfiles()
		if err != nil {
			return err
		}
		name := f.Current
		if len(args) == 1 {
			name = args[0]
		}
		for _, p := range f.Profiles {
			if p.Name != name {
				continue
			}
			if p.APIEndpoint != "" {
				fmt.Fprintln(cmd.ErrOrStderr(), "warning: api_endpoint is deprecated; use endpoints.vibeknow")
			}
			return cmdutil.Emit(cmd, map[string]any{
				"name":            p.Name,
				"current":         p.Name == f.Current,
				"trust":           p.Trust,
				"is_production":   p.IsProduction,
				"credential_ref":  p.CredentialRef,
				"default_project": p.DefaultProject,
				"endpoints":       p.Endpoints,
			}, "profile.show", func(w io.Writer) {
				fmt.Fprintf(w, "name: %s\ntrust: %s\nis_production: %v\ncredential_ref: %s\ndefault_project: %s\n",
					p.Name, p.Trust, p.IsProduction, p.CredentialRef, p.DefaultProject)
				fmt.Fprintln(w, "endpoints:")
				if len(p.Endpoints) == 0 {
					fmt.Fprintln(w, "  (all using cloud defaults)")
					return
				}
				for _, k := range []string{"account", "vectoria", "figlens", "vibeknow"} {
					if v, ok := p.Endpoints[k]; ok {
						fmt.Fprintf(w, "  %s: %s\n", k, v)
					}
				}
			})
		}
		return fmt.Errorf("profile %q not found", name)
	},
}
