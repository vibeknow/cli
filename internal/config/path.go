// Package config manages profiles.yaml and related configuration.
package config

import (
	"os"
	"path/filepath"
	"runtime"
)

// ConfigDir returns the platform-appropriate vibeknow config directory,
// honoring VIBEKNOW_CONFIG_HOME override. Directory is NOT created.
func ConfigDir() (string, error) {
	if d := os.Getenv("VIBEKNOW_CONFIG_HOME"); d != "" {
		return d, nil
	}
	if runtime.GOOS == "windows" {
		return filepath.Join(os.Getenv("AppData"), "vibeknow"), nil
	}
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "vibeknow"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "vibeknow"), nil
}

// ProfilesPath returns the absolute path to profiles.yaml.
func ProfilesPath() (string, error) {
	d, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "profiles.yaml"), nil
}
