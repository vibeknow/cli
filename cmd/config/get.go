package config

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/vibeknow/cli/internal/cmdutil"
	intconfig "github.com/vibeknow/cli/internal/config"
)

var getCmd = &cobra.Command{
	Use:   "get KEY",
	Short: "read a config value",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		kv, err := intconfig.LoadKV()
		if err != nil {
			return err
		}
		v, ok := kv[args[0]]
		if !ok {
			return fmt.Errorf("key %q not set", args[0])
		}
		return cmdutil.Emit(cmd, map[string]any{
			"key":   args[0],
			"value": v,
		}, "config.value", func(w io.Writer) {
			fmt.Fprintln(w, v)
		})
	},
}
