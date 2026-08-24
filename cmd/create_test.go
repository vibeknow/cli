package cmd

import (
	"strings"
	"testing"

	"github.com/vibeknow/cli/client/figlens"
	"github.com/vibeknow/cli/client/vibeknow"
	"github.com/vibeknow/cli/internal/clerr"
)

func TestResolveMode(t *testing.T) {
	tests := []struct {
		desc           string
		mode           string
		scriptLockFlag bool
		wantKind       string
		wantLock       bool
		wantDeprecated bool
		wantErr        bool
	}{
		{desc: "empty is the freeform line", mode: "", wantKind: ""},
		{desc: "replica", mode: "replica", wantKind: figlens.VideoKindReplica},
		{desc: "image", mode: "image", wantKind: figlens.VideoKindImage2},
		{desc: "handdraw", mode: "handdraw", wantKind: figlens.VideoKindHandDraw},
		{desc: "case-insensitive", mode: "HandDraw", wantKind: figlens.VideoKindHandDraw},

		// script_lock is a separate axis now: it must survive every --mode,
		// not be swallowed by one.
		{desc: "lock alone", mode: "", scriptLockFlag: true, wantKind: "", wantLock: true},
		{desc: "lock rides image", mode: "image", scriptLockFlag: true, wantKind: figlens.VideoKindImage2, wantLock: true},
		{desc: "lock rides replica", mode: "replica", scriptLockFlag: true, wantKind: figlens.VideoKindReplica, wantLock: true},
		{desc: "lock rides handdraw", mode: "handdraw", scriptLockFlag: true, wantKind: figlens.VideoKindHandDraw, wantLock: true},

		// The deprecated alias must land on the boolean, NOT on video_kind —
		// sending "script_lock" as a video_kind is exactly the silent
		// no-op this split exists to fix.
		{desc: "deprecated alias sets the boolean", mode: "script", wantKind: "", wantLock: true, wantDeprecated: true},
		{desc: "deprecated alias case-insensitive", mode: "SCRIPT", wantKind: "", wantLock: true, wantDeprecated: true},

		// Backend wire values are not CLI flag values.
		{desc: "rejects backend jargon script_lock", mode: "script_lock", wantErr: true},
		{desc: "rejects backend jargon image2", mode: "image2", wantErr: true},
		{desc: "rejects backend jargon hand-draw", mode: "hand-draw", wantErr: true},
		{desc: "rejects unknown", mode: "bogus", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			kind, lock, deprecated, err := resolveMode(tt.mode, tt.scriptLockFlag)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q", tt.mode)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if kind != tt.wantKind {
				t.Errorf("video_kind = %q, want %q", kind, tt.wantKind)
			}
			if lock != tt.wantLock {
				t.Errorf("script_lock = %v, want %v", lock, tt.wantLock)
			}
			if deprecated != tt.wantDeprecated {
				t.Errorf("deprecated = %v, want %v", deprecated, tt.wantDeprecated)
			}
		})
	}
}

func TestResolveAspect(t *testing.T) {
	tests := []struct {
		flag    string
		want    string
		wantErr bool
	}{
		{"", "", false},
		{"horizontal", "horizontal", false},
		{"vertical", "vertical", false},
		{"16:9", "horizontal", false},
		{"9:16", "vertical", false},
		{"HORIZONTAL", "horizontal", false},
		{"square", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.flag, func(t *testing.T) {
			got, err := resolveAspect(tt.flag)
			if tt.wantErr && err == nil {
				t.Fatalf("expected error for %q", tt.flag)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("resolveAspect(%q) = %q, want %q", tt.flag, got, tt.want)
			}
		})
	}
}

func TestResolveMode_ErrorMessageMentionsValues(t *testing.T) {
	_, _, _, err := resolveMode("xyz", false)
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	for _, want := range []string{"replica", "image", "handdraw", "--script-lock"} {
		if !strings.Contains(msg, want) {
			// --script-lock is in there because a user typing a bad --mode
			// is the most likely person to be reaching for the old
			// `--mode script`; the error is where they will look.
			t.Errorf("error must mention %q, got: %q", want, msg)
		}
	}
}

