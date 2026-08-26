package preset

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/pflag"
)

// flags mirrors the subset of `create` flags a preset is allowed to touch,
// with the same types, so pflag's parsing behaves as it does in the real
// command.
func flags() *pflag.FlagSet {
	fs := pflag.NewFlagSet("create", pflag.ContinueOnError)
	fs.String("mode", "", "")
	fs.Bool("script-lock", false, "")
	fs.String("aspect", "", "")
	fs.Bool("bgm", false, "")
	fs.String("engine", "", "")
	fs.Int("pages", 0, "")
	fs.String("images", "", "")
	fs.String("theme", "", "")
	fs.String("language", "", "")
	fs.String("voice", "", "")
	fs.String("prompt", "", "")
	fs.String("avatar", "", "")
	fs.String("avatar-position", "", "")
	fs.Float64("avatar-size", 0, "")
	// Present on the real command, never settable from a file.
	fs.String("from", "", "")
	fs.Bool("export", false, "")
	fs.Bool("yes", false, "")
	return fs
}

func write(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "p.yaml")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func load(t *testing.T, body string) *File {
	t.Helper()
	f, err := Load(write(t, body))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return f
}

func TestApply_SetsValuesTheCallerDidNotGive(t *testing.T) {
	f := load(t, `
name: brand
create:
  mode: image
  aspect: horizontal
  bgm: true
  pages: 12
  avatar-size: 240
`)
	fs := flags()
	applied, err := Apply(fs, f)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	want := "aspect, avatar-size, bgm, mode, pages"
	if got := strings.Join(applied, ", "); got != want {
		t.Fatalf("applied = %q, want %q (sorted, so an error always names the same key first)", got, want)
	}
	if v, _ := fs.GetString("mode"); v != "image" {
		t.Fatalf("mode = %q", v)
	}
	if v, _ := fs.GetBool("bgm"); !v {
		t.Fatal("bgm not set")
	}
	if v, _ := fs.GetInt("pages"); v != 12 {
		t.Fatalf("pages = %d", v)
	}
	if v, _ := fs.GetFloat64("avatar-size"); v != 240 {
		t.Fatalf("avatar-size = %v", v)
	}
}

// The rule the whole feature rests on: a file may not contradict what the
// user typed. If this inverts, a stale preset silently redirects every run.
func TestApply_CommandLineWins(t *testing.T) {
	f := load(t, `
create:
  mode: image
  aspect: horizontal
`)
	fs := flags()
	if err := fs.Parse([]string{"--mode", "replica"}); err != nil {
		t.Fatal(err)
	}
	applied, err := Apply(fs, f)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if v, _ := fs.GetString("mode"); v != "replica" {
		t.Fatalf("preset overwrote an explicit --mode: got %q, want replica", v)
	}
	if v, _ := fs.GetString("aspect"); v != "horizontal" {
		t.Fatalf("aspect should still come from the preset, got %q", v)
	}
	for _, k := range applied {
		if k == "mode" {
			t.Fatal("mode was reported as applied though the command line supplied it")
		}
	}
}

// An explicit `--bgm=false` is still an instruction. pflag reports it as
// Changed, so a preset saying `bgm: true` must not turn it back on.
func TestApply_ExplicitFalseIsStillTheCallersChoice(t *testing.T) {
	f := load(t, "create:\n  bgm: true\n")
	fs := flags()
	if err := fs.Parse([]string{"--bgm=false"}); err != nil {
		t.Fatal(err)
	}
	if _, err := Apply(fs, f); err != nil {
		t.Fatal(err)
	}
	if v, _ := fs.GetBool("bgm"); v {
		t.Fatal("preset re-enabled bgm over an explicit --bgm=false")
	}
}

// Consent that arrives inside a file someone forwarded to you is not
// consent. These three keys are the reason the option set is an allowlist.
func TestApply_APresetCannotAuthorizeSpend(t *testing.T) {
	for _, key := range []string{"export", "yes", "confirm"} {
		f := load(t, "create:\n  "+key+": true\n")
		_, err := Apply(flags(), f)
		if err == nil {
			t.Fatalf("%q was accepted from a preset; a file must not be able to approve a charge", key)
		}
		if !strings.Contains(err.Error(), key) {
			t.Fatalf("error for %q does not name the key: %v", key, err)
		}
		if !strings.Contains(err.Error(), "command line") {
			t.Fatalf("error for %q does not say where to pass it instead: %v", key, err)
		}
	}
}

func TestApply_PerRunInputIsRejected(t *testing.T) {
	f := load(t, "create:\n  from: deck.pdf\n")
	_, err := Apply(flags(), f)
	if err == nil || !strings.Contains(err.Error(), "not a reusable style") {
		t.Fatalf("want a refusal explaining why --from is per-run, got %v", err)
	}
	if v, _ := flags().GetString("from"); v != "" {
		t.Fatal("from leaked through")
	}
}

