package cmdutil

import (
	"sync"
	"time"
)

// StallNotifier calls notify when a stream has produced no events for
// `interval`, and again for every further interval of continued silence.
// It exists because some pipeline stretches are silent by design — the
// hand-drawn line's whole middle section (theme select, storyboard,
// drawing, vectorize) emits no process events at all, which can mean
// minutes of nothing on an otherwise healthy run. Without a local notice
// that silence is indistinguishable from a hang.
//
// notify runs on the notifier's own goroutine; callers hand it a closure
// that only writes to stderr. The elapsed argument is total silence time
// since the last event (not since start).
type StallNotifier struct {
	mu   sync.Mutex
	last time.Time
	stop chan struct{}
	done chan struct{}
}

func StartStallNotifier(interval time.Duration, notify func(elapsed time.Duration)) *StallNotifier {
	s := &StallNotifier{
		last: time.Now(),
		stop: make(chan struct{}),
		done: make(chan struct{}),
	}
	go func() {
		defer close(s.done)
		// Tick at a fraction of the interval so a notice lands close to
		// the moment the threshold is crossed, not up to one full
		// interval late.
		t := time.NewTicker(interval / 4)
		defer t.Stop()
		var lastNotified time.Time
		for {
			select {
			case <-s.stop:
				return
			case <-t.C:
				s.mu.Lock()
				last := s.last
				s.mu.Unlock()
				silent := time.Since(last)
				if silent >= interval && time.Since(lastNotified) >= interval {
					notify(silent.Truncate(time.Second))
					lastNotified = time.Now()
				}
			}
		}
	}()
	return s
}

// Touch records stream activity, resetting the silence clock.
func (s *StallNotifier) Touch() {
	s.mu.Lock()
	s.last = time.Now()
	s.mu.Unlock()
}

// Stop halts the notifier and waits for the goroutine to exit, so no
// notice can be written after Stop returns (e.g. into a closed stderr
// pipeline or over a final result line).
func (s *StallNotifier) Stop() {
	select {
	case <-s.stop:
	default:
		close(s.stop)
	}
	<-s.done
}
