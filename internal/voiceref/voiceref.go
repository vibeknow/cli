// Package voiceref resolves a --voice value into the speech_voice_id the
// backend's TTS keys on. Shared by every command that takes --voice, so a
// `vk voice list` reference number means the same thing everywhere.
package voiceref

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/vibeknow/cli/client/vibeknow"
	"github.com/vibeknow/cli/internal/clerr"
	"github.com/vibeknow/cli/internal/cmdutil"
	"github.com/vibeknow/cli/internal/i18n"
)

// numericRe matches a --voice value given as the reference number printed in
// the first column of `vk voice list`, rather than a speech_voice_id.
var numericRe = regexp.MustCompile(`^[0-9]+$`)

// Resolve turns whatever the user passed to --voice into the value the
// backend's TTS actually keys on, the speech_voice_id.
//
// `vk voice list` prints two identifiers per row and only the long one works;
// passing the short one used to be accepted silently and blow up minutes
// later inside the TTS node with "音色不存在" — after cover and background
// images had already been generated and billed. Rather than making the user
// learn which column is real, a numeric value is looked up and translated
// here, before anything is uploaded or spent.
//
// Non-numeric values pass through untouched: they are already speech_voice_ids,
// and cloned voices do not appear in the template list, so a "not in the list"
// check would reject valid input.
func Resolve(ctx context.Context, flag string) (string, error) {
	ref := strings.TrimSpace(flag)
	if !NeedsLookup(ref) {
		return ref, nil
	}

	_, url, tp, err := cmdutil.Default().Service("vibeknow")
	if err != nil {
		return "", err
	}
	catalog, err := vibeknow.New(url, tp).ListPipelineVoices(ctx)
	if err != nil {
		return "", fmt.Errorf("%s: %w", i18n.T("create.err.voice_lookup_failed"), err)
	}
	// Flatten includes cloned voices, so `--voice <#>` works for the
	// user's own clones too, not just public templates.
	return Map(ref, catalog.Flatten())
}

// NeedsLookup reports whether a --voice value is a `vk voice list`
// reference number that has to be translated, as opposed to a speech_voice_id
// that can go to the backend as-is.
func NeedsLookup(ref string) bool {
	return ref != "" && numericRe.MatchString(ref)
}

// Map translates a list reference number into its speech_voice_id.
func Map(ref string, templates []vibeknow.VoiceTemplate) (string, error) {
	for _, t := range templates {
		if strconv.Itoa(t.ID) != ref {
			continue
		}
		if t.SpeechVoiceID == "" {
			// A listed template with no usable id is a backend data
			// problem; failing here beats forwarding an empty voice.
			return "", clerr.Validation(i18n.T("create.err.voice_no_speech_id", ref, t.Name))
		}
		fmt.Fprintln(os.Stderr, i18n.T("create.voice_resolved", ref, t.Name, t.SpeechVoiceID))
		return t.SpeechVoiceID, nil
	}
	return "", clerr.Validation(i18n.T("create.err.voice_unknown_ref", ref))
}
