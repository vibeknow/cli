package update

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/vibeknow/cli/internal/config"
)

const cacheTTL = 24 * time.Hour

type state struct {
	LatestVersion string `json:"latest_version"`
	CheckedAt     int64  `json:"checked_at"`
}

// Info holds version comparison data.
type Info struct {
	Current string
	Latest  string
}

// Message returns a human-readable upgrade notice.
func (i *Info) Message() string {
	return fmt.Sprintf("新版本 %s 可用 (当前 %s)，运行 `npm update -g vibeknow-cli` 升级", i.Latest, i.Current)
}

var pending atomic.Pointer[Info]

// SetPending stores a pending update notice.
func SetPending(info *Info) { pending.Store(info) }

// GetPending returns any pending update notice.
func GetPending() *Info { return pending.Load() }

func statePath() string {
	dir, _ := config.ConfigDir()
	return filepath.Join(dir, "update-state.json")
}

func loadState() *state {
	data, err := os.ReadFile(statePath())
	if err != nil {
		return nil
	}
	var s state
	if err := json.Unmarshal(data, &s); err != nil {
		return nil
	}
	return &s
}

func saveState(s *state) {
	dir, _ := config.ConfigDir()
	os.MkdirAll(dir, 0o700)
	data, _ := json.Marshal(s)
	os.WriteFile(statePath(), data, 0o644)
}

// CheckCached checks local cache only (no network). Fast.
func CheckCached(currentVersion string) *Info {
	if shouldSkip(currentVersion) {
		return nil
	}
	s := loadState()
	if s == nil || s.LatestVersion == "" {
		return nil
	}
	if !isNewer(s.LatestVersion, currentVersion) {
		return nil
	}
	return &Info{Current: currentVersion, Latest: s.LatestVersion}
}

// RefreshCache queries npm registry for latest version. Safe for goroutine.
func RefreshCache(currentVersion string) {
	if shouldSkip(currentVersion) {
		return
	}
	s := loadState()
	if s != nil && time.Since(time.Unix(s.CheckedAt, 0)) < cacheTTL {
		return
	}
	latest := fetchLatestVersion()
	if latest == "" {
		return
	}
	saveState(&state{LatestVersion: latest, CheckedAt: time.Now().Unix()})
}

func fetchLatestVersion() string {
	out, err := exec.Command("npm", "view", "vibeknow-cli", "version").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func shouldSkip(version string) bool {
	if os.Getenv("VIBEKNOW_NO_UPDATE_CHECK") != "" {
		return true
	}
	for _, key := range []string{"CI", "BUILD_NUMBER", "GITHUB_ACTIONS"} {
		if os.Getenv(key) != "" {
			return true
		}
	}
	if version == "dev" || version == "" {
		return true
	}
	return false
}

// isNewer returns true if latest > current (simple semver comparison).
func isNewer(latest, current string) bool {
	latest = strings.TrimPrefix(latest, "v")
	current = strings.TrimPrefix(current, "v")
	if latest == current {
		return false
	}
	lParts := strings.Split(latest, ".")
	cParts := strings.Split(current, ".")
	for i := 0; i < 3; i++ {
		var l, c int
		if i < len(lParts) {
			fmt.Sscanf(lParts[i], "%d", &l)
		}
		if i < len(cParts) {
			fmt.Sscanf(cParts[i], "%d", &c)
		}
		if l > c {
			return true
		}
		if l < c {
			return false
		}
	}
	return false
}
