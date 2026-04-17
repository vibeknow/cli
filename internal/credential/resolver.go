package credential

import "fmt"

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

// FileSource wraps a *FileStore (nil means unavailable).
type FileSource struct{ Store *FileStore }

func (f FileSource) Get() (string, error) {
	if f.Store == nil {
		return "", ErrNotFound
	}
	data, err := f.Store.Get()
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// Resolver implements the priority order from spec §8.5: env > keychain > file.
type Resolver struct {
	Env      EnvSource
	Keychain KeychainSource
	File     FileSource
}

// Resolve returns (token, sourceName, error).
func (r Resolver) Resolve() (string, string, error) {
	if tok, err := r.Env.Get(); err == nil {
		return tok, "env", nil
	}
	if tok, err := r.Keychain.Get(); err == nil {
		return tok, "keychain", nil
	}
	if tok, err := r.File.Get(); err == nil {
		return tok, "file", nil
	}
	return "", "", fmt.Errorf("no credential available (checked env, keychain, file)")
}
