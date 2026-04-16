package vibeknow

import (
	"context"
	"fmt"
)

type VoiceTemplate struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Language   string `json:"language"`
	Gender     string `json:"gender"`
	PreviewURL string `json:"preview_url,omitempty"`
}

func (c *Client) ListVoiceTemplates(ctx context.Context) ([]VoiceTemplate, error) {
	var resp struct {
		Items []VoiceTemplate `json:"items"`
	}
	if err := c.do(ctx, "GET", "/v1/voice-templates?page=1&size=100", nil, &resp); err != nil {
		return nil, fmt.Errorf("list voice templates: %w", err)
	}
	return resp.Items, nil
}
