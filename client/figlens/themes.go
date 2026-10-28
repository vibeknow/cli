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

// User asset kinds that stand in for a catalog theme. A theme asset is
// referenced as "ua_<id>" in the stream request's theme field; the two kinds
// share the prefix but the backend accepts each only in its own mode.
const (
	AssetTypeTheme       = "theme"        // 灵活创作 custom theme (我的主题)
	AssetTypeImage2Theme = "image2_theme" // 图解视频 trained style
)

// ThemeAsset is one of the caller's own theme assets from
// GET /v1/assets?type=theme|image2_theme.
type ThemeAsset struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	Status int    `json:"status"`
	// Config is the asset's stored config. For a theme asset it names the
	// catalog theme it was derived from (baseThemeId); image2 styles have
	// no base.
	Config struct {
		BaseThemeID string `json:"baseThemeId"`
	} `json:"config"`
}

// Token is the value the stream request's theme field takes for this asset.
func (a ThemeAsset) Token() string { return fmt.Sprintf("ua_%d", a.ID) }

// ListThemeAssets fetches the caller's own assets of one theme kind, all
// states. Only UserAssetStatusActive ones are usable: the backend ignores
// any other without an error.
func (c *Client) ListThemeAssets(ctx context.Context, assetType string) ([]ThemeAsset, error) {
	var assets []ThemeAsset
	path := "/v1/assets?type=" + url.QueryEscape(assetType)
	if err := c.http.Do(ctx, "GET", path, nil, &assets); err != nil {
		return nil, fmt.Errorf("list %s assets: %w", assetType, err)
	}
	return assets, nil
}
