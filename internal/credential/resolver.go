package credential

import (
	"errors"
	"fmt"

	"github.com/vibeknow/cli/internal/i18n"
)

// ErrNotFound signals an absent credential.
var ErrNotFound = errors.New("credential: not found")

// KeychainAccess is the subset of internal/keychain we need, broken out so
// tests can substitute a fake.
type KeychainAccess interface {
	Get(key string) ([]byte, error)
	Set(key string, data []byte) error
	Delete(key string) error
}

// KeychainSource wraps a keychain-like store and a specific entry.
type KeychainSource struct {
	Keychain KeychainAccess
	Entry    string
}

func (k KeychainSource) Get() (string, error) {
	if k.Keychain == nil || k.Entry == "" {
		return "", ErrNotFound
	}
	data, err := k.Keychain.Get(k.Entry)
	if err != nil {
		return "", err
	}
	st := ParseStored(string(data))
	return st.AccessToken, nil
}

// GetStored returns the full StoredToken from the keychain entry.
func (k KeychainSource) GetStored() (StoredToken, error) {
	if k.Keychain == nil || k.Entry == "" {
		return StoredToken{}, ErrNotFound
	}
	data, err := k.Keychain.Get(k.Entry)
	if err != nil {
		return StoredToken{}, err
	}
	return ParseStored(string(data)), nil
}

// Resolver implements the priority order from spec §8.5: env > keychain.
// The spec's third level — an encrypted file fallback — was implemented but
// never wired to a caller, and the keychain backends carry their own on-disk
// fallback for systems without a real keychain, so it was removed rather
// than left as dead crypto code.
type Resolver struct {
	Env      EnvSource
	Keychain KeychainSource
}

// Resolve returns (token, sourceName, error).
func (r Resolver) Resolve() (string, string, error) {
	if tok, err := r.Env.Get(); err == nil {
		return tok, "env", nil
	}
	if tok, err := r.Keychain.Get(); err == nil {
		return tok, "keychain", nil
	}
	return "", "", fmt.Errorf("%s", i18n.T("auth.not_signed_in_short"))
}
