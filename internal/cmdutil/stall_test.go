package cmdutil

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestStallNotifier_FiresOnSilenceAndResetsOnTouch(t *testing.T) {
	var fired atomic.Int32
	s := StartStallNotifier(80*time.Millisecond, func(time.Duration) { fired.Add(1) })
	defer s.Stop()

	// Keep touching for a while: no notice may fire.
	for range 5 {
		time.Sleep(20 * time.Millisecond)
		s.Touch()
	}
	if n := fired.Load(); n != 0 {
		t.Fatalf("notifier fired %d times while stream was active", n)
	}

	// Now go silent past the threshold.
	deadline := time.Now().Add(2 * time.Second)
	for fired.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if fired.Load() == 0 {
		t.Fatal("notifier never fired during silence")
	}
}

func TestStallNotifier_StopIsIdempotentAndFinal(t *testing.T) {
	var fired atomic.Int32
	s := StartStallNotifier(30*time.Millisecond, func(time.Duration) { fired.Add(1) })
	s.Stop()
	s.Stop() // second Stop must not panic
	n := fired.Load()
	time.Sleep(100 * time.Millisecond)
	if fired.Load() != n {
		t.Fatal("notifier fired after Stop returned")
	}
}
