package profile

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/shiliu-ai/vibeknow-cli/internal/config"
	"github.com/shiliu-ai/vibeknow-cli/internal/i18n"
)

var addFlags struct {
	apiEndpoint    string
	credentialRef  string
	defaultProject string
	trust          string
	isProduction   bool
}

var addCmd = &cobra.Command{
	Use:   "add NAME",
	Short: "add a new profile",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		p := config.Profile{
			Name:           args[0],
			APIEndpoint:    addFlags.apiEndpoint,
			CredentialRef:  addFlags.credentialRef,
			DefaultProject: addFlags.defaultProject,
			Trust:          addFlags.trust,
			IsProduction:   addFlags.isProduction,
		}
		if err := config.AddProfile(p); err != nil {
			return err
		}
		fmt.Println(i18n.T("msg.profile.added", p.Name))
		return nil
	},
}

func init() {
	addCmd.Flags().StringVar(&addFlags.apiEndpoint, "api-endpoint", "", "gateway URL (required)")
	addCmd.Flags().StringVar(&addFlags.credentialRef, "credential-ref", "", "keychain entry name or file:// path (required)")
	addCmd.Flags().StringVar(&addFlags.defaultProject, "default-project", "", "optional default project name")
	addCmd.Flags().StringVar(&addFlags.trust, "trust", "user", "user|dev")
	addCmd.Flags().BoolVar(&addFlags.isProduction, "is-production", true, "treat as production (required false to allow service_overrides)")
	_ = addCmd.MarkFlagRequired("api-endpoint")
	_ = addCmd.MarkFlagRequired("credential-ref")
}
