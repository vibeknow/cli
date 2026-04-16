package profile

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/vibeknow/cli/internal/config"
	"github.com/vibeknow/cli/internal/i18n"
)

var addFlags struct {
	endpointAccount  string
	endpointVectoria string
	endpointFiglens  string
	endpointVibeknow string
	apiEndpoint      string
	credentialRef    string
	defaultProject   string
	trust            string
	isProduction     bool
}

var addCmd = &cobra.Command{
	Use:   "add NAME",
	Short: "add a new profile",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		endpoints := map[string]string{}
		if addFlags.endpointAccount != "" {
			endpoints["account"] = addFlags.endpointAccount
		}
		if addFlags.endpointVectoria != "" {
			endpoints["vectoria"] = addFlags.endpointVectoria
		}
		if addFlags.endpointFiglens != "" {
			endpoints["figlens"] = addFlags.endpointFiglens
		}
		if addFlags.endpointVibeknow != "" {
			endpoints["vibeknow"] = addFlags.endpointVibeknow
		}
		if addFlags.apiEndpoint != "" && endpoints["vibeknow"] == "" {
			endpoints["vibeknow"] = addFlags.apiEndpoint
		}
		p := config.Profile{
			Name:           args[0],
			Endpoints:      endpoints,
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
	addCmd.Flags().StringVar(&addFlags.endpointAccount, "endpoint-account", "", "go-account URL override (optional; default uses cloud)")
	addCmd.Flags().StringVar(&addFlags.endpointVectoria, "endpoint-vectoria", "", "go-vectoria URL override")
	addCmd.Flags().StringVar(&addFlags.endpointFiglens, "endpoint-figlens", "", "go-figlens URL override")
	addCmd.Flags().StringVar(&addFlags.endpointVibeknow, "endpoint-vibeknow", "", "go-vibeknow URL override")
	addCmd.Flags().StringVar(&addFlags.apiEndpoint, "api-endpoint", "", "DEPRECATED: alias for --endpoint-vibeknow")
	addCmd.Flags().StringVar(&addFlags.credentialRef, "credential-ref", "", "keychain entry name or file:// path (required)")
	addCmd.Flags().StringVar(&addFlags.defaultProject, "default-project", "", "optional default project name")
	addCmd.Flags().StringVar(&addFlags.trust, "trust", "user", "user|dev")
	addCmd.Flags().BoolVar(&addFlags.isProduction, "is-production", true, "treat as production (must be false to allow non-prod endpoint overrides)")
	_ = addCmd.MarkFlagRequired("credential-ref")
}
