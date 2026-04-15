// Package keychain wraps an OS-native secret store (delegating to
// 99designs/keyring). See spec §4.2.
package keychain

import (
	"fmt"

	"github.com/99designs/keyring"
)

type Keychain struct{ ring keyring.Keyring }

type Option func(*keyring.Config)

// WithFileBackend forces the encrypted FileBackend rooted at dir with the
// given passphrase. Intended for tests and the Linux-headless fallback
// described in spec §4.2.
func WithFileBackend(dir, passphrase string) Option {
	return func(c *keyring.Config) {
		c.AllowedBackends = []keyring.BackendType{keyring.FileBackend}
		c.FileDir = dir
		c.FilePasswordFunc = keyring.FixedStringPrompt(passphrase)
	}
}

// OpenFor opens (or creates) a keychain scoped to service. Production callers
// pass no options — the 99designs library will try Keychain / WinCred /
// SecretService / FileBackend in that order.
func OpenFor(service string, opts ...Option) (*Keychain, error) {
	cfg := keyring.Config{
		ServiceName: service,
		AllowedBackends: []keyring.BackendType{
			keyring.KeychainBackend,
			keyring.WinCredBackend,
			keyring.SecretServiceBackend,
			keyring.FileBackend,
		},
	}
	for _, o := range opts {
		o(&cfg)
	}
	r, err := keyring.Open(cfg)
	if err != nil {
		return nil, fmt.Errorf("keychain open: %w", err)
	}
	return &Keychain{ring: r}, nil
}

func (k *Keychain) Set(key string, data []byte) error {
	return k.ring.Set(keyring.Item{Key: key, Data: data})
}

func (k *Keychain) Get(key string) ([]byte, error) {
	item, err := k.ring.Get(key)
	if err != nil {
		return nil, err
	}
	return item.Data, nil
}

func (k *Keychain) Delete(key string) error { return k.ring.Remove(key) }
