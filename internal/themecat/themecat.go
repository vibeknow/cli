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
	"errors"
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
	// Variants are the second level under a canvas art direction: palettes,
	// each a valid --theme value of its own that also pins the colours.
	Variants []Variant
	// Scenes are the scene cards of an image style, for --theme-scene.
	Scenes []figlens.ThemeScene
	// Parent is set when this entry is a variant: the direction it is under.
	Parent string

	// ipCharacter marks an image2_theme asset that is an IP character, not
	// a style. Kept only so a refusal can point at --ip-asset; never listed.
	ipCharacter bool
}

// Variant is one palette under a canvas art direction.
type Variant struct {
	ID   string
	Name string
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

// usableIn reports whether an entry survives the backend's per-kind filter.
// A custom theme is resolved through its base, so it fails when its base
// does. Replica is exempt: it has no theme_select node, and its packaging
// step renders work.theme as given.
func usableIn(m modes.Mode, e Entry) bool {
	id := e.ID
	if e.Source == SourceCustom {
		id = e.Base
	}
	return m.Name != modes.Freeform || !bookIntroOnly[id]
}

// List returns what `--theme` accepts in mode m: the caller's own active
// theme assets first (the web lists 我的 first too), then the official
// suite with the entries the backend would replace filtered out.
// Mine is one of the user's own themes with every mode that accepts it.
type Mine struct {
	Entry
	Modes []string
}

// ListMine lists the user's own themes across all modes, each with the modes
// it can be used in. A user names a theme ("my 国风 template"), not the
// mode it was made for, and custom themes live in more than one catalog;
// without this the caller lists mode after mode looking for a name.
// Each mode is read through List, so a theme appears under a mode exactly
// when create would accept it there.
func ListMine(ctx context.Context, l Lister) ([]Mine, error) {
	var out []Mine
	at := map[string]int{}
	for _, m := range modes.All {
		if m.CustomAssetType == "" {
			continue
		}
		entries, err := List(ctx, l, m)
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			if e.Source != SourceCustom {
				continue
			}
			if i, ok := at[e.ID]; ok {
				out[i].Modes = append(out[i].Modes, m.Name)
				continue
			}
			at[e.ID] = len(out)
			out = append(out, Mine{Entry: e, Modes: []string{m.Name}})
		}
	}
	return out, nil
}

