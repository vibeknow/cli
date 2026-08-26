// Package preset expands a saved bundle of `create` options into flags.
//
// `vk create` now takes 21 flags. Most of them describe a *style* rather
// than a run: mode, aspect, theme, voice, language, bgm, avatar placement.
// A team that has settled on how its videos should look re-types the same
// dozen flags on every invocation, and an agent driving the CLI has to
// carry that combination in its prompt, where it drifts.
//
// A preset is that combination in a file the user can version-control:
//
//	# ~/.config/vibeknow/presets/brand-explainer.yaml
//	schema_version: "1"
//	name: brand-explainer
//	description: how our explainers look
//	create:
//	  mode: image
//	  aspect: horizontal
//	  language: zh-CN
//	  bgm: true
//	  voice: "12"
//
//	$ vk create --from deck.pdf --preset brand-explainer
//
// Two rules make it safe to accept a file someone else wrote.
//
// The first is that a preset only supplies defaults. Anything given on the
// command line wins, always, because Apply skips every flag cobra reports
// as already Changed. Reading a preset can therefore never contradict what
// the caller typed.
//
// The second is that the option set is an allowlist, not "every create
// flag". A preset cannot set --export, --yes or --confirm: those authorize
// a charge, and consent that arrives inside a file someone forwarded to you
// is not consent. It cannot set --from or --kb-id either, which identify
// one run's input rather than a reusable style. Both refusals are explicit
// errors naming the reason, not silent drops — a preset whose `yes: true`
// were quietly ignored would read as though it had worked.
//
// This is deliberately a client-side expansion with no backend involvement:
// every key here becomes an ordinary flag before anything is uploaded, so
// a preset can express exactly what the command line can express and
// nothing more. It is not a place to put per-node instructions — the task
// init request has no field to carry them (client/figlens/task.go:31-56).
package preset

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/pflag"
	"gopkg.in/yaml.v3"

	"github.com/vibeknow/cli/internal/config"
)

// SchemaVersion is the only version this build understands. A file
// declaring anything else is refused rather than read on a guess.
const SchemaVersion = "1"

// File is the on-disk shape.
type File struct {
	SchemaVersion string         `yaml:"schema_version"`
	Name          string         `yaml:"name"`
	Description   string         `yaml:"description"`
	Create        map[string]any `yaml:"create"`

	// Path is where it was read from, kept so every error message can name
	// the file the caller has to go and edit.
	Path string `yaml:"-"`
}

// allowed lists the `create` flags a preset may set: the ones that describe
// how a video should look and sound, and would be identical across runs.
var allowed = map[string]bool{
	"mode":            true,
	"script-lock":     true,
	"aspect":          true,
	"bgm":             true,
	"engine":          true,
	"pages":           true,
	"images":          true,
	"theme":           true,
	"language":        true,
	"voice":           true,
	"prompt":          true,
	"avatar":          true,
	"avatar-position": true,
	"avatar-size":     true,
}

// denied maps a rejected key to why it is rejected. Keys are listed here
// rather than merely omitted from `allowed` so the error can say what the
// caller should do instead.
var denied = map[string]string{
	"export":      "authorizes a charge; pass --export on the command line",
	"yes":         "grants consent to a charge; pass --yes on the command line",
	"confirm":     "carries a one-time action token; pass --confirm on the command line",
	"from":        "identifies one run's input, not a reusable style",
	"kb-id":       "identifies one run's input, not a reusable style",
	"async":       "describes this invocation, not the style; pass --async on the command line",
	"preview-dir": "is a local path that does not travel with the file; pass --preview-dir on the command line",
	"output":      "describes this invocation, not the style; pass --output on the command line",
}

// Dir returns the directory searched when --preset is given a bare name.
func Dir() (string, error) {
	d, err := config.ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "presets"), nil
}

