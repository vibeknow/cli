package figlens_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/vibeknow/cli/client/figlens"
)

// catalogServer answers the two subtitle catalog endpoints with the given
// payloads, wrapped in the backend's envelope.
func catalogServer(t *testing.T, path string, data any) *figlens.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != path {
			t.Errorf("unexpected request to %s", r.URL.Path)
			w.WriteHeader(404)
			return
		}
		if r.Method != "GET" {
			t.Errorf("method = %s, want GET — reading a catalog must not look like a write", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": data})
	}))
	t.Cleanup(srv.Close)
	return figlens.New(srv.URL, staticToken("tok"))
}

func TestListSubtitleFonts_KeepsCatalogOrder(t *testing.T) {
	// Order is not cosmetic: it is what the display index in `vk subtitle
	// fonts` numbers, and --subtitle-font accepts that index. Sorting the
	// list anywhere would silently point every numeric answer at a different
	// font.
	c := catalogServer(t, "/v1/works/subtitleFonts", map[string]any{"fonts": []map[string]any{
		{"family": "Noto Sans SC", "label": "黑体", "cdnUrl": "https://cdn.test/a.css"},
		{"family": "LXGW WenKai", "label": "霞鹜文楷", "cdnUrl": "https://cdn.test/b.css"},
	}})

	fonts, err := c.ListSubtitleFonts(context.Background())
	if err != nil {
		t.Fatalf("ListSubtitleFonts: %v", err)
	}
	if len(fonts) != 2 {
		t.Fatalf("got %d fonts, want 2: %+v", len(fonts), fonts)
	}
	if fonts[0].Family != "Noto Sans SC" || fonts[1].Family != "LXGW WenKai" {
		t.Fatalf("catalog order not preserved: %+v", fonts)
	}
	if fonts[0].Label != "黑体" {
		t.Errorf("label = %q, want 黑体", fonts[0].Label)
	}
}

// TestListSubtitlePresets_ZeroIsNotTheSameAsAbsent is the reason the patch
// type is built out of pointers.
//
// "白字·黑底" carries strokeWidth: 0 to switch off an outline the work may
// already have, and says nothing at all about fontSize. Decoded into plain
// value fields those two collapse into the same thing — both zero — and the
// merge can no longer tell "clear this" from "leave this alone". Whichever way
// it then guesses, one of the presets is broken: either the plate look keeps a
// stale outline, or every preset resets the subtitle size.
func TestListSubtitlePresets_ZeroIsNotTheSameAsAbsent(t *testing.T) {
	c := catalogServer(t, "/v1/works/subtitlePresets", map[string]any{"presets": []map[string]any{
		{"name": "白字·黑底", "patch": map[string]any{
			"color": "#ffffff", "backgroundColor": "rgba(8,8,12,0.68)", "strokeWidth": 0,
		}},
	}})

	presets, err := c.ListSubtitlePresets(context.Background())
	if err != nil {
		t.Fatalf("ListSubtitlePresets: %v", err)
	}
	if len(presets) != 1 {
		t.Fatalf("got %d presets, want 1", len(presets))
	}
	p := presets[0].Patch
	if p.StrokeWidth == nil {
		t.Fatal("strokeWidth:0 decoded as absent — the preset can no longer clear an outline")
	}
	if *p.StrokeWidth != 0 {
		t.Errorf("strokeWidth = %g, want 0", *p.StrokeWidth)
	}
	if p.FontSize != nil {
		t.Errorf("fontSize decoded as present (%d) when the preset never mentions it", *p.FontSize)
	}
}

// TestSubtitleStylePatch_ApplyLeavesTheVideoAlone covers what a preset must
// not touch. Size, vertical position and entry animation belong to the video,
// not to the look; a preset that reset them would make "try a different
// subtitle style" quietly undo the tuning around it.
func TestSubtitleStylePatch_ApplyLeavesTheVideoAlone(t *testing.T) {
	current := figlens.SubtitleStyle{
		FontFamily: "Source Han Sans", FontSize: 48, FontWeight: 700,
		Color: "#000000", StrokeColor: "#FFFFFF", StrokeWidth: 2,
		BottomPercent: 0.2, Animation: "fadeup",
	}

	white, plate := "#ffffff", "rgba(8,8,12,0.68)"
	var zero float64
	got := figlens.SubtitleStylePatch{
		Color: &white, BackgroundColor: &plate, StrokeWidth: &zero,
	}.Apply(current)

	if got.Color != white || got.BackgroundColor != plate {
		t.Errorf("preset did not apply: %+v", got)
	}
	if got.StrokeWidth != 0 {
		t.Errorf("strokeWidth = %g, want 0 — a plate look has to clear the outline", got.StrokeWidth)
	}
	for _, tc := range []struct {
		name      string
		got, want any
	}{
		{"fontSize", got.FontSize, 48},
		{"bottomPercent", got.BottomPercent, 0.2},
		{"animation", got.Animation, "fadeup"},
		{"fontFamily", got.FontFamily, "Source Han Sans"},
		{"fontWeight", got.FontWeight, 700},
		// strokeColor survives even though the outline is now zero-width:
		// the preset did not mention it, and the width alone decides
		// whether it is drawn. Clearing it here would lose the setting for
		// anyone who later widens the outline again.
		{"strokeColor", got.StrokeColor, "#FFFFFF"},
	} {
		if tc.got != tc.want {
			t.Errorf("%s = %v, want %v — the preset disturbed a field it never named", tc.name, tc.got, tc.want)
		}
	}

	// Apply must not mutate its input: the command applies a preset and then
	// layers explicit flags on top, and a caller re-reading the original
	// would otherwise see it already changed.
	if current.Color != "#000000" || current.StrokeWidth != 2 {
		t.Errorf("Apply mutated its receiver's argument: %+v", current)
	}
}

func TestNormalizeSubtitleAnimation(t *testing.T) {
	// The backend compares against its whitelist exactly, so anything this
	// accepts it must also lower-case, or the value goes out in a spelling
	// the server refuses.
	for _, tc := range []struct {
		in     string
		want   string
		wantOK bool
	}{
		{"fade", "fade", true},
		{"Fade", "fade", true},
		{"  KARAOKE ", "karaoke", true},
		{"none", "none", true},
		{"zoom", "zoom", false},
		{"", "", false},
	} {
		got, ok := figlens.NormalizeSubtitleAnimation(tc.in)
		if got != tc.want || ok != tc.wantOK {
			t.Errorf("NormalizeSubtitleAnimation(%q) = (%q, %v), want (%q, %v)",
				tc.in, got, ok, tc.want, tc.wantOK)
		}
	}
}
