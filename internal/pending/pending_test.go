package pending

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func isolate(t *testing.T) {
	t.Helper()
	t.Setenv("VIBEKNOW_CONFIG_HOME", t.TempDir())
}

func TestToken_StableAcrossCalls(t *testing.T) {
	isolate(t)
	p := map[string]any{"session_id": "s1", "credits": 1}

	a, err := Token("export_confirmation", p)
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	b, err := Token("export_confirmation", p)
	if err != nil {
		t.Fatalf("mint again: %v", err)
	}
	if a != b {
		t.Fatalf("a token must survive the process that minted it: %q vs %q", a, b)
	}
	if !strings.HasPrefix(a, TokenPrefix) {
		t.Fatalf("token %q should carry the %q prefix so a caller passing something else can be told what is wrong", a, TokenPrefix)
	}
}

// The point of hashing the payload rather than only displaying it: consent
// is to specific terms. A token that survived a price change would be
// consent to a number the user never saw.
func TestToken_ChangesWithTheTerms(t *testing.T) {
	isolate(t)
	base := map[string]any{"session_id": "s1", "credits": 1}

	got, err := Token("export_confirmation", base)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name    string
		payload map[string]any
	}{
		{"different run", map[string]any{"session_id": "s2", "credits": 1}},
		{"different price", map[string]any{"session_id": "s1", "credits": 9}},
		{"extra term", map[string]any{"session_id": "s1", "credits": 1, "hd": true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			other, err := Token("export_confirmation", tc.payload)
			if err != nil {
				t.Fatal(err)
			}
			if other == got {
				t.Fatalf("%s must mint a different token; both were %q", tc.name, got)
			}
			if Verify(got, "export_confirmation", tc.payload) {
				t.Fatalf("a token minted for other terms must not verify against %v", tc.payload)
			}
		})
	}
}

func TestToken_ChangesWithTheActionType(t *testing.T) {
	isolate(t)
	p := map[string]any{"session_id": "s1"}
	a, _ := Token("export_confirmation", p)
	b, _ := Token("delete_confirmation", p)
	if a == b {
		t.Fatal("two different boundaries must not share a token; a confirmation for one would authorise the other")
	}
}

func TestVerify(t *testing.T) {
	isolate(t)
	p := map[string]any{"session_id": "s1", "credits": 1}
	tok, err := Token("export_confirmation", p)
	if err != nil {
		t.Fatal(err)
	}

	if !Verify(tok, "export_confirmation", p) {
		t.Fatal("the token this gate minted must verify against it")
	}
	if !Verify("  "+tok+"\n", "export_confirmation", p) {
		t.Fatal("surrounding whitespace is a shell artefact, not a different answer")
	}
	for _, bad := range []string{"", "act_", "yes", "act_deadbeefdeadbeefdeadbeef", tok + "x"} {
		if Verify(bad, "export_confirmation", p) {
			t.Fatalf("%q must not verify — a value a caller can guess is not consent", bad)
		}
	}
}

// The key is per-installation, so a token cannot be minted on one machine
// and replayed on another.
func TestToken_DiffersPerInstallation(t *testing.T) {
	p := map[string]any{"session_id": "s1"}

	t.Setenv("VIBEKNOW_CONFIG_HOME", t.TempDir())
	a, err := Token("export_confirmation", p)
	if err != nil {
		t.Fatal(err)
	}

	t.Setenv("VIBEKNOW_CONFIG_HOME", t.TempDir())
	b, err := Token("export_confirmation", p)
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Fatal("two installations minted the same token; the key is not actually per-installation")
	}
}

func TestSecret_FileIsPrivateAndReused(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("VIBEKNOW_CONFIG_HOME", dir)

	if _, err := Token("export_confirmation", nil); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "action.key")
	fi, err := os.Stat(p)
	if err != nil {
		t.Fatalf("key file should have been created: %v", err)
	}
	// Windows has no POSIX mode bits. Go synthesizes them from the read-only
	// attribute, so a writable file always reports 0666 no matter what perm
	// was passed to create it, and asserting 0600 there tests the synthesis
	// rather than the file. Access on that platform is decided by the ACL
	// inherited from the config directory, which this assertion could not see
	// even if the number were right.
	if runtime.GOOS != "windows" {
		if got := fi.Mode().Perm(); got != 0o600 {
			t.Fatalf("key mode = %o, want 600 — it is the thing an agent must not be able to reason its way around", got)
		}
	}

	before, _ := os.ReadFile(p)
	if _, err := Token("other_action", nil); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(p)
	if string(before) != string(after) {
		t.Fatal("the key was rewritten; every previously issued token would have silently stopped verifying")
	}
}

func TestAction_MapCarriesWhatTheCallerMustRelay(t *testing.T) {
	a := Action{
		ActionID: "act_x", Type: "export_confirmation", Blocking: true,
		Message: "costs 1 credit",
		Payload: map[string]any{"credits": 1},
		Options: []Option{
			{ID: "confirm", Effect: EffectResume, Label: "go"},
			{ID: "cancel", Effect: EffectNone, Label: "stop"},
		},
		ResumeCommand: "vk video export --confirm act_x",
	}
	m := a.Map()
	for _, k := range []string{"action_id", "type", "blocking", "message", "options", "resume_command", "payload"} {
		if _, ok := m[k]; !ok {
			t.Fatalf("%q missing: the caller cannot present this decision without it", k)
		}
	}
	opts, ok := m["options"].([]map[string]any)
	if !ok || len(opts) != 2 {
		t.Fatalf("options = %v, want two renderable choices", m["options"])
	}
	if opts[1]["effect"] != EffectNone {
		t.Fatalf("cancel must carry effect=%q so a caller knows it runs nothing at all", EffectNone)
	}
}
