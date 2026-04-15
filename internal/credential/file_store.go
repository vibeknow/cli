package credential

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"golang.org/x/crypto/scrypt"
)

// ErrNotFound signals absent credential. Also returned by Delete when file is absent.
var ErrNotFound = errors.New("credential: not found")

type FileStore struct {
	path       string
	passphrase string
}

func NewFileStore(path, passphrase string) *FileStore {
	return &FileStore{path: path, passphrase: passphrase}
}

// scryptParams are tuned for an interactive CLI; tests will complete in <~1s on a 2020 laptop.
const (
	scryptN    = 1 << 15
	scryptR    = 8
	scryptP    = 1
	keyLen     = 32
	saltLen    = 16
	filePermRW = 0o600
	dirPerm    = 0o700
)

// Set encrypts plaintext with AES-GCM and writes to disk.
// Layout: [salt(16)][nonce][ciphertext+tag].
func (f *FileStore) Set(plaintext []byte) error {
	salt := make([]byte, saltLen)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return err
	}
	key, err := scrypt.Key([]byte(f.passphrase), salt, scryptN, scryptR, scryptP, keyLen)
	if err != nil {
		return err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return err
	}
	ct := gcm.Seal(nil, nonce, plaintext, nil)

	buf := make([]byte, 0, saltLen+len(nonce)+len(ct))
	buf = append(buf, salt...)
	buf = append(buf, nonce...)
	buf = append(buf, ct...)
	if err := os.MkdirAll(filepath.Dir(f.path), dirPerm); err != nil {
		return err
	}
	return os.WriteFile(f.path, buf, filePermRW)
}

// Get returns the decrypted plaintext or ErrNotFound / a decrypt error.
func (f *FileStore) Get() ([]byte, error) {
	buf, err := os.ReadFile(f.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if len(buf) < saltLen+12 {
		return nil, fmt.Errorf("credential: file too short")
	}
	salt := buf[:saltLen]
	key, err := scrypt.Key([]byte(f.passphrase), salt, scryptN, scryptR, scryptP, keyLen)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	ns := gcm.NonceSize()
	if len(buf) < saltLen+ns {
		return nil, fmt.Errorf("credential: file too short for nonce")
	}
	nonce := buf[saltLen : saltLen+ns]
	ct := buf[saltLen+ns:]
	return gcm.Open(nil, nonce, ct, nil)
}

// Delete removes the credential file; returns ErrNotFound if absent.
func (f *FileStore) Delete() error {
	err := os.Remove(f.path)
	if errors.Is(err, os.ErrNotExist) {
		return ErrNotFound
	}
	return err
}
