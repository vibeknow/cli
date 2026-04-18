package cmdutil

import "sync"

// Default returns the process-global Factory, building one on first use.
// Tests should call SetDefault with their own Factory instead.
func Default() *Factory {
	defaultMu.Lock()
	defer defaultMu.Unlock()
	if defaultFactory == nil {
		defaultFactory = NewDefault()
	}
	return defaultFactory
}

// SetDefault overrides the process-global Factory. Safe to call concurrently
// with Default.
func SetDefault(f *Factory) {
	defaultMu.Lock()
	defer defaultMu.Unlock()
	defaultFactory = f
}

var (
	defaultMu      sync.Mutex
	defaultFactory *Factory
)
