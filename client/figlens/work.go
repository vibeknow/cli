package figlens

import (
	"context"
	"fmt"
)

type Work struct {
	ID        int64  `json:"id"`
	Title     string `json:"title"`
	VideoPath string `json:"video_path"`
	CoverURL  string `json:"cover_url"`
	Duration  int    `json:"duration"`
}

func (c *Client) GetWorkBySession(ctx context.Context, sessionID string) (*Work, error) {
	var w Work
	path := fmt.Sprintf("/v1/works/detailBySession?session_id=%s", sessionID)
	if err := c.http.Do(ctx, "GET", path, nil, &w); err != nil {
		return nil, fmt.Errorf("get work by session: %w", err)
	}
	return &w, nil
}

type WorkListItem struct {
	ID        int64  `json:"id"`
	SessionID string `json:"session_id"`
	Title     string `json:"title"`
	VideoPath string `json:"video_path"`
	CoverURL  string `json:"cover_url"`
	Duration  int64  `json:"duration"`
	Status    int    `json:"status"`
	CreatedAt string `json:"created_at"`
}

func (c *Client) ListWorks(ctx context.Context, page, size int) ([]WorkListItem, int, error) {
	var resp struct {
		List  []WorkListItem `json:"list"`
		Total int            `json:"total"`
	}
	path := fmt.Sprintf("/v1/works/page?page=%d&size=%d", page, size)
	if err := c.http.Do(ctx, "GET", path, nil, &resp); err != nil {
		return nil, 0, fmt.Errorf("list works: %w", err)
	}
	return resp.List, resp.Total, nil
}

func (c *Client) GetVideoURL(ctx context.Context, workID int64) (string, error) {
	var resp struct {
		URL string `json:"url"`
	}
	path := fmt.Sprintf("/v1/works/videoUrl?id=%d", workID)
	if err := c.http.Do(ctx, "GET", path, nil, &resp); err != nil {
		return "", fmt.Errorf("get video url: %w", err)
	}
	return resp.URL, nil
}
