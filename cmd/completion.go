package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var completionCmd = &cobra.Command{
	Use:       "completion [bash|zsh|fish|powershell]",
	Short:     "generate shell completion script",
	Args:      cobra.ExactArgs(1),
	ValidArgs: []string{"bash", "zsh", "fish", "powershell"},
	RunE: func(cmd *cobra.Command, args []string) error {
		valid := map[string]bool{"bash": true, "zsh": true, "fish": true, "powershell": true}
		if !valid[args[0]] {
			return fmt.Errorf(`invalid shell %q; supported: bash, zsh, fish, powershell`, args[0])
		}
		switch args[0] {
		case "bash":
			_ = rootCmd.GenBashCompletion(os.Stdout)
		case "zsh":
			_ = rootCmd.GenZshCompletion(os.Stdout)
		case "fish":
			_ = rootCmd.GenFishCompletion(os.Stdout, true)
		case "powershell":
			_ = rootCmd.GenPowerShellCompletion(os.Stdout)
		}
		return nil
	},
}

func init() { rootCmd.AddCommand(completionCmd) }
