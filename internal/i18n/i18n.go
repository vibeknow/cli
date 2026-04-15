// Package i18n provides a minimal key-based string table with locale
// selection via VIBEKNOW_LANG / LANG env vars. See spec §8.9.
package i18n

import (
	"fmt"
	"os"
	"strings"
	"sync"
)

var (
	mu      sync.RWMutex
	tables  = map[string]map[string]string{}
	current = "en"
)

// Register merges entries for a locale (call once per locale at init).
func Register(locale string, entries map[string]string) {
	mu.Lock()
	defer mu.Unlock()
	if tables[locale] == nil {
		tables[locale] = map[string]string{}
	}
	for k, v := range entries {
		tables[locale][k] = v
	}
}

// SetLocale forces the active locale (used by root cmd after flag parsing).
func SetLocale(l string) {
	mu.Lock()
	current = l
	mu.Unlock()
}

// Init reads env vars and picks a locale.
func Init() { SetLocale(selectLocale()) }

func selectLocale() string {
	// VIBEKNOW_LANG takes priority; if set (even to unknown), do not fall
	// through to LANG — unknown values fall back to "en".
	if v := os.Getenv("VIBEKNOW_LANG"); v != "" {
		lc := strings.ToLower(strings.SplitN(v, ".", 2)[0])
		lc = strings.ReplaceAll(lc, "-", "_")
		if strings.HasPrefix(lc, "zh") {
			return "zh"
		}
		if strings.HasPrefix(lc, "en") {
			return "en"
		}
		return "en"
	}
	// Fall back to system LANG.
	if v := os.Getenv("LANG"); v != "" {
		lc := strings.ToLower(strings.SplitN(v, ".", 2)[0])
		lc = strings.ReplaceAll(lc, "-", "_")
		if strings.HasPrefix(lc, "zh") {
			return "zh"
		}
		if strings.HasPrefix(lc, "en") {
			return "en"
		}
	}
	return "en"
}

// T returns the localized string for key formatted with args, falling back
// to English, then to the key itself if unknown.
func T(key string, args ...any) string {
	mu.RLock()
	defer mu.RUnlock()
	if tpl, ok := tables[current][key]; ok {
		return fmt.Sprintf(tpl, args...)
	}
	if tpl, ok := tables["en"][key]; ok {
		return fmt.Sprintf(tpl, args...)
	}
	return key
}
