package config

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"

	intconfig "github.com/vibeknow/cli/internal/config"
)

var listCmd = &cobra.Command{
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
		for _, k := range keys {
			fmt.Printf("%s=%s\n", k, kv[k])
		}
		return nil
	},
}