func List(ctx context.Context, l Lister, m modes.Mode) ([]Entry, error) {
	custom, err := customEntries(ctx, l, m.CustomAssetType)
	if err != nil {
		return nil, err
	}
	official, err := officialEntries(ctx, l, m.Suite)
	if err != nil {
		return nil, err
	}
	var out []Entry
	for _, e := range custom {
		if !e.ipCharacter && usableIn(m, e) && baseKnown(e, official) {
			out = append(out, e)
		}
	}
	for _, e := range official {
		if usableIn(m, e) {
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
		//
		// Still refused rather than corrected: the caller has to send the
		// value it will see reported back. But the user's own catalog is at
		// hand, so the answer is definite — "that is your theme X, spelled
		// ua_<id>" or the reason ua_<id> would not work either — instead of
		// a guess the caller has to try.
		e, err := validateCustom(ctx, l, m, "ua_"+id)
		if err == nil {
			return Entry{}, clerr.Validation(i18n.T("themecat.err.bare_number", id)).
				WithHint(i18n.T("themecat.hint.bare_number_found", e.Name, e.ID))
		}
		var cl *clerr.Error
		if !errors.As(err, &cl) || cl.Type != clerr.TypeValidation {
			return Entry{}, err // the lookup itself failed
		}
		return Entry{}, clerr.Validation(i18n.T("themecat.err.bare_number_because", i18n.T("themecat.err.bare_number", id), cl.Message)).
			WithHint(cl.Hint)
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
			if e.ipCharacter {
				return Entry{}, clerr.Validation(i18n.T("themecat.err.ip_not_theme", id)).
					WithHint(i18n.T("themecat.hint.ip_not_theme", id))
			}
			if !usableIn(m, e) {
				return Entry{}, clerr.Validation(i18n.T("themecat.err.book_intro_base", id, e.Base)).
					WithHint(i18n.T("themecat.hint.list", m.Name))
			}
			if e.Base != "" {
				official, err := officialEntries(ctx, l, m.Suite)
				if err != nil {
					return Entry{}, err
				}
				if !baseKnown(e, official) {
					return Entry{}, clerr.Validation(i18n.T("themecat.err.base_missing", id, e.Base)).
						WithHint(i18n.T("themecat.hint.list", m.Name))
				}
			}
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
		if e, ok := find(entries, id); ok && !e.ipCharacter {
			return Entry{}, wrongMode(id, other, m)
		}
	}
	if m.CustomAssetType == "" {
		return Entry{}, clerr.Validation(i18n.T("themecat.err.no_custom", m.Name)).
			WithHint(i18n.T("themecat.hint.list", m.Name))
	}
	// A ua_ id the user does not have: the useful next step is their own
	// list, across modes, not the whole catalog of this one.
	return Entry{}, clerr.Validation(i18n.T("themecat.err.custom_unknown", id)).
		WithHint(i18n.T("themecat.hint.mine"))
}

func validateOfficial(ctx context.Context, l Lister, m modes.Mode, id string) (Entry, error) {
	entries, err := officialEntries(ctx, l, m.Suite)
	if err != nil {
		return Entry{}, err
	}
	if e, ok := find(entries, id); ok {
		if !usableIn(m, e) {
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
		e := Entry{
			ID: t.ID, Name: t.Name, Desc: t.Desc, Tags: t.Tags, Badge: t.Badge,
			Source: SourceOfficial, Preview: t.Preview, Scenes: t.Scenes,
		}
		for _, p := range t.Palettes {
			e.Variants = append(e.Variants, Variant{ID: p.ID, Name: p.Name})
		}
		out = append(out, e)
	}
	return out, nil
}

// customEntries lists the active assets of one kind; inactive ones (deleted,
// still training, failed) are left out because the backend ignores them
// without an error. The image2 kind also holds IP characters, which the
// backend refuses as a theme; they come back marked so List can drop them
// and Validate can say where they go instead. An empty kind lists nothing
// and costs no request.
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
		e := Entry{
			ID: a.Token(), Name: a.Name, Tags: []string{},
			Source: SourceCustom, Base: strings.TrimSpace(a.Config.BaseThemeID),
		}
		if assetType == figlens.AssetTypeImage2Theme && a.Config.Category != figlens.Image2CategoryStyle {
			e.ipCharacter = true
		}
		out = append(out, e)
	}
	return out, nil
}

// find looks an id up among the entries and the variants under them. A
// variant comes back as an entry of its own, naming its parent.
// baseKnown reports whether a custom theme's base still exists in the
// official catalog. The backend resolves a custom theme through its base and,
// when the base is gone, drops the custom theme altogether and treats the
// ua_ token as an unknown theme — the fallback substitution again. An empty
// base is fine: the backend uses its default theme.
func baseKnown(e Entry, official []Entry) bool {
	if e.Source != SourceCustom || e.Base == "" {
		return true
	}
	_, ok := find(official, e.Base)
	return ok
}

func find(entries []Entry, id string) (Entry, bool) {
	for _, e := range entries {
		if e.ID == id {
			return e, true
		}
		for _, v := range e.Variants {
			if v.ID == id {
				return Entry{
					ID: v.ID, Name: e.Name + " · " + v.Name, Tags: []string{},
					Source: SourceOfficial, Parent: e.ID,
				}, true
			}
		}
	}
	return Entry{}, false
}

// ValidateScene checks a --theme-scene slug against the scenes of the theme
// it was given with. The backend falls back to the style's own anchor image
// for a scene it cannot find, so an unchecked slug is another silent
// substitution.
func ValidateScene(e Entry, slug string) error {
	slug = strings.TrimSpace(slug)
	for _, s := range e.Scenes {
		if s.Slug == slug {
			return nil
		}
	}
	if len(e.Scenes) == 0 {
		return clerr.Validation(i18n.T("themecat.err.no_scenes", e.ID))
	}
	slugs := make([]string, len(e.Scenes))
	for i, s := range e.Scenes {
		slugs[i] = s.Slug
	}
	return clerr.Validation(i18n.T("themecat.err.scene_unknown", slug, e.ID)).
		WithHint(i18n.T("themecat.hint.scenes", strings.Join(slugs, ", ")))
}
