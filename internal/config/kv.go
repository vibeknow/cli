package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/shiliu-ai/vibeknow-cli/internal/lockfile"
	"gopkg.in/yaml.v3"
)

// KV is the flat string->string storage for `vibeknow config`.
type KV map[string]string

func kvPath() (string, error) {
	d, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "config.yaml"), nil
}

func kvLock() (string, error) {
	d, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "config.lock"), nil
}

// LoadKV reads config.yaml, returning an empty KV if the file is absent.
func LoadKV() (KV, error) {
	path, err := kvPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return KV{}, nil
	}
	if err != nil {
		return nil, err
	}
	kv := KV{}
	if err := yaml.Unmarshal(data, &kv); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return kv, nil
}

// SaveKV writes config.yaml atomically under a lock.
func SaveKV(kv KV) error {
	path, err := kvPath()
	if err != nil {
		return err
	}
	lp, err := kvLock()
	if err != nil {
		return err
	}
	return lockfile.WithLock(lp, func() error {
		data, err := yaml.Marshal(kv)
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
