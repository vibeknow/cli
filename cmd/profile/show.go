package profile

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

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
			if p.Name == name {
				fmt.Printf("name: %s\ntrust: %s\nis_production: %v\ncredential_ref: %s\ndefault_project: %s\n",
					p.Name, p.Trust, p.IsProduction, p.CredentialRef, p.DefaultProject)
				fmt.Println("endpoints:")
				if len(p.Endpoints) == 0 {
					fmt.Println("  (all using cloud defaults)")
				} else {
					for _, k := range []string{"account", "vectoria", "figlens", "vibeknow"} {
						if v, ok := p.Endpoints[k]; ok {
							fmt.Printf("  %s: %s\n", k, v)
						}
					}
				}
				if p.APIEndpoint != "" {
					fmt.Fprintln(os.Stderr, "warning: api_endpoint is deprecated; use endpoints.vibeknow")
				}
				return nil
			}
		}
		return fmt.Errorf("profile %q not found", name)
	},
}
