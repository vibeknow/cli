package figlens

import (
	"context"
	"fmt"
	"slices"
	"strings"
)

// The subtitle catalogs: which font families may be used, and the ready-made
// looks the product ships. Both are read-only, free, and unauthenticated as
// far as billing goes — they exist so a caller never has to guess at a value
// the backend will reject.
//
// The backend serves all of this from one vendored file per catalog
// (assets/design-suite/catalog/{fonts,subtitle-presets}.json) and calls it the
// single source: the web font picker, the render-time whitelist and these
// endpoints all read the same bytes. So what this returns is what will be
// accepted, not an approximation of it.

// SubtitleFont is one family that may be used for subtitles.
//
// This is a subset of the full font catalog: families marked
// subtitleEligible:false — display faces too heavy to read at subtitle size —
// are filtered out server-side, so everything returned here is a valid
// --subtitle-font value.
//
// The catalog also records which numeric weights each family actually ships,
// but the endpoint does not return them, so a caller cannot tell whether a
// family has the weight it is about to ask for. An unavailable weight is not
// an error; the renderer falls back.
type SubtitleFont struct {
	Family string `json:"family"`
	Label  string `json:"label"`
	// CdnURL is the @font-face stylesheet. Of no use to a CLI, but it is the
	// reason a family is listed at all: the server drops any entry without
	// one, since a font with no stylesheet cannot render.
	CdnURL string `json:"cdnUrl"`
}

type subtitleFontsResponse struct {
	Fonts []SubtitleFont `json:"fonts"`
}

// ListSubtitleFonts returns every family that may be used for subtitles, in
// catalog order.
func (c *Client) ListSubtitleFonts(ctx context.Context) ([]SubtitleFont, error) {
	var out subtitleFontsResponse
	if err := c.http.Do(ctx, "GET", "/v1/works/subtitleFonts", nil, &out); err != nil {
		return nil, fmt.Errorf("list subtitle fonts: %w", err)
	}
	return out.Fonts, nil
}

// SubtitlePreset is one ready-made subtitle look, as shipped by the design
// team and offered in the web editor's one-click list.
type SubtitlePreset struct {
	Name  string             `json:"name"`
	Patch SubtitleStylePatch `json:"patch"`
}

// SubtitleStylePatch is a partial style: the fields a preset sets, and only
// those.
//
// Every field is a pointer because absent and zero mean different things
// here. The presets rely on it: the ones that put subtitles on a solid plate
// carry "strokeWidth": 0 to switch off an outline the work may already have,
// while leaving font size and vertical position alone by not mentioning them.
// Decoding into value fields would collapse those two cases and turn "clear
// the outline" into "change nothing".
type SubtitleStylePatch struct {
	FontFamily      *string  `json:"fontFamily,omitempty"`
	FontSize        *int     `json:"fontSize,omitempty"`
	FontWeight      *int     `json:"fontWeight,omitempty"`
	Color           *string  `json:"color,omitempty"`
	BackgroundColor *string  `json:"backgroundColor,omitempty"`
	BottomPercent   *float64 `json:"bottomPercent,omitempty"`
	StrokeColor     *string  `json:"strokeColor,omitempty"`
	StrokeWidth     *float64 `json:"strokeWidth,omitempty"`
	Animation       *string  `json:"animation,omitempty"`
}

// Apply merges the patch onto an existing style and returns the result,
// leaving the input untouched.
//
// Merge, not replace: a preset describes a look — colour, plate, outline,
// often a face and weight — and deliberately says nothing about size,
// position or entry animation, which belong to the video rather than to the
// look. Replacing wholesale would silently reset those every time a preset
// was applied.
func (p SubtitleStylePatch) Apply(s SubtitleStyle) SubtitleStyle {
	if p.FontFamily != nil {
		s.FontFamily = *p.FontFamily
	}
	if p.FontSize != nil {
		s.FontSize = *p.FontSize
	}
	if p.FontWeight != nil {
		s.FontWeight = *p.FontWeight
	}
	if p.Color != nil {
		s.Color = *p.Color
	}
	if p.BackgroundColor != nil {
		s.BackgroundColor = *p.BackgroundColor
	}
	if p.BottomPercent != nil {
		s.BottomPercent = *p.BottomPercent
	}
	if p.StrokeColor != nil {
		s.StrokeColor = *p.StrokeColor
	}
	if p.StrokeWidth != nil {
		s.StrokeWidth = *p.StrokeWidth
	}
	if p.Animation != nil {
		s.Animation = *p.Animation
	}
	return s
}

type subtitlePresetsResponse struct {
	Presets []SubtitlePreset `json:"presets"`
}

// ListSubtitlePresets returns the ready-made subtitle looks, in the order the
// product shows them.
func (c *Client) ListSubtitlePresets(ctx context.Context) ([]SubtitlePreset, error) {
	var out subtitlePresetsResponse
	if err := c.http.Do(ctx, "GET", "/v1/works/subtitlePresets", nil, &out); err != nil {
		return nil, fmt.Errorf("list subtitle presets: %w", err)
	}
	return out.Presets, nil
}

// Ranges the backend applies to a subtitle style. It clamps rather than
// refuses, which is worse than it sounds for a caller: ask for a subtitle at
// 150% of the frame height and the request succeeds, having quietly stored
// something else. Checking here means an impossible number is reported as the
// mistake it is instead of being silently rewritten.
const (
	// SubtitleBottomMin / SubtitleBottomMax bound the distance from the
	// bottom of the frame, as a fraction of frame height. The margins exist
	// so a subtitle cannot be positioned entirely off-frame.
	SubtitleBottomMin = 0.02
	SubtitleBottomMax = 0.98

	// SubtitleStrokeWidthMax caps the outline in px against the unscaled
	// frame. 0 means no outline.
	SubtitleStrokeWidthMax = 12.0

	// SubtitleFontWeightMin / SubtitleFontWeightMax are the CSS weight range.
	// The backend does not check this one at all — it stores whatever it is
	// given and lets the renderer cope — so this bound is the only thing
	// standing between a typo and a subtitle rendered at some default weight
	// with no explanation.
	SubtitleFontWeightMin = 100
	SubtitleFontWeightMax = 900
)

// SubtitleAnimations are the entry animations the backend accepts, in its own
// order. Anything else is refused outright.
//
// This is a copy of a server-side constant, with no endpoint to read it from —
// unlike fonts and presets, which are catalogs the backend serves. Keeping the
// list here buys a local rejection that names the twelve valid values instead
// of a round trip that comes back "animation not allowed"; the cost is that a
// value added server-side is unusable until this list catches up, which is why
// an unknown value is reported as unrecognised-by-this-CLI rather than as
// invalid.
var SubtitleAnimations = []string{
	"none", "fade", "fadeup", "fadedown",
	"slideleft", "slideright", "scale", "blur",
	"pop", "springup", "rotate", "karaoke",
}

// NormalizeSubtitleAnimation lower-cases and checks an entry animation. The
// backend matches its whitelist exactly, so "Fade" would be refused on the
// wire; folding case here is the difference between that and it just working.
func NormalizeSubtitleAnimation(v string) (string, bool) {
	got := strings.ToLower(strings.TrimSpace(v))
	return got, slices.Contains(SubtitleAnimations, got)
}
