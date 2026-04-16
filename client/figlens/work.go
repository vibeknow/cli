package figlens

import (
	"context"
	"fmt"
)

type Work struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	VideoPath string `json:"video_path"`
	CoverURL  string `json:"cover_url"`
	Duration  int    `json:"duration"`
}

func (c *Client) GetWorkBySession(ctx context.Context, sessionID string) (*Work, error) {
	var w Work
	path := fmt.Sprintf("/v1/works/detailBySession?session_id=%s", sessionID)
	if err := c.do(ctx, "GET", path, nil, &w); err != nil {
		return nil, fmt.Errorf("get work by session: %w", err)
	}
	return &w, nil
}
