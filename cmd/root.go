package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	apicmd "github.com/vibeknow/cli/cmd/api"
	authcmd "github.com/vibeknow/cli/cmd/auth"
	avatarcmd "github.com/vibeknow/cli/cmd/avatar"
	configcmd "github.com/vibeknow/cli/cmd/config"
	creditscmd "github.com/vibeknow/cli/cmd/credits"
	doccmd "github.com/vibeknow/cli/cmd/doc"
	jobscmd "github.com/vibeknow/cli/cmd/jobs"
	kbcmd "github.com/vibeknow/cli/cmd/kb"
	profilecmd "github.com/vibeknow/cli/cmd/profile"
	themecmd "github.com/vibeknow/cli/cmd/theme"
	videocmd "github.com/vibeknow/cli/cmd/video"
	voicecmd "github.com/vibeknow/cli/cmd/voice"
	"github.com/vibeknow/cli/internal/clerr"
	"github.com/vibeknow/cli/internal/i18n"
	"github.com/vibeknow/cli/internal/output"
	"github.com/vibeknow/cli/internal/update"
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
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		i18n.Init()
		// --verbose is sugar for VIBEKNOW_DEBUG=1; the HTTP middleware reads
		// the env var at request time, so setting it here propagates.
		if flagVerbose {
			_ = os.Setenv("VIBEKNOW_DEBUG", "1")
		}
		return resolveOutputFlag(cmd)
	},
}

// resolveOutputFlag folds VIBEKNOW_OUTPUT into --output and rejects
// unrecognized formats, once, before any command runs.
//
// It writes the result back into flagOutput, which is the variable behind
// the persistent --output flag, so the ~20 call sites that read
// `cmd.Flags().GetString("output")` see the resolved value without
// each having to know about the env var or the default.
func resolveOutputFlag(cmd *cobra.Command) error {
	f := cmd.Flags().Lookup("output")
	if f == nil {
		return nil
	}
	format, err := output.ResolveFormat(f.Value.String(), f.Changed, os.Getenv(output.EnvFormat))
	if err != nil {
		e := clerr.Validation(err.Error())
		// The one collision worth calling out by name: `video download`
		// used to take a file path on --output, and that spelling is in
		// released docs and skill files.
		if looksLikePath(f.Value.String()) {
			e = e.WithHint("to write the download to a file, use --dest (e.g. `vk video download --dest out.mp4`)")
		}
		return e
	}
	flagOutput = format
	return nil
}

func looksLikePath(v string) bool {
	return strings.ContainsAny(v, "/\\.") || strings.HasSuffix(strings.ToLower(v), "mp4")
}

func init() {
	rootCmd.PersistentFlags().StringVar(&flagProfile, "profile", "", "override active profile for this command")
	rootCmd.PersistentFlags().StringVar(&flagOutput, "output", "", "output format: text|json|ndjson (default text; set "+output.EnvFormat+" to change the default)")
	rootCmd.PersistentFlags().BoolVar(&flagVerbose, "verbose", false, "emit request/response summaries (credentials redacted)")
	rootCmd.Version = version // enables --version
	rootCmd.AddCommand(apicmd.Cmd)
	rootCmd.AddCommand(authcmd.Cmd)
	rootCmd.AddCommand(avatarcmd.Cmd)
	rootCmd.AddCommand(configcmd.Cmd)
	rootCmd.AddCommand(createCmd)
	rootCmd.AddCommand(creditscmd.Cmd)
	rootCmd.AddCommand(doccmd.Cmd)
	rootCmd.AddCommand(jobscmd.Cmd)
	rootCmd.AddCommand(kbcmd.Cmd)
	rootCmd.AddCommand(profilecmd.Cmd)
	rootCmd.AddCommand(themecmd.Cmd)
	rootCmd.AddCommand(videocmd.Cmd)
	rootCmd.AddCommand(voicecmd.Cmd)
	rootCmd.AddCommand(initCmd)
}

func setupUpdateNotice() {
	// Sync: check cache (fast, no network)
	if info := update.CheckCached(version); info != nil {
		update.SetPending(info)
	}

	// Async: refresh cache for future runs
	go func() {
		defer func() { recover() }()
		update.RefreshCache(version)
		if update.GetPending() == nil {
			if info := update.CheckCached(version); info != nil {
				update.SetPending(info)
			}
		}
	}()

	// Wire pending notices into JSON error envelopes.
	clerr.PendingNotice = func() map[string]interface{} {
		info := update.GetPending()
		if info == nil {
			return nil
		}
		return map[string]interface{}{
			"update": map[string]interface{}{
				"message": info.Message(),
				"latest":  info.Latest,
			},
		}
	}
}

// Execute runs the root command and returns the error (if any). Callers
// should use clerr.ExitCodeFor(err) to pick the process exit code.
func Execute() error {
	setupUpdateNotice()
	rootCmd.SilenceErrors = true
	err := asUsageError(rootCmd.Execute())
	if err != nil {
		format := "text"
		if flagOutput == "json" {
			format = "json"
		}
		clerr.RenderAs(os.Stderr, err, format, "")
	}
	// Show update notice at the end (stderr, text-mode only — JSON callers
	// read the envelope's _notice field instead).
	if flagOutput != "json" {
		if info := update.GetPending(); info != nil {
			fmt.Fprint(os.Stderr, i18n.T("root.update_notice", info.Message()))
		}
	}
	return err
}
