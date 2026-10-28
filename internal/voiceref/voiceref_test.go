package voiceref

import (
	"strings"
	"testing"

	"github.com/vibeknow/cli/client/vibeknow"
	"github.com/vibeknow/cli/internal/clerr"
)

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
			if got := NeedsLookup(tt.ref); got != tt.want {
				t.Fatalf("NeedsLookup(%q) = %v, want %v", tt.ref, got, tt.want)
			}
		})
	}
}

func TestMapVoiceRef(t *testing.T) {
	templates := []vibeknow.VoiceTemplate{
		{ID: 1, Name: "若溪", SpeechVoiceID: "t260312180132IV37e603"},
		{ID: 2, Name: "无声", SpeechVoiceID: ""},
	}

	got, err := Map("1", templates)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The whole point: the number the user reads off the table must not be
	// what reaches the backend — TTS only knows the speech_voice_id.
	if got != "t260312180132IV37e603" {
		t.Fatalf("Map(1) = %q, want the speech_voice_id", got)
	}

	// An unknown reference must fail here, at exit 2, rather than minutes
	// later inside the TTS node with cover and background images already
	// generated and billed.
	_, err = Map("99", templates)
	if err == nil {
		t.Fatal("Map must reject a reference that is not in the list")
	}
	if clerr.ExitCodeFor(err) != clerr.ExitValidation {
		t.Errorf("unknown voice ref should exit %d, got %d", clerr.ExitValidation, clerr.ExitCodeFor(err))
	}
	if !strings.Contains(err.Error(), "voice list") {
		t.Errorf("error should point at `vk voice list`, got: %q", err.Error())
	}

	// A listed voice with no usable id must not be forwarded as an empty
	// voice — that silently swaps the user's choice for the default.
	if _, err := Map("2", templates); err == nil {
		t.Fatal("Map must reject a template with an empty speech_voice_id")
	}
}
