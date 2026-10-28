package figlens

import (
	"context"
	"fmt"
	"net/url"
)

// Theme suites accepted by GET /v1/themes?type=. Each creation mode draws
// from exactly one suite: the standard line (and replica, and 原稿锁定)
// from design-suite, image mode from image2-suite, hand-drawn mode from
// hand-draw-suite, canvas mode from stage-craft-suite. Suite names are
// backend wire values.
const (
	ThemeSuiteDesign     = "design-suite"
	ThemeSuiteImage2     = "image2-suite"
	ThemeSuiteHandDraw   = "hand-draw-suite"
	ThemeSuiteStageCraft = "stage-craft-suite"
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
	// Scenes is populated for image2-suite only: the scene cards a style
	// unfolds into, selected with the stream request's theme_scene.
	Scenes []ThemeScene `json:"scenes,omitempty"`
	// Palettes is populated for stage-craft-suite only. An entry there is
	// an art direction; each palette under it is itself a valid theme id
	// that pins the colours as well as the direction.
	Palettes []ThemePalette `json:"themes,omitempty"`
}

// ThemeScene is one scene card of an image2 style.
type ThemeScene struct {
	Slug   string `json:"slug"`
	Name   string `json:"name"`
	Domain string `json:"domain,omitempty"`
}

// ThemePalette is one colour scheme of a canvas art direction.
type ThemePalette struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Colors struct {
		Bg     string `json:"bg"`
		Accent string `json:"accent"`
	} `json:"colors"`
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
	// catalog theme it was derived from (baseThemeId). An image2_theme
	// asset is either a style or an IP character (Category); only a style
	// can stand in for a theme — characters go to --ip-asset.
	Config struct {
		BaseThemeID string `json:"baseThemeId"`
		Category    string `json:"category"`
	} `json:"config"`
}

// Image2 asset categories (config.category of an image2_theme asset).
const (
	Image2CategoryStyle = "style"
	Image2CategoryIP    = "ip"
)

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

// IPAsset is one character from GET /v1/assets/image2Theme/ipCatalog: the
// public ones first, then the caller's own trained ones. Ref is the value
// the init and stream requests' ip_asset field takes.
type IPAsset struct {
	Ref      string `json:"ref"`
	Name     string `json:"name"`
	ImageURL string `json:"image_url"`
	Subtitle string `json:"subtitle,omitempty"`
	Public   bool   `json:"public"`
	// Enabled marks the caller's account default among their own; the
	// backend uses it when a run names no IP. Always 0 on public rows.
	Enabled int `json:"enabled"`
}

// IPAssetNone is the ip_asset value that turns the account default off for
// one run.
const IPAssetNone = "none"

// ListIPAssets fetches the characters an image-mode run can feature. Only
// usable ones are listed: the backend leaves out any without a reference
// image.
func (c *Client) ListIPAssets(ctx context.Context) ([]IPAsset, error) {
	var resp struct {
		Items []IPAsset `json:"items"`
	}
	if err := c.http.Do(ctx, "GET", "/v1/assets/image2Theme/ipCatalog", nil, &resp); err != nil {
		return nil, fmt.Errorf("list ip assets: %w", err)
	}
	return resp.Items, nil
}
