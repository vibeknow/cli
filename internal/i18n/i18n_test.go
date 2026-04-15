package i18n

import (
	"os"
	"testing"
)

func TestSelectLocale(t *testing.T) {
	cases := []struct {
		vibe, lang, want string
	}{
		{"", "", "en"},
		{"", "zh_CN.UTF-8", "zh"},
		{"", "en_US.UTF-8", "en"},
		{"zh", "en_US.UTF-8", "zh"},
		{"fr_FR", "zh_CN", "en"}, // unknown falls back
	}
	for _, c := range cases {
		os.Setenv("VIBEKNOW_LANG", c.vibe)
		os.Setenv("LANG", c.lang)
		if got := selectLocale(); got != c.want {
			t.Errorf("VIBEKNOW_LANG=%q LANG=%q -> %q, want %q", c.vibe, c.lang, got, c.want)
		}
	}
	os.Unsetenv("VIBEKNOW_LANG")
	os.Unsetenv("LANG")
}

func TestT(t *testing.T) {
	Register("en", map[string]string{"hello": "Hello, %s!"})
	Register("zh", map[string]string{"hello": "你好，%s！"})

	SetLocale("en")
	if got := T("hello", "world"); got != "Hello, world!" {
		t.Errorf("en: %q", got)
	}
	SetLocale("zh")
	if got := T("hello", "world"); got != "你好，world！" {
		t.Errorf("zh: %q", got)
	}
	if got := T("missing.key"); got != "missing.key" {
		t.Errorf("missing key: %q", got)
	}
}
