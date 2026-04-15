// Package lockfile provides a cross-process file lock helper.
// See spec §8.7.
package lockfile

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/gofrs/flock"
)

// WithLock acquires an exclusive OS-level advisory lock on path, runs fn,
// then releases. Blocks until the lock is obtained.
func WithLock(path string, fn func() error) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("lockfile: mkdir: %w", err)
	}
	l := flock.New(path)
	if err := l.Lock(); err != nil {
		return fmt.Errorf("lockfile: acquire %s: %w", path, err)
	}
	defer l.Unlock()
	return fn()
}
