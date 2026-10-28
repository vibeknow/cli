// Package modes is the one table of creation modes the CLI exposes.
//
// `vk create --mode` and `vk theme list --mode` used to keep their own
// switch statements, and they drifted: `theme list --mode default` worked
// while `create --mode default` was a validation error, so an agent that
// copied the spelling from one command into the other failed. Both now
// resolve through Resolve, so a name valid for one is valid for the other.
//
// A mode fixes three things at once: the backend video_kind, the theme
// catalog `--theme` draws from, and which kind of user asset can stand in
// for a catalog theme. Keeping them in one row is what lets `theme list`
// and `create --theme` agree on what a theme id means.
package modes

import (
	"strings"

	"github.com/vibeknow/cli/client/figlens"
)

// Mode is one row of the table.
type Mode struct {
	// Name is the canonical CLI spelling, the one echoed in output.
	Name string
	// VideoKind is the backend wire value; empty is the standard line.
	VideoKind string
	// Suite is the GET /v1/themes?type= catalog `--theme` draws from.
	Suite string
	// CustomAssetType is the GET /v1/assets?type= kind whose "ua_<id>"
	// tokens this mode accepts as a theme; empty means none. The two kinds
	// that exist share the ua_ prefix but are not interchangeable — the
	// backend rejects an image2_theme token outside image mode.
	CustomAssetType string
}

const (
	Freeform = "freeform"
	Replica  = "replica"
	Image    = "image"
	HandDraw = "handdraw"
)

// All lists the modes in the order help text and errors name them.
var All = []Mode{
	{Name: Freeform, VideoKind: "", Suite: figlens.ThemeSuiteDesign, CustomAssetType: figlens.AssetTypeTheme},
	// The replica graph has no theme_select node, but video_package still
	// renders its chrome from work.theme, custom tokens included.
	{Name: Replica, VideoKind: figlens.VideoKindReplica, Suite: figlens.ThemeSuiteDesign, CustomAssetType: figlens.AssetTypeTheme},
	{Name: Image, VideoKind: figlens.VideoKindImage2, Suite: figlens.ThemeSuiteImage2, CustomAssetType: figlens.AssetTypeImage2Theme},
	{Name: HandDraw, VideoKind: figlens.VideoKindHandDraw, Suite: figlens.ThemeSuiteHandDraw},
}

// aliases are accepted spellings that are not the canonical name. Empty is
// the default; "default" is what `theme list` documented before freeform
// had a name of its own.
var aliases = map[string]string{
	"":        Freeform,
	"default": Freeform,
}

// Resolve maps a --mode value onto its row, case-insensitively. Backend
// wire values ("image2", "hand-draw") are deliberately not accepted: they
// are not CLI vocabulary.
func Resolve(s string) (Mode, bool) {
	name := strings.ToLower(strings.TrimSpace(s))
	if canon, ok := aliases[name]; ok {
		name = canon
	}
	for _, m := range All {
		if m.Name == name {
			return m, true
		}
	}
	return Mode{}, false
}

// ForVideoKind finds the row for a backend video_kind, for reporting what
// a work was made with. Unknown kinds (modes this CLI does not expose)
// return false.
func ForVideoKind(kind string) (Mode, bool) {
	kind = strings.TrimSpace(kind)
	for _, m := range All {
		if m.VideoKind == kind {
			return m, true
		}
	}
	return Mode{}, false
}

// Names returns the canonical names, comma-separated, for error messages.
func Names() string {
	names := make([]string, len(All))
	for i, m := range All {
		names[i] = m.Name
	}
	return strings.Join(names, ", ")
}
