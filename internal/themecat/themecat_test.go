package themecat

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/vibeknow/cli/client/figlens"
	"github.com/vibeknow/cli/internal/clerr"
	"github.com/vibeknow/cli/internal/modes"
)

type fakeLister struct {
	suites   map[string][]figlens.Theme
	assets   map[string][]figlens.ThemeAsset
	suiteErr error
	calls    []string
}

func (f *fakeLister) ListThemes(_ context.Context, suite string) ([]figlens.Theme, error) {
	f.calls = append(f.calls, "suite:"+suite)
	if f.suiteErr != nil {
		return nil, f.suiteErr
	}
	return f.suites[suite], nil
}

func (f *fakeLister) ListThemeAssets(_ context.Context, typ string) ([]figlens.ThemeAsset, error) {
	f.calls = append(f.calls, "assets:"+typ)
	return f.assets[typ], nil
}

func asset(id int64, name string, status int, base string) figlens.ThemeAsset {
	a := figlens.ThemeAsset{ID: id, Name: name, Status: status}
	a.Config.BaseThemeID = base
	return a
}

func image2Asset(id int64, name, category string) figlens.ThemeAsset {
	a := figlens.ThemeAsset{ID: id, Name: name, Status: figlens.UserAssetStatusActive}
	a.Config.Category = category
	return a
}

func newFake() *fakeLister {
	return &fakeLister{
		suites: map[string][]figlens.Theme{
			figlens.ThemeSuiteDesign: {
				{ID: "mint", Name: "纯白薄荷"},
				{ID: "ember-bronze", Name: "深炭古铜"},
			},
			figlens.ThemeSuiteImage2: {{ID: "ink", Name: "Ink", Scenes: []figlens.ThemeScene{
				{Slug: "office", Name: "Office"}, {Slug: "street", Name: "Street"},
			}}},
			figlens.ThemeSuiteHandDraw: {{ID: "crayon", Name: "Crayon"}},
			figlens.ThemeSuiteStageCraft: {{ID: "riso", Name: "Riso", Palettes: []figlens.ThemePalette{
				{ID: "riso-aqua", Name: "Aqua"},
			}}},
		},
		assets: map[string][]figlens.ThemeAsset{
			figlens.AssetTypeTheme: {
				asset(1234567, "红薄荷", figlens.UserAssetStatusActive, "mint"),
				asset(9, "deleted one", 2, "mint"),
				asset(11, "古铜改", figlens.UserAssetStatusActive, "ember-bronze"),
			},
			figlens.AssetTypeImage2Theme: {
				image2Asset(77, "my style", figlens.Image2CategoryStyle),
				image2Asset(78, "my character", figlens.Image2CategoryIP),
			},
		},
	}
}

func mode(t *testing.T, name string) modes.Mode {
	t.Helper()
	m, ok := modes.Resolve(name)
	if !ok {
		t.Fatalf("no mode %q", name)
	}
	return m
}

func TestList_FreeformHasCustomFirstAndDropsBookIntro(t *testing.T) {
	got, err := List(context.Background(), newFake(), mode(t, "freeform"))
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, e := range got {
		ids = append(ids, e.ID)
	}
	if strings.Join(ids, ",") != "ua_1234567,mint" {
		t.Fatalf("ids = %v; want the active custom theme first, the inactive one and the book-intro theme absent", ids)
	}
	if got[0].Source != SourceCustom || got[0].Base != "mint" {
		t.Errorf("custom entry = %+v", got[0])
	}
}

func TestList_ReplicaKeepsBookIntroThemes(t *testing.T) {
	got, err := List(context.Background(), newFake(), mode(t, "replica"))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range got {
		found = found || e.ID == "ember-bronze"
	}
	if !found {
		t.Error("replica renders work.theme as given; the book-intro filter is a freeform rule")
	}
}

