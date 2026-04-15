package config

import (
	"github.com/spf13/cobra"

	intconfig "github.com/shiliu-ai/vibeknow-cli/internal/config"
)

var setCmd = &cobra.Command{
	Use:   "set KEY VALUE",
	Short: "write a config value",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		kv, err := intconfig.LoadKV()
		if err != nil {
			return err
		}
		kv[args[0]] = args[1]
		return intconfig.SaveKV(kv)
	},
}
