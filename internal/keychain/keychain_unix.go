//go:build linux || freebsd || openbsd || netbsd || dragonfly

package keychain

import (
	"errors"
	"os"
	"path/filepath"
)

// storageDir is overridden in keychain_unix_test.go.
var storageDir = defaultStorageDir

func defaultStorageDir(service string) string {
	if dir := os.Getenv("VIBEKNOW_CONFIG_HOME"); dir != "" {
		return filepath.Join(dir, "keychain", service)
	}
	home, _ := os.UserHomeDir()
	xdg := os.Getenv("XDG_DATA_HOME")
	if xdg == "" {
		xdg = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(xdg, service)
}

func getMasterKey(service string, allowCreate bool) ([]byte, error) {
	dir := storageDir(service)
	keyPath := filepath.Join(dir, "master.key")
	data, err := os.ReadFile(keyPath)
	if err == nil {
		if len(data) == masterKeyBytes {
			return data, nil
		}
		return nil, errors.New("keychain: master key corrupted")
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if !allowCreate {
		return nil, errNotInitialized
	}
	key, rerr := randomBytes(masterKeyBytes)
	if rerr != nil {
		return nil, rerr
	}
	if err := writeFileAtomic(keyPath, key, 0o600); err != nil {
		// Lost a race — another process created the file; read it back.
		if existing, readErr := os.ReadFile(keyPath); readErr == nil && len(existing) == masterKeyBytes {
			return existing, nil
		}
		return nil, err
	}
	return key, nil
}

func platformGet(service, account string) ([]byte, error) {
	path := filepath.Join(storageDir(service), safeFileName(account))
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	key, err := getMasterKey(service, false)
	if err != nil {
		return nil, err
	}
	return decryptData(data, key)
}

func platformSet(service, account string, data []byte) error {
	key, err := getMasterKey(service, true)
	if err != nil {
		return err
	}
	enc, err := encryptData(data, key)
	if err != nil {
		return err
	}
	target := filepath.Join(storageDir(service), safeFileName(account))
	return writeFileAtomic(target, enc, 0o600)
}

func platformRemove(service, account string) error {
	path := filepath.Join(storageDir(service), safeFileName(account))
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
