//go:build darwin

package keychain

import (
	"testing"

	"github.com/zalando/go-keyring"
)

// stubSystemKeychain replaces keyringGet/keyringSet with an in-memory map so
// tests never touch the real login keychain.
func stubSystemKeychain(t *testing.T) {
	t.Helper()
	mem := map[string]string{}
	origGet, origSet := keyringGet, keyringSet
	t.Cleanup(func() {
		keyringGet = origGet
		keyringSet = origSet
	})
	keyringGet = func(service, account string) (string, error) {
		if v, ok := mem[service+"\x00"+account]; ok {
			return v, nil
		}
		return "", keyring.ErrNotFound
	}
	keyringSet = func(service, account, value string) error {
		mem[service+"\x00"+account] = value
		return nil
	}
}
