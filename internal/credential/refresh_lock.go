// Package credential provides helpers for managing CLI credentials.
package credential

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gofrs/flock"
	"golang.org/x/sync/singleflight"
)

// RefreshLock provides dual-layer locking for token refresh:
//  1. Process-internal: singleflight.Group deduplicates concurrent refresh
//     calls within the same process.
//  2. Cross-process: file-based flock prevents multiple CLI processes from
//     refreshing simultaneously.
type RefreshLock struct {
	lockDir       string
	credentialRef string
	group         singleflight.Group
}

// NewRefreshLock returns a RefreshLock that stores its lock file under
// lockDir, keyed by credentialRef.
func NewRefreshLock(lockDir, credentialRef string) *RefreshLock {
	return &RefreshLock{
		lockDir:       lockDir,
		credentialRef: credentialRef,
	}
}

// Do wraps fn in a singleflight.Group keyed by credentialRef so that
// concurrent goroutines in the same process share a single in-flight call.
func (r *RefreshLock) Do(fn func() (string, error)) (string, error) {
	v, err, _ := r.group.Do(r.credentialRef, func() (interface{}, error) {
		return fn()
	})
	if err != nil {
		return "", err
	}
	return v.(string), nil
}

// DoWithDoubleCheck wraps fn in both the singleflight layer and an OS-level
// advisory file lock. After acquiring the file lock it calls isAlreadyRefreshed;
// if that returns true the lock is released and ("", nil) is returned so the
// caller can re-read the credential store. Otherwise fn is executed under the
// lock and its result is returned.
//
// The file lock acquisition times out after 30 s (checked every 500 ms).
func (r *RefreshLock) DoWithDoubleCheck(
	isAlreadyRefreshed func() bool,
	fn func() (string, error),
) (string, error) {
	return r.Do(func() (string, error) {
		lockPath := filepath.Join(r.lockDir, fmt.Sprintf("refresh_%s.lock", r.credentialRef))

		if err := os.MkdirAll(r.lockDir, 0o700); err != nil {
			return "", fmt.Errorf("refresh_lock: mkdir %s: %w", r.lockDir, err)
		}

		fl := flock.New(lockPath)

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		locked, err := fl.TryLockContext(ctx, 500*time.Millisecond)
		if err != nil {
			return "", fmt.Errorf("refresh_lock: acquire %s: %w", lockPath, err)
		}
		if !locked {
			return "", fmt.Errorf("refresh_lock: timed out waiting for %s", lockPath)
		}
		defer fl.Unlock() //nolint:errcheck

		if isAlreadyRefreshed() {
			return "", nil
		}

		return fn()
	})
}
