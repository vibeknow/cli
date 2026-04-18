//go:build darwin

package keychain

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/zalando/go-keyring"
)

// keychainTimeout bounds system Keychain access to avoid hanging on blocked
// permission prompts.
const keychainTimeout = 5 * time.Second

// fileMasterKeyName is the filename used when the system Keychain is
// unavailable and the master key has to fall back to a local file.
const fileMasterKeyName = "master.key.file"

// Test seams — overridden in keychain_darwin_test.go.
var (
	keyringGet = keyring.Get
	keyringSet = keyring.Set
	storageDir = defaultStorageDir
)

func defaultStorageDir(service string) string {
	if dir := os.Getenv("VIBEKNOW_CONFIG_HOME"); dir != "" {
		return filepath.Join(dir, "keychain", service)
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(".vibeknow", "keychain", service)
	}
	return filepath.Join(home, "Library", "Application Support", service)
}

// useSystemKeychain reports whether the darwin backend should touch the
// real login keychain. When VIBEKNOW_CONFIG_HOME is set (tests, portable
// installs) we stay fully file-based for hermeticity.
func useSystemKeychain() bool {
	return os.Getenv("VIBEKNOW_CONFIG_HOME") == ""
}

// getMasterKey fetches (or creates, if allowCreate) the AES-256 master key
// stored in the system Keychain via zalando/go-keyring. A timeout guards
// against blocked prompts.
func getMasterKey(service string, allowCreate bool) ([]byte, error) {
	if !useSystemKeychain() {
		return nil, errors.New("keychain: system access disabled")
	}
	ctx, cancel := context.WithTimeout(context.Background(), keychainTimeout)
	defer cancel()

	type result struct {
		key []byte
		err error
	}
	ch := make(chan result, 1)
	go func() {
		defer func() { recover() }()
		encoded, err := keyringGet(service, "master.key")
		if err == nil {
			key, decErr := base64.StdEncoding.DecodeString(encoded)
			if decErr == nil && len(key) == masterKeyBytes {
				ch <- result{key: key}
				return
			}
			ch <- result{err: errors.New("keychain: master key corrupted")}
			return
		}
		if !errors.Is(err, keyring.ErrNotFound) {
			ch <- result{err: errors.New("keychain: access blocked")}
			return
		}
		if !allowCreate {
			ch <- result{err: errNotInitialized}
			return
		}
		key, rerr := randomBytes(masterKeyBytes)
		if rerr != nil {
			ch <- result{err: rerr}
			return
		}
		if setErr := keyringSet(service, "master.key", base64.StdEncoding.EncodeToString(key)); setErr != nil {
			ch <- result{err: setErr}
			return
		}
		ch <- result{key: key}
	}()

	select {
	case r := <-ch:
		return r.key, r.err
	case <-ctx.Done():
		return nil, errors.New("keychain: access blocked")
	}
}

// getFileMasterKey is the fallback when the system Keychain is unavailable
// or explicitly disabled.
func getFileMasterKey(service string, allowCreate bool) ([]byte, error) {
	dir := storageDir(service)
	keyPath := filepath.Join(dir, fileMasterKeyName)
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
	// Prefer the file-based master key (works even if system Keychain is
	// blocked). Fall back to the system key for entries encrypted before a
	// file master existed.
	if key, ferr := getFileMasterKey(service, false); ferr == nil {
		if pt, derr := decryptData(data, key); derr == nil {
			return pt, nil
		}
	}
	key, err := getMasterKey(service, false)
	if err != nil {
		return nil, err
	}
	return decryptData(data, key)
}

func platformSet(service, account string, data []byte) error {
	// Priority: existing file master key → system Keychain → new file master.
	key, err := getFileMasterKey(service, false)
	if err != nil {
		key, err = getMasterKey(service, true)
		if err != nil {
			key, err = getFileMasterKey(service, true)
			if err != nil {
				return err
			}
		}
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
