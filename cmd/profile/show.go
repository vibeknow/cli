package profile

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/shiliu-ai/vibeknow-cli/internal/config"
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
				fmt.Printf("name: %s\napi_endpoint: %s\ncredential_ref: %s\ntrust: %s\nis_production: %v\ndefault_project: %s\n",
					p.Name, p.APIEndpoint, p.CredentialRef, p.Trust, p.IsProduction, p.DefaultProject)
				return nil
			}
		}
		return fmt.Errorf("profile %q not found", name)
	},
}
