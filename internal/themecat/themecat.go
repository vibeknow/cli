// Package themecat is the theme catalog as `vk create --theme` sees it: the
// official suite for a mode plus the caller's own theme assets, and the
// validation that keeps a theme id from being silently replaced.
//
// Validation is the point of the package. The backend does not reject a
// theme it does not recognise for the run's kind — on the standard line it
// substitutes a fallback theme and the run succeeds (go-figlens
// internal/pipeline/node/video_theme.go, resolveValidDesignThemeIDForVideoKind),
// and the canvas line ignores unknown ids by design. A caller that sent the
// wrong id gets a billed video in a style nobody chose and no error. So the
// CLI checks the id against the same catalog `vk theme list` shows, before
// anything is uploaded.
package themecat

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/vibeknow/cli/client/figlens"
	"github.com/vibeknow/cli/internal/clerr"
	"github.com/vibeknow/cli/internal/i18n"
	"github.com/vibeknow/cli/internal/modes"
)

// Where an entry comes from.
const (
	SourceOfficial = "official"
	SourceCustom   = "custom"
)

// Entry is one theme a mode accepts.
type Entry struct {
	ID     string
	Name   string
	Desc   string
	Tags   []string
	Badge  string
	Source string
	// Base is the catalog theme a custom theme was derived from; empty for
	// official entries and for image2 styles, which have none.
	Base    string
	Preview *figlens.ThemePreview
}

// Lister is the slice of the figlens client the catalog needs.
type Lister interface {
	ListThemes(ctx context.Context, suite string) ([]figlens.Theme, error)
	ListThemeAssets(ctx context.Context, assetType string) ([]figlens.ThemeAsset, error)
}

// bookIntroOnly are the design-suite themes marked bgAbstract. The backend
// allows them only for video_kind book-intro (a line this CLI does not
// expose) and, on every other kind, swaps them for a fallback
// theme without saying so (video_theme.go isDesignThemeAllowedForVideoKind).
// GET /themes lists them anyway and does not carry the flag, so this is a
// copy of a server-side list — errors say so rather than calling the id
// invalid.
var bookIntroOnly = map[string]bool{
	"flow-light":   true,
	"ember-bronze": true,
	"moss-mist":    true,
	"moonlight":    true,
}

// usableIn reports whether an official id survives the backend's per-kind
// filter. Replica is exempt: it has no theme_select node, and its packaging
// step renders work.theme as given.
func usableIn(m modes.Mode, id string) bool {
	return m.Name != modes.Freeform || !bookIntroOnly[id]
}

// List returns what `--theme` accepts in mode m: the caller's own active
// theme assets first (the web lists 我的 first too), then the official
// suite with the entries the backend would replace filtered out.
func List(ctx context.Context, l Lister, m modes.Mode) ([]Entry, error) {
	custom, err := customEntries(ctx, l, m.CustomAssetType)
	if err != nil {
		return nil, err
	}
	official, err := officialEntries(ctx, l, m.Suite)
	if err != nil {
		return nil, err
	}
	out := custom
	for _, e := range official {
		if usableIn(m, e.ID) {
			out = append(out, e)
		}
	}
	return out, nil
}

var bareNumberRe = regexp.MustCompile(`^[0-9]+$`)

// Validate checks a non-empty --theme value against mode m and returns the
// entry it names. Every refusal is a validation error (exit 2) that says
// what to pass instead; a catalog that cannot be fetched returns the fetch
// error, so it exits the way that failure does (4 for network / 5xx, 3 for
// auth) — an unverifiable id is never sent on the hope that it is right.
//
// Only the catalogs needed are fetched: an id found in the mode's own
// catalog costs one request; the other modes are consulted only to explain
// a miss.
func Validate(ctx context.Context, l Lister, m modes.Mode, id string) (Entry, error) {
	id = strings.TrimSpace(id)

	if bareNumberRe.MatchString(id) {
		// The asset id as the web shows it, without the prefix the stream
		// request needs. Sent as-is it is an unknown catalog id, which the
		// backend replaces — the failure this check was added for.
		if m.CustomAssetType != "" {
			return Entry{}, clerr.Validation(i18n.T("themecat.err.bare_number", id)).
				WithHint(i18n.T("themecat.hint.bare_number", id, m.Name))
		}
		return Entry{}, clerr.Validation(i18n.T("themecat.err.bare_number", id)).
			WithHint(i18n.T("themecat.hint.list", m.Name))
	}

	if strings.HasPrefix(id, "ua_") {
		return validateCustom(ctx, l, m, id)
	}
	return validateOfficial(ctx, l, m, id)
}

