package vibeknow

import (
	"context"
	"fmt"
)

type VoiceTemplate struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Category string `json:"category"`
	// Language is the template's locale group (zh-CN, en-US, …). Empty for
	// cloned voices, which are language-independent by design.
	Language        string   `json:"language,omitempty"`
	Tags            []string `json:"tags"`
	SpeechVoiceID   string   `json:"speech_voice_id"`
	PreviewAudioURL string   `json:"preview_audio_url,omitempty"`
}

// LanguageVoices is one language group of the pipeline voice catalog.
type LanguageVoices struct {
	Language string          `json:"language"`
	Voices   []VoiceTemplate `json:"voices"`
}

// PipelineVoices is the /v1/pipeline-voices payload: public templates
// grouped by language, plus the caller's own cloned voices (usable with
// any language; empty when unauthenticated).
type PipelineVoices struct {
	Languages []LanguageVoices `json:"languages"`
	Cloned    []VoiceTemplate  `json:"cloned"`
}

// Flatten returns every voice in catalog order — language groups first,
// cloned voices last — for consumers that key on the numeric ID rather
// than the grouping (e.g. `--voice 3` resolution).
func (p PipelineVoices) Flatten() []VoiceTemplate {
	var out []VoiceTemplate
	for _, g := range p.Languages {
		out = append(out, g.Voices...)
	}
	return append(out, p.Cloned...)
}

// ListPipelineVoices fetches the v3 pipeline voice catalog. This supersedes
// the flat /v1/voice-templates listing: it is the endpoint the product's
// multi-language voice picker feeds from, and the only one that returns the
// caller's cloned voices alongside the public templates.
func (c *Client) ListPipelineVoices(ctx context.Context) (PipelineVoices, error) {
	var resp PipelineVoices
	if err := c.http.Do(ctx, "GET", "/v1/pipeline-voices", nil, &resp); err != nil {
		return PipelineVoices{}, fmt.Errorf("list pipeline voices: %w", err)
	}
	return resp, nil
}
