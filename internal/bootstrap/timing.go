// Package bootstrap provides initialization utilities for the browser.
package bootstrap

import (
	"context"
	"sync"
	"time"

	"github.com/bnema/dumber/internal/logging"
)

// StartupTimer tracks timing for cold start phases.
// Thread-safe for use with parallel initialization.
type StartupTimer struct {
	start  time.Time
	phases map[string]time.Duration
	order  []string // Track insertion order for logging
	last   time.Time
	mu     sync.Mutex
}

// NewStartupTimer creates a new timer starting from now.
func NewStartupTimer() *StartupTimer {
	now := time.Now()
	return &StartupTimer{
		start:  now,
		phases: make(map[string]time.Duration),
		order:  make([]string, 0),
		last:   now,
	}
}

// Mark records the duration since the last mark (or start) for the given phase.
func (t *StartupTimer) Mark(phase string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := time.Now()
	t.phases[phase] = now.Sub(t.last)
	t.order = append(t.order, phase)
	t.last = now
}

// MarkDuration records a specific duration for a phase.
// Useful for operations timed independently (e.g., parallel goroutines).
func (t *StartupTimer) MarkDuration(phase string, d time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.phases[phase] = d
	t.order = append(t.order, phase)
}

// Total returns the total elapsed time since timer creation.
// Thread-safe: uses mutex for memory visibility guarantees.
func (t *StartupTimer) Total() time.Duration {
	t.mu.Lock()
	defer t.mu.Unlock()
	return time.Since(t.start)
}

// Log outputs all timing information to the context logger.
func (t *StartupTimer) Log(ctx context.Context) {
	t.mu.Lock()
	defer t.mu.Unlock()

	log := logging.FromContext(ctx)
	total := time.Since(t.start)

	event := log.Info().Dur("total", total)
	for _, phase := range t.order {
		if dur, ok := t.phases[phase]; ok {
			event = event.Dur(phase, dur)
		}
	}
	event.Msg("startup timing")
}

// LogDebug outputs timing as debug level (for less verbose production logs).
func (t *StartupTimer) LogDebug(ctx context.Context) {
	t.mu.Lock()
	defer t.mu.Unlock()

	log := logging.FromContext(ctx)
	total := time.Since(t.start)

	event := log.Debug().Dur("total", total)
	for _, phase := range t.order {
		if dur, ok := t.phases[phase]; ok {
			event = event.Dur(phase, dur)
		}
	}
	event.Msg("startup timing")
}
