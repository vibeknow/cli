package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/vibeknow/cli/internal/clerr"
	"github.com/vibeknow/cli/internal/events"
	"github.com/vibeknow/cli/internal/i18n"
	"github.com/vibeknow/cli/internal/preset"
)

var flagCreatePreset string

// applyCreatePreset expands --preset into flags before anything reads them.
//
// Every failure here is exit 2. A preset is part of the command line as far
// as the caller is concerned, so a broken one is a bad invocation — and one
// that costs nothing, since this runs before the first upload.
func applyCreatePreset(cmd *cobra.Command) error {
	ref := strings.TrimSpace(flagCreatePreset)
	if ref == "" {
		return nil
	}
	f, err := preset.Load(ref)
	if err != nil {
		e := clerr.Validation(err.Error())
		if dir, derr := preset.Dir(); derr == nil {
			e = e.WithHint(i18n.T("create.preset.hint_dir", dir))
		}
		return e
	}
	applied, err := preset.Apply(cmd.Flags(), f)
	if err != nil {
		return clerr.Validation(err.Error())
	}
	if applied == nil {
		applied = []string{}
	}

	// Which flags a run actually used is the first thing anyone asks when a
	// video comes out wrong, and with a preset in play the command line no
	// longer answers it. Say so on every run, in whichever register the
	// caller is reading.
	format, _ := cmd.Flags().GetString("output")
	em := events.New(cmd.ErrOrStderr(), format, os.Getenv(events.EnvEvents))
	if em.Enabled() {
		em.Emit(map[string]any{
			"type":   "preset.applied",
			"preset": f.Name,
			"path":   f.Path,
			"keys":   applied,
		})
		return nil
	}
	if len(applied) == 0 {
		fmt.Fprintln(cmd.ErrOrStderr(), i18n.T("create.preset.applied_none", f.Name))
		return nil
	}
	fmt.Fprintln(cmd.ErrOrStderr(), i18n.T("create.preset.applied", f.Name, strings.Join(applied, ", ")))
	return nil
}

func init() {
	// Registered here rather than in create.go's init: createCmd is a
	// package-level var, so it exists before any init() in the package runs.
	// No --style alias: --theme is documented as the "style ID", so a user
	// typing --style is more likely reaching for that than for this.
	createCmd.Flags().StringVar(&flagCreatePreset, "preset", "", i18n.T("create.flag.preset"))
}