func TestList_HandDrawSkipsAssetRequest(t *testing.T) {
	f := newFake()
	if _, err := List(context.Background(), f, mode(t, "handdraw")); err != nil {
		t.Fatal(err)
	}
	for _, c := range f.calls {
		if strings.HasPrefix(c, "assets:") {
			t.Errorf("handdraw has no custom kind but asked %s", c)
		}
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		desc    string
		mode    string
		id      string
		wantID  string
		wantErr []string // substrings of message+hint; nil = success
	}{
		{desc: "official id", mode: "freeform", id: "mint", wantID: "mint"},
		{desc: "own custom theme", mode: "freeform", id: "ua_1234567", wantID: "ua_1234567"},
		{desc: "custom theme in replica", mode: "replica", id: "ua_1234567", wantID: "ua_1234567"},
		{desc: "image2 style in image", mode: "image", id: "ua_77", wantID: "ua_77"},

		// The asset id as the web shows it, without the ua_ prefix.
		{desc: "bare number", mode: "freeform", id: "1234567", wantErr: []string{"ua_1234567"}},
		{desc: "inactive custom", mode: "freeform", id: "ua_9", wantErr: []string{"ua_9", "theme list --mode freeform"}},
		{desc: "unknown custom", mode: "freeform", id: "ua_1", wantErr: []string{"ua_1"}},
		{desc: "image2 style outside image", mode: "freeform", id: "ua_77", wantErr: []string{"--mode image"}},
		{desc: "freeform theme in image", mode: "image", id: "ua_1234567", wantErr: []string{"--mode freeform"}},
		{desc: "custom in handdraw", mode: "handdraw", id: "ua_1234567", wantErr: []string{"--mode freeform"}},
		{desc: "other mode's official", mode: "freeform", id: "ink", wantErr: []string{"--mode image"}},
		{desc: "book-intro theme in freeform", mode: "freeform", id: "ember-bronze", wantErr: []string{"ember-bronze", "does not offer"}},
		{desc: "book-intro theme in replica", mode: "replica", id: "ember-bronze", wantID: "ember-bronze"},
		// Custom themes resolve through their base, so they inherit its fate.
		{desc: "custom on a book-intro base in freeform", mode: "freeform", id: "ua_11", wantErr: []string{"ua_11", "ember-bronze"}},
		{desc: "custom on a book-intro base in replica", mode: "replica", id: "ua_11", wantID: "ua_11"},
		{desc: "unknown slug", mode: "freeform", id: "nope", wantErr: []string{"nope", "theme list --mode freeform"}},

		// Canvas: a direction and a palette under it are both theme ids.
		{desc: "canvas direction", mode: "canvas", id: "riso", wantID: "riso"},
		{desc: "canvas palette", mode: "canvas", id: "riso-aqua", wantID: "riso-aqua"},
		{desc: "canvas palette outside canvas", mode: "freeform", id: "riso-aqua", wantErr: []string{"--mode canvas"}},

		// The image2 asset type also holds IP characters, which are not styles.
		{desc: "IP character as theme", mode: "image", id: "ua_78", wantErr: []string{"--ip-asset ua_78"}},
	}
	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			e, err := Validate(context.Background(), newFake(), mode(t, tt.mode), tt.id)
			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if e.ID != tt.wantID {
					t.Fatalf("entry id = %q, want %q", e.ID, tt.wantID)
				}
				return
			}
			var ce *clerr.Error
			if !errors.As(err, &ce) || ce.Code != clerr.ExitValidation {
				t.Fatalf("err = %v, want a validation error (exit 2)", err)
			}
			text := ce.Message + " | " + ce.Hint
			for _, want := range tt.wantErr {
				if !strings.Contains(text, want) {
					t.Errorf("error %q does not mention %q", text, want)
				}
			}
		})
	}
}

// A catalog that cannot be fetched must not turn into a pass, and must not
// be dressed up as a validation error either: it exits the way the fetch
// failure does.
func TestValidate_FetchFailureIsNotAPassOrAnExit2(t *testing.T) {
	f := newFake()
	cause := errors.New("connection refused")
	f.suiteErr = cause
	_, err := Validate(context.Background(), f, mode(t, "freeform"), "mint")
	if err == nil {
		t.Fatal("a theme that could not be checked was let through")
	}
	if !errors.Is(err, cause) {
		t.Errorf("fetch error not preserved: %v", err)
	}
	var ce *clerr.Error
	if errors.As(err, &ce) && ce.Code == clerr.ExitValidation {
		t.Error("a fetch failure is not the caller's argument mistake")
	}
}

// An id in the mode's own catalog costs one request.
func TestValidate_HitCostsOneRequest(t *testing.T) {
	f := newFake()
	if _, err := Validate(context.Background(), f, mode(t, "freeform"), "mint"); err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != 1 {
		t.Errorf("calls = %v, want exactly one", f.calls)
	}
}

func TestList_ImageDropsIPCharacters(t *testing.T) {
	got, err := List(context.Background(), newFake(), mode(t, "image"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range got {
		if e.ID == "ua_78" {
			t.Error("an IP character was listed as a theme")
		}
	}
}

func TestList_CanvasCarriesVariants(t *testing.T) {
	got, err := List(context.Background(), newFake(), mode(t, "canvas"))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || len(got[0].Variants) != 1 || got[0].Variants[0].ID != "riso-aqua" {
		t.Fatalf("canvas entries = %+v", got)
	}
}

func TestValidateScene(t *testing.T) {
	e, err := Validate(context.Background(), newFake(), mode(t, "image"), "ink")
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateScene(e, "office"); err != nil {
		t.Errorf("known scene refused: %v", err)
	}
	err = ValidateScene(e, "beach")
	var ce *clerr.Error
	if !errors.As(err, &ce) || ce.Code != clerr.ExitValidation || !strings.Contains(ce.Hint, "office, street") {
		t.Errorf("unknown scene: err = %v", err)
	}
	custom, _ := Validate(context.Background(), newFake(), mode(t, "image"), "ua_77")
	if err := ValidateScene(custom, "office"); err == nil {
		t.Error("a style with no scenes accepted a scene")
	}
}