// isPathRef reports whether ref should be read as a filesystem path rather
// than looked up by name. Anything with a separator or a YAML extension is
// a path; a bare word is a name.
func isPathRef(ref string) bool {
	if strings.ContainsAny(ref, `/\`) {
		return true
	}
	ext := strings.ToLower(filepath.Ext(ref))
	return ext == ".yaml" || ext == ".yml"
}

// Resolve turns a --preset value into a file path.
func Resolve(ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", errors.New("--preset needs a name or a path")
	}
	if isPathRef(ref) {
		return ref, nil
	}
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	for _, ext := range []string{".yaml", ".yml"} {
		p := filepath.Join(dir, ref+ext)
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	// Naming what is available turns a typo into a one-step fix, and is the
	// only listing this feature offers — there is no `vk preset list`.
	return "", fmt.Errorf("no preset named %q in %s%s", ref, dir, availableSuffix(dir))
}

func availableSuffix(dir string) string {
	names := available(dir)
	if len(names) == 0 {
		return " (the directory is empty or absent)"
	}
	return " (available: " + strings.Join(names, ", ") + ")"
}

func available(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if ext == ".yaml" || ext == ".yml" {
			names = append(names, strings.TrimSuffix(e.Name(), filepath.Ext(e.Name())))
		}
	}
	sort.Strings(names)
	return names
}

// Load reads and validates the preset named or pointed at by ref.
func Load(ref string) (*File, error) {
	path, err := Resolve(ref)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("preset file not found: %s", path)
		}
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var f File
	dec := yaml.NewDecoder(bytes.NewReader(data))
	// A misspelled top-level key is a preset that would silently do nothing.
	dec.KnownFields(true)
	if err := dec.Decode(&f); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if f.SchemaVersion == "" {
		f.SchemaVersion = SchemaVersion
	}
	if f.SchemaVersion != SchemaVersion {
		return nil, fmt.Errorf("%s: schema_version %q is not supported by this build (want %q)", path, f.SchemaVersion, SchemaVersion)
	}
	if len(f.Create) == 0 {
		return nil, fmt.Errorf("%s: the `create:` block is empty; a preset that sets nothing is a mistake, not a default", path)
	}
	if f.Name == "" {
		f.Name = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	f.Path = path
	return &f, nil
}

// Apply sets every preset key on fs that the caller did not already give on
// the command line, and returns the names it set, sorted.
//
// It never overwrites a Changed flag: the command line is the caller's
// direct instruction and a file must not be able to contradict it.
func Apply(fs *pflag.FlagSet, f *File) ([]string, error) {
	if f == nil {
		return nil, nil
	}
	// Sorted so an invalid file always reports the same key first; map
	// iteration order would make the error message depend on the run.
	keys := make([]string, 0, len(f.Create))
	for k := range f.Create {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var applied []string
	for _, raw := range keys {
		// YAML prefers underscores, flags prefer dashes. Accept both rather
		// than making the file's spelling a second thing to get right.
		name := strings.ReplaceAll(strings.TrimSpace(raw), "_", "-")
		if why, bad := denied[name]; bad {
			return nil, fmt.Errorf("%s: `%s` may not be set by a preset — it %s", f.Path, raw, why)
		}
		if !allowed[name] {
			return nil, fmt.Errorf("%s: unknown preset key %q (allowed: %s)", f.Path, raw, strings.Join(allowedNames(), ", "))
		}
		if fs.Lookup(name) == nil {
			// The allowlist and the command's flag set disagree — a build
			// error, not a user error, but not worth a panic either.
			return nil, fmt.Errorf("%s: `%s` is not a flag of this command", f.Path, raw)
		}
		if fs.Changed(name) {
			continue // command line wins
		}
		val, err := scalar(f.Create[raw])
		if err != nil {
			return nil, fmt.Errorf("%s: `%s`: %w", f.Path, raw, err)
		}
		// pflag parses and range-checks the value, so a bad `pages: many`
		// fails here with the same message the command line would give.
		if err := fs.Set(name, val); err != nil {
			return nil, fmt.Errorf("%s: `%s`: %w", f.Path, raw, err)
		}
		applied = append(applied, name)
	}
	return applied, nil
}

func allowedNames() []string {
	out := make([]string, 0, len(allowed))
	for k := range allowed {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// scalar renders one YAML value as the string pflag expects. A sequence
// becomes a comma-joined list, so `images: [1, 3, 5]` and `images: "1,3,5"`
// both work — the first is what a YAML author reaches for.
func scalar(v any) (string, error) {
	switch t := v.(type) {
	case nil:
		return "", errors.New("has no value")
	case string:
		return t, nil
	case bool:
		return strconv.FormatBool(t), nil
	case int:
		return strconv.Itoa(t), nil
	case int64:
		return strconv.FormatInt(t, 10), nil
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64), nil
	case []any:
		parts := make([]string, 0, len(t))
		for _, e := range t {
			s, err := scalar(e)
			if err != nil {
				return "", fmt.Errorf("list entry: %w", err)
			}
			parts = append(parts, s)
		}
		return strings.Join(parts, ","), nil
	default:
		return "", fmt.Errorf("must be a scalar or a list of scalars, got %T", v)
	}
}
