package config

import (
	"fmt"
	"io"
	"sort"

	"github.com/spf13/cobra"

	"github.com/vibeknow/cli/internal/cmdutil"
	intconfig "github.com/vibeknow/cli/internal/config"
)

var listCmd = &cobra.Command{
	// Takes no positional arguments. Without this cobra accepts and
	// silently discards them, so a stray argument looks like success.
	Args:  cobra.NoArgs,
	Use:   "list",
	Short: "list all config keys",
	RunE: func(cmd *cobra.Command, args []string) error {
		kv, err := intconfig.LoadKV()
		if err != nil {
			return err
		}
		keys := make([]string, 0, len(kv))
		for k := range kv {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		return cmdutil.Emit(cmd, map[string]any{
			"config": kv,
			"keys":   keys,
		}, "config.list", func(w io.Writer) {
			for _, k := range keys {
				fmt.Fprintf(w, "%s=%s\n", k, kv[k])
			}
		})
	},
}
