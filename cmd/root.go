package cmd

import (
	"os"

	"github.com/spf13/cobra"

	apicmd "github.com/vibeknow/cli/cmd/api"
	authcmd "github.com/vibeknow/cli/cmd/auth"
	configcmd "github.com/vibeknow/cli/cmd/config"
	creditscmd "github.com/vibeknow/cli/cmd/credits"
	doccmd "github.com/vibeknow/cli/cmd/doc"
	profilecmd "github.com/vibeknow/cli/cmd/profile"
	videocmd "github.com/vibeknow/cli/cmd/video"
	voicecmd "github.com/vibeknow/cli/cmd/voice"
	"github.com/vibeknow/cli/internal/clerr"
	"github.com/vibeknow/cli/internal/i18n"
)

var (
	version     = "dev" // set via -ldflags
	flagProfile string
	flagOutput  string
	flagVerbose bool
)

var rootCmd = &cobra.Command{
	Use:           "vibeknow",
	Short:         "vibeknow CLI — turn docs into videos",
	SilenceUsage:  true,
	SilenceErrors: false,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		i18n.Init()
	},
}

func init() {
	rootCmd.PersistentFlags().StringVar(&flagProfile, "profile", "", "override active profile for this command")
	rootCmd.PersistentFlags().StringVar(&flagOutput, "output", "", "output format: text|json|ndjson (auto-selects based on TTY)")
	rootCmd.PersistentFlags().BoolVar(&flagVerbose, "verbose", false, "emit request/response summaries (credentials redacted)")
	rootCmd.Version = version // enables --version
	rootCmd.AddCommand(apicmd.Cmd)
	rootCmd.AddCommand(authcmd.Cmd)
	rootCmd.AddCommand(configcmd.Cmd)
	rootCmd.AddCommand(createCmd)
	rootCmd.AddCommand(creditscmd.Cmd)
	rootCmd.AddCommand(doccmd.Cmd)
	rootCmd.AddCommand(profilecmd.Cmd)
	rootCmd.AddCommand(videocmd.Cmd)
	rootCmd.AddCommand(voicecmd.Cmd)
	rootCmd.AddCommand(initCmd)
}

func Execute() error {
	rootCmd.SilenceErrors = true
	err := rootCmd.Execute()
	if err != nil {
		clerr.Render(os.Stderr, err)
	}
	return err
}