func validateCustom(ctx context.Context, l Lister, m modes.Mode, id string) (Entry, error) {
	if m.CustomAssetType != "" {
		entries, err := customEntries(ctx, l, m.CustomAssetType)
		if err != nil {
			return Entry{}, err
		}
		if e, ok := find(entries, id); ok {
			return e, nil
		}
	}
	// Not one of this mode's: say which mode it belongs to if it is the
	// other kind of custom asset.
	for _, other := range modes.All {
		if other.CustomAssetType == "" || other.CustomAssetType == m.CustomAssetType {
			continue
		}
		entries, err := customEntries(ctx, l, other.CustomAssetType)
		if err != nil {
			return Entry{}, err
		}
		if _, ok := find(entries, id); ok {
			return Entry{}, wrongMode(id, other, m)
		}
	}
	if m.CustomAssetType == "" {
		return Entry{}, clerr.Validation(i18n.T("themecat.err.no_custom", m.Name)).
			WithHint(i18n.T("themecat.hint.list", m.Name))
	}
	return Entry{}, clerr.Validation(i18n.T("themecat.err.custom_unknown", id)).
		WithHint(i18n.T("themecat.hint.list", m.Name))
}

func validateOfficial(ctx context.Context, l Lister, m modes.Mode, id string) (Entry, error) {
	entries, err := officialEntries(ctx, l, m.Suite)
	if err != nil {
		return Entry{}, err
	}
	if e, ok := find(entries, id); ok {
		if !usableIn(m, id) {
			return Entry{}, clerr.Validation(i18n.T("themecat.err.book_intro_only", id)).
				WithHint(i18n.T("themecat.hint.list", m.Name))
		}
		return e, nil
	}
	checked := map[string]bool{m.Suite: true}
	for _, other := range modes.All {
		if checked[other.Suite] {
			continue
		}
		checked[other.Suite] = true
		entries, err := officialEntries(ctx, l, other.Suite)
		if err != nil {
			return Entry{}, err
		}
		if _, ok := find(entries, id); ok {
			return Entry{}, wrongMode(id, other, m)
		}
	}
	return Entry{}, clerr.Validation(i18n.T("themecat.err.unknown", id, m.Name)).
		WithHint(i18n.T("themecat.hint.list", m.Name))
}

func wrongMode(id string, owner, asked modes.Mode) error {
	return clerr.Validation(i18n.T("themecat.err.wrong_mode", id, owner.Name, asked.Name)).
		WithHint(i18n.T("themecat.hint.wrong_mode", owner.Name, asked.Name))
}

func officialEntries(ctx context.Context, l Lister, suite string) ([]Entry, error) {
	themes, err := l.ListThemes(ctx, suite)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", i18n.T("themecat.err.lookup_failed"), err)
	}
	out := make([]Entry, 0, len(themes))
	for _, t := range themes {
		out = append(out, Entry{
			ID: t.ID, Name: t.Name, Desc: t.Desc, Tags: t.Tags, Badge: t.Badge,
			Source: SourceOfficial, Preview: t.Preview,
		})
	}
	return out, nil
}

// customEntries lists the active assets of one kind; inactive ones (deleted,
// still training, failed) are left out because the backend ignores them
// without an error. An empty kind lists nothing and costs no request.
func customEntries(ctx context.Context, l Lister, assetType string) ([]Entry, error) {
	if assetType == "" {
		return nil, nil
	}
	assets, err := l.ListThemeAssets(ctx, assetType)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", i18n.T("themecat.err.lookup_failed"), err)
	}
	var out []Entry
	for _, a := range assets {
		if a.Status != figlens.UserAssetStatusActive {
			continue
		}
		out = append(out, Entry{
			ID: a.Token(), Name: a.Name, Tags: []string{},
			Source: SourceCustom, Base: strings.TrimSpace(a.Config.BaseThemeID),
		})
	}
	return out, nil
}

func find(entries []Entry, id string) (Entry, bool) {
	for _, e := range entries {
		if e.ID == id {
			return e, true
		}
	}
	return Entry{}, false
}
