package modes

import (
	"testing"

	"github.com/vibeknow/cli/client/figlens"
)

func TestResolve(t *testing.T) {
	tests := []struct {
		in       string
		wantName string
		wantOK   bool
	}{
		{"", Freeform, true},
		{"default", Freeform, true},
		{"freeform", Freeform, true},
		{"FreeForm", Freeform, true},
		{"replica", Replica, true},
		{"image", Image, true},
		{" handdraw ", HandDraw, true},
		// Backend wire values are not CLI vocabulary.
		{"image2", "", false},
		{"hand-draw", "", false},
		{"script_lock", "", false},
		{"bogus", "", false},
	}
	for _, tt := range tests {
		m, ok := Resolve(tt.in)
		if ok != tt.wantOK || m.Name != tt.wantName {
			t.Errorf("Resolve(%q) = (%q, %v), want (%q, %v)", tt.in, m.Name, ok, tt.wantName, tt.wantOK)
		}
	}
}

// The custom-asset kinds are not interchangeable on the backend, so the
// table must give each mode the one it actually accepts.
func TestCustomAssetTypePerMode(t *testing.T) {
	want := map[string]string{
		Freeform: figlens.AssetTypeTheme,
		Replica:  figlens.AssetTypeTheme,
		Image:    figlens.AssetTypeImage2Theme,
		HandDraw: "",
	}
	for _, m := range All {
		if m.CustomAssetType != want[m.Name] {
			t.Errorf("%s: CustomAssetType = %q, want %q", m.Name, m.CustomAssetType, want[m.Name])
		}
	}
}

func TestForVideoKind(t *testing.T) {
	for _, m := range All {
		got, ok := ForVideoKind(m.VideoKind)
		if !ok || got.Name != m.Name {
			t.Errorf("ForVideoKind(%q) = (%q, %v), want %q", m.VideoKind, got.Name, ok, m.Name)
		}
	}
	if _, ok := ForVideoKind("stage_craft"); ok {
		t.Error("ForVideoKind must not claim a kind this CLI does not expose")
	}
}
