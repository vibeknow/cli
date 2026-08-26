package integration

import (
	"encoding/json"
	"strings"
	"testing"
)

// sentStyle pulls the subtitle style out of the recorded write, failing the
// test if no write was made.
func sentStyle(t *testing.T, seen func(string) map[string]any) map[string]any {
	t.Helper()
	sent := seen("/v1/works/subtitleStyle")
	if sent == nil {
		t.Fatal("no subtitleStyle write was made")
	}
	style, _ := sent["subtitleStyle"].(map[string]any)
	if style == nil {
		t.Fatalf("no subtitleStyle payload: %+v", sent)
	}
	return style
}

// TestVideoSet_PresetClearsWhatItReplaces is the whole reason presets exist as
// a command-line concept rather than a hint in the docs.
//
// The work starts with a 2px outline and no background plate. "白字·黑底" puts
// the text on a plate, and its patch carries strokeWidth: 0 precisely so the
// old outline goes with it. Apply only the fields the preset "sets" in the
// loose sense — the non-zero ones — and the result is a subtitle wearing both
// a plate and an outline, which is exactly the muddy look the preset exists to
// avoid. Nothing on the wire would report that; it only shows up in the video.
func TestVideoSet_PresetClearsWhatItReplaces(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	srv, seen, _ := settingsServer(t)
	defer srv.Close()

	bin := build(t)
	configHome := buildVideoProfile(t, srv.URL)

	stdout, stderr, code := runVideoCmd(t, bin, configHome,
		"video", "set", "42", "--session-id", "s_run", "--subtitle-preset", "1", "--output", "json")
	if code != 0 {
		t.Fatalf("set exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}

	style := sentStyle(t, seen)

	// The fixture's colour is upper-case and the preset's is lower-case, so
	// this alone proves the preset was applied rather than the work's own
	// style being echoed back.
	if style["color"] != "#ffffff" {
		t.Errorf("color = %v, want #ffffff — the preset was not applied", style["color"])
	}
	if style["backgroundColor"] != "rgba(8,8,12,0.68)" {
		t.Errorf("backgroundColor = %v, want the preset's plate", style["backgroundColor"])
	}
	// strokeWidth 0 is sent as an absent key: the backend's own payload type
	// omits it too, and its renderer treats missing and zero identically. So
	// absent is correct, and any *non-zero* value here is the stale outline
	// the preset was supposed to remove.
	if w, ok := style["strokeWidth"]; ok && w != float64(0) {
		t.Errorf("strokeWidth = %v, want 0 or absent — the preset failed to clear the old 2px outline", w)
	}

	// And the fields the preset never names stay exactly as the work had
	// them. Size and animation belong to the video, not to the look.
	for key, want := range map[string]any{
		"fontSize":   float64(36),
		"fontFamily": "Source Han Sans",
		"fontWeight": float64(700),
		"animation":  "fade",
	} {
		if style[key] != want {
			t.Errorf("%s = %v, want %v — applying a look reset something it never mentioned: %+v",
				key, style[key], want, style)
		}
	}
}

// TestVideoSet_PresetByNameSetsTheOutlinedLook covers the other direction: an
// outline is only legible once the plate behind it is gone, so the preset has
// to carry both halves and both have to arrive.
func TestVideoSet_PresetByNameSetsTheOutlinedLook(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	srv, seen, _ := settingsServer(t)
	defer srv.Close()

	bin := build(t)
	configHome := buildVideoProfile(t, srv.URL)

	stdout, stderr, code := runVideoCmd(t, bin, configHome,
		"video", "set", "42", "--session-id", "s_run", "--subtitle-preset", "白字·黑边", "--output", "json")
	if code != 0 {
		t.Fatalf("set exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}

	style := sentStyle(t, seen)
	for key, want := range map[string]any{
		"backgroundColor": "transparent",
		"strokeWidth":     float64(3),
		"strokeColor":     "rgba(0,0,0,0.92)",
		"fontFamily":      "Noto Sans SC",
		"fontWeight":      float64(600),
	} {
		if style[key] != want {
			t.Errorf("%s = %v, want %v: %+v", key, style[key], want, style)
		}
	}

	var got map[string]any
	_ = json.Unmarshal([]byte(stdout), &got)
	applied, _ := got["applied"].(map[string]any)
	if applied["subtitle_preset"] != "白字·黑边" {
		t.Errorf("applied.subtitle_preset = %v, want the preset's name: %+v", applied["subtitle_preset"], applied)
	}
}

// TestVideoSet_ExplicitFlagsWinOverThePreset pins the precedence. "That look,
// but bigger" is a thing people ask for; "bigger, but whatever the look says
// about size" is not, so the flag has to be applied after the patch.
func TestVideoSet_ExplicitFlagsWinOverThePreset(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	srv, seen, _ := settingsServer(t)
	defer srv.Close()

	bin := build(t)
	configHome := buildVideoProfile(t, srv.URL)

	stdout, stderr, code := runVideoCmd(t, bin, configHome,
		"video", "set", "42", "--session-id", "s_run",
		"--subtitle-preset", "2", "--subtitle-color", "#ffd84d", "--subtitle-size", "60",
		"--output", "json")
	if code != 0 {
		t.Fatalf("set exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}

	style := sentStyle(t, seen)
	if style["color"] != "#ffd84d" {
		t.Errorf("color = %v, want #ffd84d — the preset overwrote an explicit flag", style["color"])
	}
	if style["fontSize"] != float64(60) {
		t.Errorf("fontSize = %v, want 60", style["fontSize"])
	}
	// The rest of the look still comes from the preset.
	if style["strokeWidth"] != float64(3) || style["backgroundColor"] != "transparent" {
		t.Errorf("preset's outline did not survive alongside the explicit flags: %+v", style)
	}
}

// TestVideoSet_FontIndexResolvesToTheExactFamily covers the shorthand. The
// families are exact strings the backend compares byte for byte, so the index
// has to become the catalog's own spelling rather than anything the caller
// typed.
func TestVideoSet_FontIndexResolvesToTheExactFamily(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	srv, seen, _ := settingsServer(t)
	defer srv.Close()

	bin := build(t)
	configHome := buildVideoProfile(t, srv.URL)

	stdout, stderr, code := runVideoCmd(t, bin, configHome,
		"video", "set", "42", "--session-id", "s_run", "--subtitle-font", "2", "--output", "json")
	if code != 0 {
		t.Fatalf("set exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if style := sentStyle(t, seen); style["fontFamily"] != "LXGW WenKai" {
		t.Errorf("fontFamily = %v, want LXGW WenKai (catalog #2)", style["fontFamily"])
	}

	// Case-folded names resolve to the catalog's spelling too, since an
	// almost-right family is refused on the wire with no explanation.
	srv2, seen2, _ := settingsServer(t)
	defer srv2.Close()
	configHome2 := buildVideoProfile(t, srv2.URL)
	stdout, stderr, code = runVideoCmd(t, bin, configHome2,
		"video", "set", "42", "--session-id", "s_run", "--subtitle-font", "lxgw wenkai", "--output", "json")
	if code != 0 {
		t.Fatalf("set exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if style := sentStyle(t, seen2); style["fontFamily"] != "LXGW WenKai" {
		t.Errorf("fontFamily = %v, want the catalog's spelling, not the caller's", style["fontFamily"])
	}
}

// TestVideoSet_RefusesValuesTheCatalogDoesNotHave keeps a wrong guess from
// reaching the backend, whose own refusal ("fontFamily not allowed") names
// neither what is allowed nor where to look.
func TestVideoSet_RefusesValuesTheCatalogDoesNotHave(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	bin := build(t)

	cases := []struct {
		name string
		args []string
		want []string
	}{
		{"unknown font", []string{"--subtitle-font", "Comic Sans"}, []string{"Comic Sans", "vk subtitle fonts", "3"}},
		{"font index past the end", []string{"--subtitle-font", "9"}, []string{"#9", "1–3"}},
		{"unknown preset", []string{"--subtitle-preset", "霓虹"}, []string{"霓虹", "白字·黑底"}},
		{"preset index past the end", []string{"--subtitle-preset", "9"}, []string{"#9", "1–2"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, _, writes := settingsServer(t)
			defer srv.Close()
			configHome := buildVideoProfile(t, srv.URL)

			args := append([]string{"video", "set", "42", "--session-id", "s_run"}, tc.args...)
			stdout, stderr, code := runVideoCmd(t, bin, configHome, append(args, "--output", "json")...)
			if code != 2 {
				t.Fatalf("exit %d, want 2\nstdout: %s\nstderr: %s", code, stdout, stderr)
			}
			for _, want := range tc.want {
				if !strings.Contains(stderr, want) {
					t.Errorf("stderr does not mention %q — the caller cannot tell what to pass instead\n%s", want, stderr)
				}
			}
			if n := writes(); n != 0 {
				t.Errorf("%d write(s) made on a refused command; nothing should have changed", n)
			}
		})
	}
}

// TestVideoSet_RefusesImpossibleNumbersLocally is about a silent substitution,
// not a rejection.
//
// The backend clamps position and outline width instead of refusing them, so
// asking for a subtitle at 1.5× the frame height succeeds while storing 0.98.
// Nothing reports the swap, which makes it the worst kind of bug to chase:
// the command said it worked and the video says otherwise. Refusing locally
// costs one exit code and states the range.
func TestVideoSet_RefusesImpossibleNumbersLocally(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	bin := build(t)

	cases := []struct {
		name string
		args []string
		want []string
	}{
		{"bottom above the frame", []string{"--subtitle-bottom", "1.5"}, []string{"--subtitle-bottom", "0.02", "0.98"}},
		{"bottom below the floor", []string{"--subtitle-bottom", "0.001"}, []string{"0.02"}},
		{"outline too thick", []string{"--subtitle-stroke-width", "20"}, []string{"--subtitle-stroke-width", "12"}},
		{"outline negative", []string{"--subtitle-stroke-width", "-1"}, []string{"--subtitle-stroke-width"}},
		{"weight off the css scale", []string{"--subtitle-font-weight", "50"}, []string{"100", "900"}},
		{"unknown animation", []string{"--subtitle-animation", "zoom"}, []string{"zoom", "karaoke", "fadeup"}},
		{"size not positive", []string{"--subtitle-size", "0"}, []string{"--subtitle-size"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, _, writes := settingsServer(t)
			defer srv.Close()
			configHome := buildVideoProfile(t, srv.URL)

			args := append([]string{"video", "set", "42", "--session-id", "s_run"}, tc.args...)
			stdout, stderr, code := runVideoCmd(t, bin, configHome, append(args, "--output", "json")...)
			if code != 2 {
				t.Fatalf("exit %d, want 2\nstdout: %s\nstderr: %s", code, stdout, stderr)
			}
			for _, want := range tc.want {
				if !strings.Contains(stderr, want) {
					t.Errorf("stderr does not mention %q\n%s", want, stderr)
				}
			}
			if n := writes(); n != 0 {
				t.Errorf("%d write(s) made on a refused command", n)
			}
		})
	}
}

// TestVideoSet_EveryStyleFlagReachesTheWire is the unglamorous one: each new
// flag has to actually land in the payload under the right key.
//
// A flag that is accepted, validated and then never applied is the worst
// possible outcome — the command exits 0, reports success, discards the
// rendered MP4, and changes nothing. There is no error anywhere to notice.
func TestVideoSet_EveryStyleFlagReachesTheWire(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	srv, seen, _ := settingsServer(t)
	defer srv.Close()

	bin := build(t)
	configHome := buildVideoProfile(t, srv.URL)

	stdout, stderr, code := runVideoCmd(t, bin, configHome,
		"video", "set", "42", "--session-id", "s_run",
		"--subtitle-size", "52",
		"--subtitle-color", "#ffd84d",
		"--subtitle-font", "3",
		"--subtitle-font-weight", "600",
		"--subtitle-bg-color", "transparent",
		"--subtitle-bottom", "0.15",
		"--subtitle-stroke-color", "rgba(0,0,0,0.92)",
		"--subtitle-stroke-width", "2.5",
		"--subtitle-animation", "karaoke",
		"--output", "json")
	if code != 0 {
		t.Fatalf("set exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}

	style := sentStyle(t, seen)
	for key, want := range map[string]any{
		"fontSize":        float64(52),
		"color":           "#ffd84d",
		"fontFamily":      "Smiley Sans",
		"fontWeight":      float64(600),
		"backgroundColor": "transparent",
		"bottomPercent":   0.15,
		"strokeColor":     "rgba(0,0,0,0.92)",
		"strokeWidth":     2.5,
		"animation":       "karaoke",
	} {
		if style[key] != want {
			t.Errorf("%s = %v (%T), want %v — the flag was accepted but never applied: %+v",
				key, style[key], style[key], want, style)
		}
	}
}

// TestVideoSet_AnimationReachesTheWireLowerCased covers a mismatch that would
// otherwise only show up as a backend refusal: the server matches its
// whitelist exactly, so "Fade" is not "fade".
func TestVideoSet_AnimationReachesTheWireLowerCased(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	srv, seen, _ := settingsServer(t)
	defer srv.Close()

	bin := build(t)
	configHome := buildVideoProfile(t, srv.URL)

	stdout, stderr, code := runVideoCmd(t, bin, configHome,
		"video", "set", "42", "--session-id", "s_run", "--subtitle-animation", "FadeUp", "--output", "json")
	if code != 0 {
		t.Fatalf("set exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if style := sentStyle(t, seen); style["animation"] != "fadeup" {
		t.Errorf("animation = %v, want fadeup", style["animation"])
	}
}

// TestVideoSet_TextOutputNamesTheFields covers the human-readable report.
//
// Printing the style struct through Go's default formatting produced
// "{Source Han Sans 36 0 #ffffff rgba(8,8,12,0.68) 0 #000000 0 fade}" — a bare
// list of values naming no field, with zeroes standing in for settings that
// were never made. The point of echoing the result is that someone can check
// it, which that form does not allow.
func TestVideoSet_TextOutputNamesTheFields(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	srv, _, _ := settingsServer(t)
	defer srv.Close()

	bin := build(t)
	configHome := buildVideoProfile(t, srv.URL)

	stdout, stderr, code := runVideoCmd(t, bin, configHome,
		"video", "set", "42", "--session-id", "s_run", "--subtitle-preset", "1")
	if code != 0 {
		t.Fatalf("set exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}

	for _, want := range []string{
		"subtitle_preset = 白字·黑底",
		"backgroundColor = rgba(8,8,12,0.68)",
		"color = #ffffff",
		"fontSize = 36",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout does not contain %q:\n%s", want, stdout)
		}
	}
	// The outline this preset removed must not be reported as a setting.
	// The payload omits a zero stroke width, so claiming "strokeWidth = 0"
	// would describe something that is not being stored.
	if strings.Contains(stdout, "strokeWidth") {
		t.Errorf("stdout reports a strokeWidth the preset cleared and the payload omits:\n%s", stdout)
	}
}

// TestSubtitleCatalogs_AnswerWhatSetWillAccept checks the discovery commands
// hold up their end: an agent that cannot enumerate the valid values will
// guess at them.
func TestSubtitleCatalogs_AnswerWhatSetWillAccept(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	srv, _, _ := settingsServer(t)
	defer srv.Close()

	bin := build(t)
	configHome := buildVideoProfile(t, srv.URL)

	stdout, stderr, code := runVideoCmd(t, bin, configHome, "subtitle", "fonts", "--output", "json")
	if code != 0 {
		t.Fatalf("subtitle fonts exit %d\nstderr: %s", code, stderr)
	}
	var fontsDoc struct {
		Fonts []struct {
			N      int    `json:"n"`
			Family string `json:"family"`
			Label  string `json:"label"`
		} `json:"fonts"`
	}
	if err := json.Unmarshal([]byte(stdout), &fontsDoc); err != nil {
		t.Fatalf("decode %q: %v", stdout, err)
	}
	if len(fontsDoc.Fonts) != 3 {
		t.Fatalf("got %d fonts, want 3", len(fontsDoc.Fonts))
	}
	// The index has to be the one --subtitle-font accepts, or the two
	// commands disagree about what "2" means.
	if fontsDoc.Fonts[1].N != 2 || fontsDoc.Fonts[1].Family != "LXGW WenKai" {
		t.Errorf("font #2 = %+v, want n=2 family=LXGW WenKai", fontsDoc.Fonts[1])
	}

	stdout, stderr, code = runVideoCmd(t, bin, configHome, "subtitle", "presets", "--output", "json")
	if code != 0 {
		t.Fatalf("subtitle presets exit %d\nstderr: %s", code, stderr)
	}
	var presetsDoc struct {
		Presets []struct {
			N     int            `json:"n"`
			Name  string         `json:"name"`
			Sets  []string       `json:"sets"`
			Patch map[string]any `json:"patch"`
		} `json:"presets"`
	}
	if err := json.Unmarshal([]byte(stdout), &presetsDoc); err != nil {
		t.Fatalf("decode %q: %v", stdout, err)
	}
	if len(presetsDoc.Presets) != 2 {
		t.Fatalf("got %d presets, want 2", len(presetsDoc.Presets))
	}
	first := presetsDoc.Presets[0]
	if first.Name != "白字·黑底" {
		t.Errorf("preset #1 = %q, want 白字·黑底", first.Name)
	}
	// strokeWidth: 0 has to survive the round trip into the JSON output.
	// It is the field that tells a caller this preset removes an outline,
	// and an encoder that drops zero values would delete exactly that.
	w, ok := first.Patch["strokeWidth"]
	if !ok || w != float64(0) {
		t.Errorf("preset #1 patch lost strokeWidth:0 (%v, present=%v): %+v", w, ok, first.Patch)
	}
	if !strings.Contains(strings.Join(first.Sets, ","), "strokeWidth") {
		t.Errorf("preset #1 sets = %v, should list strokeWidth as a field it overwrites", first.Sets)
	}
}