func TestResolveEngine(t *testing.T) {
	tests := []struct {
		flag    string
		want    figlens.Engine
		wantErr bool
	}{
		{"", figlens.EnginePipeline, false},
		{"pipeline", figlens.EnginePipeline, false},
		{"PIPELINE", figlens.EnginePipeline, false},
		{"agent", figlens.EngineAgent, false},
		{"Agent", figlens.EngineAgent, false},
		{"suite", figlens.EnginePipeline, true},
		{"v2", figlens.EnginePipeline, true},
		{"bogus", figlens.EnginePipeline, true},
	}
	for _, tt := range tests {
		t.Run(tt.flag, func(t *testing.T) {
			got, err := resolveEngine(tt.flag)
			if tt.wantErr && err == nil {
				t.Fatalf("expected error for %q", tt.flag)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !tt.wantErr && got != tt.want {
				t.Fatalf("resolveEngine(%q) = %v, want %v", tt.flag, got, tt.want)
			}
		})
	}
}

func TestEngineAgentRejectsReplica(t *testing.T) {
	err := validateEngineModeCombo(figlens.EngineAgent, figlens.VideoKindReplica)
	if err == nil {
		t.Fatal("expected error: --engine agent + --mode replica should be rejected")
	}
	if !strings.Contains(err.Error(), "replica") || !strings.Contains(err.Error(), "pipeline") {
		t.Fatalf("error message should mention both replica and pipeline, got: %q", err.Error())
	}
}

func TestEngineAgentAllowsDefaultMode(t *testing.T) {
	// The agent engine only has its own line. Script lock is not listed
	// here because it is no longer a video_kind at all — it rides along
	// as a boolean and constrains nothing about the engine.
	if err := validateEngineModeCombo(figlens.EngineAgent, ""); err != nil {
		t.Fatalf("--engine agent with no --mode should be allowed: %v", err)
	}
}

func TestEnginePipelineAllowsAllModes(t *testing.T) {
	for _, mode := range []string{"", figlens.VideoKindReplica, figlens.VideoKindImage2, figlens.VideoKindHandDraw} {
		if err := validateEngineModeCombo(figlens.EnginePipeline, mode); err != nil {
			t.Fatalf("--engine pipeline + mode %q should be allowed: %v", mode, err)
		}
	}
}

func TestValidateEngineModeCombo_HandDraw(t *testing.T) {
	// hand-draw runs a dedicated graph the agent engine never dispatches
	// to; accepting the combination would silently render an ordinary
	// video instead of a hand-drawn one.
	err := validateEngineModeCombo(figlens.EngineAgent, figlens.VideoKindHandDraw)
	if err == nil {
		t.Fatal("agent+handdraw must be rejected (no hand-draw branch on the agent engine)")
	}
	if !strings.Contains(err.Error(), "handdraw") || !strings.Contains(err.Error(), "pipeline") {
		t.Fatalf("error should mention both handdraw and pipeline, got: %q", err.Error())
	}
}

func TestDocIDRegex(t *testing.T) {
	tests := []struct {
		in    string
		match bool
		desc  string
	}{
		// Legacy form (kept for backward compat).
		{"doc_abc12345", true, "legacy doc_<alnum>"},
		{"doc_a1b2c3d4e5f6", true, "legacy long"},
		{"doc_short", false, "legacy too-short suffix"},

		// Vectoria native UUID form (what `vk create` actually prints).
		{"3c498964-2c54-4ac0-97e8-1125d3bed640", true, "real vectoria UUID"},
		{"00000000-0000-0000-0000-000000000000", true, "all-zero UUID"},

		// Negative: file paths and URLs must not match.
		{"./my-file.docx", false, "relative path"},
		{"/abs/path.pdf", false, "absolute path"},
		{"my-file.docx", false, "filename with extension"},
		{"https://example.com/x", false, "URL"},

		// Negative: malformed UUIDs.
		{"3c498964-2c54-4ac0-97e8", false, "truncated UUID"},
		{"3C498964-2C54-4AC0-97E8-1125D3BED640", false, "uppercase (vectoria emits lowercase)"},
		{"3c498964_2c54_4ac0_97e8_1125d3bed640", false, "underscores not hyphens"},
		{"xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx", false, "non-hex chars"},
	}
	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			got := docIDRe.MatchString(tt.in)
			if got != tt.match {
				t.Fatalf("docIDRe.MatchString(%q) = %v, want %v", tt.in, got, tt.match)
			}
		})
	}
}

