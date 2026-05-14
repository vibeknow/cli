package cmd

import (
	"strings"
	"testing"

	"github.com/vibeknow/cli/client/figlens"
)

func TestResolveVideoKind(t *testing.T) {
	tests := []struct {
		flag    string
		want    string
		wantErr bool
	}{
		{"", "", false},
		{"replica", "replica", false},
		{"script", "script_lock", false},
		{"SCRIPT", "script_lock", false},
		{"script_lock", "", true}, // backend jargon, not a CLI flag value
		{"bogus", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.flag, func(t *testing.T) {
			got, err := resolveVideoKind(tt.flag)
			if tt.wantErr && err == nil {
				t.Fatalf("expected error for %q", tt.flag)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("resolveVideoKind(%q) = %q, want %q", tt.flag, got, tt.want)
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

func TestResolveVideoKind_ErrorMessageMentionsValues(t *testing.T) {
	_, err := resolveVideoKind("xyz")
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "replica") || !strings.Contains(msg, "script") {
		t.Fatalf("error must list allowed values, got: %q", msg)
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

func TestEngineAgentAllowsOtherModes(t *testing.T) {
	for _, mode := range []string{"", figlens.VideoKindScriptLock} {
		if err := validateEngineModeCombo(figlens.EngineAgent, mode); err != nil {
			t.Fatalf("--engine agent + mode %q should be allowed: %v", mode, err)
		}
	}
}

func TestEnginePipelineAllowsAllModes(t *testing.T) {
	for _, mode := range []string{"", figlens.VideoKindReplica, figlens.VideoKindScriptLock} {
		if err := validateEngineModeCombo(figlens.EnginePipeline, mode); err != nil {
			t.Fatalf("--engine pipeline + mode %q should be allowed: %v", mode, err)
		}
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
