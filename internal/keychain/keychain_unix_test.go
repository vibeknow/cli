//go:build linux || freebsd || openbsd || netbsd || dragonfly

package keychain

import "testing"

// stubSystemKeychain is a no-op on Unix-likes — the backend is fully file-based.
func stubSystemKeychain(_ *testing.T) {}
