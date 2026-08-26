package figlens

import (
	"context"
	"fmt"
	"strings"
)

// WorkStatus values mirror the figlens backend's WorkStatus enum.
// Kept in lockstep with the backend so the CLI can reason about a work's
// lifecycle without hardcoding magic numbers at every call site.
const (
	WorkStatusGenerating = 0 // pipeline still running
	WorkStatusActive     = 1 // generation completed, preview/share URL live
	WorkStatusDeleted    = 2 // user-deleted; do not surface as ready
	WorkStatusFailed     = 3 // pipeline failed terminally
)

type Work struct {
	ID         int64  `json:"id"`
	SessionID  string `json:"session_id"`
	Title      string `json:"title"`
	HtmlPath   string `json:"html_path"`
	VideoPath  string `json:"video_path"`
	CoverURL   string `json:"cover_url"`
	ShareToken string `json:"share_token"`
	Exporting  int    `json:"exporting"`
	Duration   int64  `json:"duration"`
	Engine     string `json:"engine,omitempty"`
	Status     int    `json:"status"`
	// Bgm / Subtitle are "on" / "off", and SubtitleStyle is the stored style
	// as raw JSON. They are read here so a partial change can be merged onto
	// what is already set: the style endpoint overwrites wholesale, so
	// sending one field without the rest silently clears the others.
	Bgm           string `json:"bgm,omitempty"`
	Subtitle      string `json:"subtitle,omitempty"`
	SubtitleStyle string `json:"subtitle_style,omitempty"`
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
	ID         int64  `json:"id"`
	SessionID  string `json:"session_id"`
	Title      string `json:"title"`
	HtmlPath   string `json:"html_path"`
	VideoPath  string `json:"video_path"`
	CoverURL   string `json:"cover_url"`
	ShareToken string `json:"share_token"`
	Exporting  int    `json:"exporting"`
	Duration   int64  `json:"duration"`
	Engine     string `json:"engine,omitempty"`
	Status     int    `json:"status"`
	CreatedAt  string `json:"created_at"`
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

// AssetURL resolves a work-row asset reference into something fetchable.
//
// The work row is inconsistent about this: cover_url is already an address,
// while video_path is an object key that has to be signed first. Callers
// that just want the bytes should not have to know which field is which.
func (c *Client) AssetURL(ctx context.Context, ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", nil
	}
	if strings.HasPrefix(ref, "http://") || strings.HasPrefix(ref, "https://") {
		return ref, nil
	}
	return c.SignedURL(ctx, ref)
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
