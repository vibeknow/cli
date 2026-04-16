package vibeknow

import (
	"context"
	"fmt"
)

type VoiceTemplate struct {
	ID             int      `json:"id"`
	Name           string   `json:"name"`
	Category       string   `json:"category"`
	Tags           []string `json:"tags"`
	SpeechVoiceID  string   `json:"speech_voice_id"`
	PreviewAudioURL string  `json:"preview_audio_url,omitempty"`
}

func (c *Client) ListVoiceTemplates(ctx context.Context) ([]VoiceTemplate, error) {
	var resp struct {
		List []VoiceTemplate `json:"list"`
	}
	if err := c.http.Do(ctx, "GET", "/v1/voice-templates?page=1&size=100", nil, &resp); err != nil {
		return nil, fmt.Errorf("list voice templates: %w", err)
	}
	return resp.List, nil
}
