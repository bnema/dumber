package port

import "context"

// IdleInhibitor prevents system idle/screensaver during fullscreen video playback.
// Implementations use refcounting internally - multiple Inhibit calls require
// matching Uninhibit calls before inhibition is released.
type IdleInhibitor interface {
	// Inhibit increments the inhibit refcount. First call activates inhibition.
	// Safe to call multiple times (refcounted).
	Inhibit(ctx context.Context, reason string) error

	// Uninhibit decrements the refcount. When zero, releases inhibition.
	// Safe to call even if not currently inhibited (no-op).
	Uninhibit(ctx context.Context) error

	// IsInhibited returns true if currently inhibiting idle.
	IsInhibited() bool

	// Close releases any held resources. Should be called on application shutdown.
	Close() error
}
