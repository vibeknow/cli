package cmdutil

import (
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// AliasFlags makes alternate spellings of a flag work without adding them to
// --help.
//
// The CLI grew two words for the same idea in a couple of places: `--size` on
// the paginated list commands versus `--limit` on the unpaginated one, and
// `--no-wait` on `auth login` versus `--async` on the commands that submit a
// job. A caller working from a partial memory of the docs — a person, or a
// model that saw a sibling command's help — reaches for the wrong one, and
// "unknown flag" is a pointless round trip when the intent was unambiguous.
//
// Aliases are normalized at parse time rather than registered as real flags,
// so `--help` still shows exactly one name per concept and there is no second
// variable to keep in sync.
//
// alias maps the accepted spelling to the flag's real name.
func AliasFlags(cmd *cobra.Command, alias map[string]string) {
	prev := cmd.Flags().GetNormalizeFunc()
	cmd.Flags().SetNormalizeFunc(func(f *pflag.FlagSet, name string) pflag.NormalizedName {
		if real, ok := alias[name]; ok {
			name = real
		}
		return prev(f, name)
	})
}
