package credential

import (
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestRefreshLock_SingleProcess(t *testing.T) {
	rl := NewRefreshLock(t.TempDir(), "test-cred")

	const n = 5
	var callCount atomic.Int32

	// started is closed by fn once it is executing, signalling the call is live.
	started := make(chan struct{})
	// release is closed by the test to allow fn to return.
	release := make(chan struct{})
	var startOnce sync.Once

	var allDone sync.WaitGroup

	// Leader: will actually run fn.
	allDone.Add(1)
	go func() {
		defer allDone.Done()
		_, err := rl.Do(func() (string, error) {
			callCount.Add(1)
			startOnce.Do(func() { close(started) })
			<-release
			return "token", nil
		})
		if err != nil {
			t.Errorf("leader Do() error: %v", err)
		}
	}()

	// Wait for fn to be in-flight.
	<-started

	// Followers: should dedup against the leader's in-flight call.
	for i := 0; i < n-1; i++ {
		allDone.Add(1)
		go func() {
			defer allDone.Done()
			_, err := rl.Do(func() (string, error) {
				callCount.Add(1) // should never execute
				return "token", nil
			})
			if err != nil {
				t.Errorf("follower Do() error: %v", err)
			}
		}()
	}

	// Yield to the scheduler repeatedly so followers have time to call Do()
	// and register with singleflight before we release the leader's fn.
	for i := 0; i < 100; i++ {
		runtime.Gosched()
	}
	time.Sleep(10 * time.Millisecond)
	close(release)

	allDone.Wait()

	if got := callCount.Load(); got != 1 {
		t.Errorf("fn called %d times, want 1 (singleflight deduplication failed)", got)
	}
}

func TestRefreshLock_DoubleCheck(t *testing.T) {
	lockDir := t.TempDir()
	rl := NewRefreshLock(lockDir, "test-cred")

	// First call: isAlreadyRefreshed returns false → fn executes.
	fnCalled := false
	result, err := rl.DoWithDoubleCheck(
		func() bool { return false },
		func() (string, error) {
			fnCalled = true
			return "refreshed", nil
		},
	)
	if err != nil {
		t.Fatalf("first DoWithDoubleCheck() error: %v", err)
	}
	if result != "refreshed" {
		t.Errorf("first call result = %q, want %q", result, "refreshed")
	}
	if !fnCalled {
		t.Error("fn was not called on first invocation, expected it to be called")
	}

	// Second call: isAlreadyRefreshed returns true → fn NOT called, returns ("", nil).
	fnCalled = false
	result, err = rl.DoWithDoubleCheck(
		func() bool { return true },
		func() (string, error) {
			fnCalled = true
			return "should-not-happen", nil
		},
	)
	if err != nil {
		t.Fatalf("second DoWithDoubleCheck() error: %v", err)
	}
	if result != "" {
		t.Errorf("second call result = %q, want %q", result, "")
	}
	if fnCalled {
		t.Error("fn was called on second invocation, expected it NOT to be called")
	}
}