func TestResolveMode_ImageMode(t *testing.T) {
	got, _, _, err := resolveMode("image", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != figlens.VideoKindImage2 {
		t.Fatalf("resolveMode(image) = %q, want %q", got, figlens.VideoKindImage2)
	}
	// The wire value is an internal model codename; it must not be
	// accepted (and thereby advertised) as a CLI flag value.
	if _, _, _, err := resolveMode("image2", false); err == nil {
		t.Fatal("resolveMode must reject the backend codename \"image2\"")
	}
}

func TestValidateEngineModeCombo_Image2(t *testing.T) {
	if err := validateEngineModeCombo(figlens.EngineAgent, figlens.VideoKindImage2); err == nil {
		t.Fatal("agent+image2 must be rejected (no image2 branch on the agent engine)")
	}
	if err := validateEngineModeCombo(figlens.EnginePipeline, figlens.VideoKindImage2); err != nil {
		t.Fatalf("pipeline+image2 must be allowed, got %v", err)
	}
}

func TestVoiceRefNeedsLookup(t *testing.T) {
	tests := []struct {
		ref  string
		want bool
		why  string
	}{
		{"1", true, "list reference number"},
		{"42", true, "multi-digit reference"},
		{"", false, "unset — nothing to resolve"},
		{"t260312180132IV37e603", false, "already a speech_voice_id"},
		// Cloned voices are not in the template list, so anything
		// non-numeric must pass through rather than be rejected as unknown.
		{"custom_clone_abc", false, "cloned voice id"},
	}
	for _, tt := range tests {
		t.Run(tt.why, func(t *testing.T) {
			if got := voiceRefNeedsLookup(tt.ref); got != tt.want {
				t.Fatalf("voiceRefNeedsLookup(%q) = %v, want %v", tt.ref, got, tt.want)
			}
		})
	}
}

func TestMapVoiceRef(t *testing.T) {
	templates := []vibeknow.VoiceTemplate{
		{ID: 1, Name: "若溪", SpeechVoiceID: "t260312180132IV37e603"},
		{ID: 2, Name: "无声", SpeechVoiceID: ""},
	}

	got, err := mapVoiceRef("1", templates)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The whole point: the number the user reads off the table must not be
	// what reaches the backend — TTS only knows the speech_voice_id.
	if got != "t260312180132IV37e603" {
		t.Fatalf("mapVoiceRef(1) = %q, want the speech_voice_id", got)
	}

	// An unknown reference must fail here, at exit 2, rather than minutes
	// later inside the TTS node with cover and background images already
	// generated and billed.
	_, err = mapVoiceRef("99", templates)
	if err == nil {
		t.Fatal("mapVoiceRef must reject a reference that is not in the list")
	}
	if clerr.ExitCodeFor(err) != clerr.ExitValidation {
		t.Errorf("unknown voice ref should exit %d, got %d", clerr.ExitValidation, clerr.ExitCodeFor(err))
	}
	if !strings.Contains(err.Error(), "voice list") {
		t.Errorf("error should point at `vk voice list`, got: %q", err.Error())
	}

	// A listed voice with no usable id must not be forwarded as an empty
	// voice — that silently swaps the user's choice for the default.
	if _, err := mapVoiceRef("2", templates); err == nil {
		t.Fatal("mapVoiceRef must reject a template with an empty speech_voice_id")
	}
}

func TestResolveImageIndexes(t *testing.T) {
	tests := []struct {
		flag    string
		want    []int
		wantErr bool
	}{
		{"", nil, false},
		{"1,3,5", []int{1, 3, 5}, false},
		{" 2 , 4 ", []int{2, 4}, false},
		{"3,3,3", []int{3}, false}, // duplicates collapse
		{"1,,2", []int{1, 2}, false},
		{"0", nil, true},  // image_index is 1-based on the backend
		{"-1", nil, true},
		{"a,b", nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.flag, func(t *testing.T) {
			got, err := resolveImageIndexes(tt.flag)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q", tt.flag)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("resolveImageIndexes(%q) = %v, want %v", tt.flag, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("resolveImageIndexes(%q) = %v, want %v", tt.flag, got, tt.want)
				}
			}
		})
	}
}
