package config

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/vibeknow/cli/internal/cmdutil"
	intconfig "github.com/vibeknow/cli/internal/config"
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
		previous, existed := kv[args[0]]
		kv[args[0]] = args[1]
		if err := intconfig.SaveKV(kv); err != nil {
			return err
		}
		return cmdutil.Emit(cmd, map[string]any{
			"key":      args[0],
			"value":    args[1],
			"previous": previous,
			"created":  !existed,
		}, "config.set", func(w io.Writer) {
			fmt.Fprintf(w, "%s=%s\n", args[0], args[1])
		})
	},
}