// A key the CLI does not understand must fail loudly. Ignoring it would
// produce a run that looks configured and is not.
func TestApply_UnknownKeyFailsAndListsWhatIsValid(t *testing.T) {
	f := load(t, "create:\n  asepct: horizontal\n")
	_, err := Apply(flags(), f)
	if err == nil {
		t.Fatal("unknown key was ignored")
	}
	if !strings.Contains(err.Error(), "asepct") {
		t.Fatalf("error does not quote the offending key: %v", err)
	}
	if !strings.Contains(err.Error(), "aspect") {
		t.Fatalf("error does not list the valid keys, leaving the caller to guess: %v", err)
	}
	if !strings.Contains(err.Error(), f.Path) {
		t.Fatalf("error does not name the file to edit: %v", err)
	}
}

// pflag already knows how to reject a bad value; routing through Set means
// a preset gets the same message the command line would give.
func TestApply_BadValueIsRejectedByTheFlagItself(t *testing.T) {
	f := load(t, "create:\n  pages: many\n")
	_, err := Apply(flags(), f)
	if err == nil {
		t.Fatal("pages: many was accepted")
	}
	if !strings.Contains(err.Error(), "pages") {
		t.Fatalf("error does not name the key: %v", err)
	}
}

func TestApply_UnderscoresAndDashesBothWork(t *testing.T) {
	f := load(t, "create:\n  script_lock: true\n  avatar_position: top-left\n")
	fs := flags()
	if _, err := Apply(fs, f); err != nil {
		t.Fatalf("YAML-style underscore keys rejected: %v", err)
	}
	if v, _ := fs.GetBool("script-lock"); !v {
		t.Fatal("script_lock did not reach --script-lock")
	}
	if v, _ := fs.GetString("avatar-position"); v != "top-left" {
		t.Fatalf("avatar-position = %q", v)
	}
}

func TestApply_ListBecomesACommaSeparatedValue(t *testing.T) {
	f := load(t, "create:\n  images: [1, 3, 5]\n")
	fs := flags()
	if _, err := Apply(fs, f); err != nil {
		t.Fatal(err)
	}
	if v, _ := fs.GetString("images"); v != "1,3,5" {
		t.Fatalf("images = %q, want 1,3,5", v)
	}
}

func TestApply_NestedValueIsRejected(t *testing.T) {
	f := load(t, "create:\n  prompt:\n    text: hi\n")
	if _, err := Apply(flags(), f); err == nil {
		t.Fatal("a nested map was accepted as a flag value")
	}
}

func TestLoad_Rejections(t *testing.T) {
	cases := []struct {
		name, body, want string
	}{
		{"future schema", "schema_version: \"2\"\ncreate:\n  bgm: true\n", "schema_version"},
		{"empty create", "name: x\ncreate: {}\n", "empty"},
		{"unknown top-level key", "craete:\n  bgm: true\n", "craete"},
		{"not yaml", "\tthis: [is\n", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Load(write(t, tc.body))
			if err == nil {
				t.Fatal("accepted")
			}
			if tc.want != "" && !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %v does not mention %q", err, tc.want)
			}
		})
	}
}

func TestLoad_NameDefaultsToTheFilename(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "brand-explainer.yaml")
	if err := os.WriteFile(p, []byte("create:\n  bgm: true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if f.Name != "brand-explainer" {
		t.Fatalf("Name = %q, want the filename stem", f.Name)
	}
}

func TestResolve_ByNameFromTheConfigDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("VIBEKNOW_CONFIG_HOME", home)
	dir := filepath.Join(home, "presets")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"brand.yaml", "shorts.yml", "notes.txt"} {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("create:\n  bgm: true\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, ref := range []string{"brand", "shorts"} {
		got, err := Resolve(ref)
		if err != nil {
			t.Fatalf("Resolve(%q): %v", ref, err)
		}
		if filepath.Dir(got) != dir {
			t.Fatalf("Resolve(%q) = %q, want a file in %s", ref, got, dir)
		}
	}

	// A typo is the common case, and the fix is the list of what exists —
	// this feature ships no `preset list` command, so the error is it.
	_, err := Resolve("brnad")
	if err == nil {
		t.Fatal("unknown name accepted")
	}
	msg := err.Error()
	for _, want := range []string{"brnad", dir, "brand", "shorts"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("error %q missing %q", msg, want)
		}
	}
	if strings.Contains(msg, "notes") {
		t.Fatalf("non-YAML file listed as a preset: %q", msg)
	}
}

func TestResolve_PathRefIsNotLookedUpByName(t *testing.T) {
	t.Setenv("VIBEKNOW_CONFIG_HOME", t.TempDir())
	for _, ref := range []string{"./a.yaml", "dir/b", "c.yml", "/tmp/d.yaml"} {
		got, err := Resolve(ref)
		if err != nil {
			t.Fatalf("Resolve(%q): %v", ref, err)
		}
		if got != ref {
			t.Fatalf("Resolve(%q) = %q, want it used verbatim as a path", ref, got)
		}
	}
}

func TestLoad_MissingPathSaysSo(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "nope.yaml"))
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("want a not-found error, got %v", err)
	}
}

func TestApply_NilPresetIsANoOp(t *testing.T) {
	applied, err := Apply(flags(), nil)
	if err != nil || applied != nil {
		t.Fatalf("Apply(fs, nil) = (%v, %v), want (nil, nil)", applied, err)
	}
}
