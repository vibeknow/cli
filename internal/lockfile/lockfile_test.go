package lockfile

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestWithLockSerializes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.lock")
	var counter int
	var mu sync.Mutex
	var observed []int
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := WithLock(path, func() error {
				mu.Lock()
				counter++
				observed = append(observed, counter)
				mu.Unlock()
				return nil
			})
			if err != nil {
				t.Errorf("WithLock: %v", err)
			}
		}()
	}
	wg.Wait()
	if counter != 5 {
		t.Fatalf("counter=%d want 5", counter)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("lock file should exist after use: %v", err)
	}
}
