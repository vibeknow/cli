package figlens

import (
	"context"
	"fmt"
	"net/url"
)

// Theme suites accepted by GET /v1/themes?type=. Each creation mode draws
// from exactly one suite: the standard line (and replica, and 原稿锁定)
// from design-suite, image mode from image2-suite, hand-drawn mode from
// hand-draw-suite. Suite names are backend wire values.
const (
	ThemeSuiteDesign   = "design-suite"
	ThemeSuiteImage2   = "image2-suite"
	ThemeSuiteHandDraw = "hand-draw-suite"
)

// Theme is one style catalog entry. ID is what `vk create --theme` sends;
// empty theme on the stream request means the pipeline auto-selects.
type Theme struct {
	ID    string   `json:"id"`
	Name  string   `json:"name"`
	Desc  string   `json:"desc"`
	Tags  []string `json:"tags"`
	Badge string   `json:"badge,omitempty"`
	// Preview is populated for hand-draw-suite only: CDN URLs for the
	// style's horizontal/vertical webp + poster previews.
	Preview *ThemePreview `json:"preview,omitempty"`
}

type ThemePreview struct {
	Webp    string `json:"webp"`
	Poster  string `json:"poster"`
	WebpV   string `json:"webpV"`
	PosterV string `json:"posterV"`
}

// ListThemes fetches the style catalog for one suite. The backend 400s on
// any other suite value, so callers should pass one of the ThemeSuite
// constants.
func (c *Client) ListThemes(ctx context.Context, suite string) ([]Theme, error) {
	var themes []Theme
	path := "/v1/themes?type=" + url.QueryEscape(suite)
	if err := c.http.Do(ctx, "GET", path, nil, &themes); err != nil {
		return nil, fmt.Errorf("list themes: %w", err)
	}
	return themes, nil
}
