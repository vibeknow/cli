package output

import (
	"fmt"
	"strings"
)

// The three formats every command understands. Commands that have no
// structured payload to emit still accept them; they simply keep writing
// their human text.
const (
	FormatText   = "text"
	FormatJSON   = "json"
	FormatNDJSON = "ndjson"
)

// EnvFormat is the environment variable that sets the default output
// format. It exists for non-interactive callers (agents, CI) that want
// structured output everywhere without threading --output through every
// invocation. An explicit --output always wins over it.
//
// Deliberately not "detect a pipe and switch to JSON": that would change
// the output of every existing shell pipeline on upgrade, and the CLIs
// this one is modelled on (gh, kubectl, docker) all keep the format
// explicit for the same reason.
const EnvFormat = "VIBEKNOW_OUTPUT"

// Formats lists the accepted values, in help/error message order.
var Formats = []string{FormatText, FormatJSON, FormatNDJSON}

// ResolveFormat picks the effective output format from the two inputs that
// can set it, in precedence order: an explicitly passed --output, then
// VIBEKNOW_OUTPUT, then the text default.
//
// flagSet distinguishes `--output ""` from "flag absent"; env is the raw
// VIBEKNOW_OUTPUT value ("" when unset).
//
// An unrecognized value is an error rather than a silent fall-through to
// text. `--output jsonl` used to print human text and exit 0, so a caller
// with a typo got prose where it expected a parseable object and had no
// signal that anything was wrong.
func ResolveFormat(flagVal string, flagSet bool, env string) (string, error) {
	if flagSet {
		return normalizeFormat(flagVal, "--output")
	}
	if env = strings.TrimSpace(env); env != "" {
		return normalizeFormat(env, EnvFormat)
	}
	return FormatText, nil
}

func normalizeFormat(v, source string) (string, error) {
	switch f := strings.ToLower(strings.TrimSpace(v)); f {
	case "":
		// `--output ""` is the same as not asking for anything.
		return FormatText, nil
	case FormatText, FormatJSON, FormatNDJSON:
		return f, nil
	default:
		return "", fmt.Errorf("%s must be one of: %s (got %q)",
			source, strings.Join(Formats, ", "), v)
	}
}
