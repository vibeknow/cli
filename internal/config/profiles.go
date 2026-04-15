package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/shiliu-ai/vibeknow-cli/internal/lockfile"
	"gopkg.in/yaml.v3"
)

func lockPath() (string, error) {
	d, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return d + "/profiles.lock", nil
}

// LoadProfiles reads profiles.yaml, returning an empty file if absent.
func LoadProfiles() (ProfilesFile, error) {
	var f ProfilesFile
	path, err := ProfilesPath()
	if err != nil {
		return f, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return ProfilesFile{SchemaVersion: "1"}, nil
	}
	if err != nil {
		return f, fmt.Errorf("read %s: %w", path, err)
	}
	if err := yaml.Unmarshal(data, &f); err != nil {
		return f, fmt.Errorf("parse %s: %w", path, err)
	}
	if f.SchemaVersion == "" {
		f.SchemaVersion = "1"
	}
	if err := f.Validate(); err != nil {
		return f, fmt.Errorf("validate %s: %w", path, err)
	}
	return f, nil
}

// SaveProfiles writes profiles.yaml atomically under a file lock.
func SaveProfiles(f ProfilesFile) error {
	if f.SchemaVersion == "" {
		f.SchemaVersion = "1"
	}
	if err := f.Validate(); err != nil {
		return err
	}
	path, err := ProfilesPath()
	if err != nil {
		return err
	}
	lp, err := lockPath()
	if err != nil {
		return err
	}
	return lockfile.WithLock(lp, func() error {
		data, err := yaml.Marshal(f)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return err
		}
		tmp := path + ".tmp"
		if err := os.WriteFile(tmp, data, 0o600); err != nil {
			return err
		}
		return os.Rename(tmp, path)
	})
}

// AddProfile inserts p; fails if p.Name already exists.
func AddProfile(p Profile) error {
	if err := p.Validate(); err != nil {
		return err
	}
	f, err := LoadProfiles()
	if err != nil {
		return err
	}
	for _, existing := range f.Profiles {
		if existing.Name == p.Name {
			return fmt.Errorf("profile %q already exists", p.Name)
		}
	}
	f.Profiles = append(f.Profiles, p)
	if f.Current == "" {
		f.Current = p.Name
	}
	return SaveProfiles(f)
}

// UseProfile sets current.
func UseProfile(name string) error {
	f, err := LoadProfiles()
	if err != nil {
		return err
	}
	for _, p := range f.Profiles {
		if p.Name == name {
			f.Current = name
			return SaveProfiles(f)
		}
	}
	return fmt.Errorf("profile %q not found", name)
}

// RemoveProfile deletes by name; clears current if it matched.
func RemoveProfile(name string) error {
	f, err := LoadProfiles()
	if err != nil {
		return err
	}
	out := f.Profiles[:0]
	found := false
	for _, p := range f.Profiles {
		if p.Name == name {
			found = true
			continue
		}
		out = append(out, p)
	}
	if !found {
		return fmt.Errorf("profile %q not found", name)
	}
	f.Profiles = out
	if f.Current == name {
		f.Current = ""
	}
	return SaveProfiles(f)
}
