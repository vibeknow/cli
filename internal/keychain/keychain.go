// Package keychain provides cross-platform secure storage for credentials.
// macOS stores an AES-256 master key in the system Keychain (via
// zalando/go-keyring, no cgo) and encrypts tokens with AES-256-GCM on disk.
// Linux / BSDs store both the master key and tokens on disk. Windows uses
// DPAPI + HKCU registry.
package keychain

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
)

// ErrNotFound is returned by Get when no entry exists for the given account.
var ErrNotFound = errors.New("keychain: item not found")

// errNotInitialized is an internal sentinel used when a master key is missing
// and creation has not been requested.
var errNotInitialized = errors.New("keychain: not initialized")

const (
	masterKeyBytes = 32
	ivBytes        = 12
	tagBytes       = 16
)

// Keychain is a handle scoped to a service name. Operations are stateless;
// the handle is cheap to create and safe to share.
type Keychain struct{ service string }

// Option customizes a Keychain. Reserved for forward compatibility.
type Option func(*Keychain)

// OpenFor returns a Keychain scoped to service. It does not touch the
// underlying store until Get/Set/Delete is called.
func OpenFor(service string, opts ...Option) (*Keychain, error) {
	k := &Keychain{service: service}
	for _, o := range opts {
		o(k)
	}
	return k, nil
}

func (k *Keychain) Set(account string, data []byte) error {
	return platformSet(k.service, account, data)
}

func (k *Keychain) Get(account string) ([]byte, error) {
	return platformGet(k.service, account)
}

func (k *Keychain) Delete(account string) error {
	return platformRemove(k.service, account)
}

var safeFileNameRe = regexp.MustCompile(`[^a-zA-Z0-9._-]`)

func safeFileName(account string) string {
	return safeFileNameRe.ReplaceAllString(account, "_") + ".enc"
}

func randomBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return nil, err
	}
	return b, nil
}

func encryptData(plaintext, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	iv, err := randomBytes(ivBytes)
	if err != nil {
		return nil, err
	}
	ct := aead.Seal(nil, iv, plaintext, nil)
	out := make([]byte, 0, ivBytes+len(ct))
	out = append(out, iv...)
	out = append(out, ct...)
	return out, nil
}

func decryptData(data, key []byte) ([]byte, error) {
	if len(data) < ivBytes+tagBytes {
		return nil, os.ErrInvalid
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return aead.Open(nil, data[:ivBytes], data[ivBytes:], nil)
}

// writeFileAtomic writes data to path via a sibling tempfile + rename, so
// readers never observe a half-written file. The parent directory is created
// with 0700 if missing.
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	success := false
	defer func() {
		if !success {
			_ = os.Remove(tmpPath)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpPath, perm); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	success = true
	return nil
}
