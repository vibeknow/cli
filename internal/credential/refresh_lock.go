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

// lockWaitTimeout caps the wait for the cross-process refresh lock when the
// caller supplies no earlier deadline of its own. It is a backstop against a
// lock file left held by a process that died, not a budget anyone should be
// relying on — commands with a deadline pass one.
const lockWaitTimeout = 30 * time.Second

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
// Waiting for the file lock is bounded by ctx, or by lockWaitTimeout when ctx
// carries no earlier deadline.
//
// ctx is honoured rather than ignored because the caller's deadline is the
// real one. `auth status` allows itself five seconds because a connector host
// polls it every three and abandons it at ten; waiting on this lock under a
// private thirty-second budget would blow through that while the caller
// believed it had bounded the work. On a slow network — where every poll
// refreshes, slowly — the queue of waiting processes is exactly when status
// must give up promptly and report from what it already knows, rather than
// hang and be recorded as a disconnection.
func (r *RefreshLock) DoWithDoubleCheck(
	ctx context.Context,
	isAlreadyRefreshed func() bool,
	fn func() (string, error),
) (string, error) {
	return r.Do(func() (string, error) {
		lockPath := filepath.Join(r.lockDir, fmt.Sprintf("refresh_%s.lock", r.credentialRef))

		if err := os.MkdirAll(r.lockDir, 0o700); err != nil {
			return "", fmt.Errorf("refresh_lock: mkdir %s: %w", r.lockDir, err)
		}

		fl := flock.New(lockPath)

		lockCtx, cancel := context.WithTimeout(ctx, lockWaitTimeout)
		defer cancel()

		locked, err := fl.TryLockContext(lockCtx, 500*time.Millisecond)
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
