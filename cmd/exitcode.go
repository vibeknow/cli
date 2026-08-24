package cmd

import (
	"errors"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/vibeknow/cli/internal/clerr"
	"github.com/vibeknow/cli/internal/errs"
	"github.com/vibeknow/cli/internal/httpclient"
)

func init() {
	// Give every command the documented exit code for a structured backend
	// error, not just the handful that classify codes themselves.
	clerr.Classifier = func(err error) int {
		var o *errs.Object
		if errors.As(err, &o) {
			return httpclient.ExitCodeForCode(o.Code)
		}
		return 0
	}

	// Flag parsing errors — unknown flag, unknown shorthand, missing value,
	// bad value for a typed flag. Inherited by every subcommand.
	rootCmd.SetFlagErrorFunc(func(c *cobra.Command, err error) error {
		e := clerr.Validation(err.Error())
		// A near-miss is the common case: a flag remembered from a sibling
		// command (`--size` where this one says `--limit`), a separator swap
		// (`--session_id`), or a plain typo. Naming the intended flag turns a
		// look-it-up-in-the-docs round trip into a one-token correction.
		if name := unknownFlagName(err.Error()); name != "" {
			if best := nearestFlag(c, name); best != "" {
				return e.WithHintf("did you mean --%s? run `%s --help` for the full list", best, c.CommandPath())
			}
		}
		return e.WithHintf("run `%s --help` for the accepted flags", c.CommandPath())
	})
}

// unknownFlagName pulls the offending long-flag name out of pflag's message.
// Returns "" for anything else (shorthand flags, missing values, bad values),
// where a spelling suggestion would not apply.
func unknownFlagName(msg string) string {
	const p = "unknown flag: --"
	if !strings.HasPrefix(msg, p) {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(msg, p))
}

// nearestFlag returns the closest flag available on c — its own plus the
// inherited persistent ones — or "" when nothing is close enough to suggest.
func nearestFlag(c *cobra.Command, name string) string {
	var best string
	bestDist := len(name)/2 + 2 // tolerate roughly a third of the name being wrong
	consider := func(f *pflag.Flag) {
		if d := levenshtein(name, f.Name); d < bestDist {
			bestDist, best = d, f.Name
		}
	}
	c.Flags().VisitAll(consider)
	c.InheritedFlags().VisitAll(consider)
	return best
}

func levenshtein(a, b string) int {
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(min(cur[j-1]+1, prev[j]+1), prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(b)]
}

// usagePrefixes are the errors cobra raises for a malformed command line that
// do not pass through SetFlagErrorFunc: unknown subcommands, missing required
// flags, and the built-in positional-argument validators.
//
// These are the mistakes a caller — a person guessing, or a model writing a
// command from a partial memory of the docs — makes most often, and they are
// exactly the ones that are trivially self-correctable. Exiting 1 for them
// told the caller "something went wrong, read stderr" when the honest answer
// was "your arguments are wrong, fix them and retry", which is what exit 2
// means everywhere else in this CLI.
//
// Cobra reports them as plain errors with no type to match on, so matching is
// by message prefix. TestUsageErrorsExitTwo drives each of these through the
// real command tree, so a cobra upgrade that rewords them fails loudly here
// rather than silently reverting the exit code.
var usagePrefixes = []string{
	"unknown command",
	"unknown flag",
	"unknown shorthand flag",
	"flag needs an argument",
	"invalid argument",
	"required flag(s)",
	"accepts ", // accepts N arg(s), received M
	"requires at least",
	"requires at most",
}

// asUsageError converts a cobra command-line error into an exit-2 validation
// error. Errors already carrying an exit code are returned untouched.
func asUsageError(err error) error {
	if err == nil {
		return nil
	}
	var e *clerr.Error
	if errors.As(err, &e) {
		return err
	}
	msg := err.Error()
	for _, p := range usagePrefixes {
		if strings.HasPrefix(msg, p) {
			return clerr.Validation(msg).WithCause(err)
		}
	}
	return err
}
