package port

// ColorSchemePreference represents the resolved color scheme preference.
type ColorSchemePreference struct {
	// PrefersDark indicates whether dark mode is preferred.
	PrefersDark bool

	// Source identifies which detector provided this preference.
	// Empty string means fallback was used.
	Source string
}

// ColorSchemeDetector detects the system's color scheme preference.
// Multiple detectors can be registered with different priorities.
type ColorSchemeDetector interface {
	// Name returns a human-readable name for this detector.
	Name() string

	// Priority returns the detector's priority.
	// Higher values = higher priority (checked first).
	// Recommended ranges:
	//   - 100+: Runtime detectors (libadwaita, GTK Settings)
	//   -  50+: Config file detectors
	//   -  10+: Fallback detectors (gsettings, env vars)
	Priority() int

	// Available returns true if this detector can be used.
	// For example, libadwaita detector returns false before adw.Init().
	Available() bool

	// Detect returns the detected preference and whether detection succeeded.
	// Returns (preference, true) on success, (_, false) if unavailable or detection failed.
	Detect() (prefersDark bool, ok bool)
}

// ColorSchemeResolver resolves the effective color scheme preference.
// It manages multiple detectors and respects config overrides.
type ColorSchemeResolver interface {
	// Resolve returns the current color scheme preference.
	// It checks config for explicit overrides, then queries detectors by priority.
	// If all detectors fail, defaults to dark mode.
	Resolve() ColorSchemePreference

	// RegisterDetector adds a detector to the resolver.
	// Safe to call at any time; the resolver re-evaluates on next Resolve().
	RegisterDetector(detector ColorSchemeDetector)

	// Refresh forces re-evaluation of the color scheme.
	// Call this after registering new detectors or when system preferences change.
	// Returns the new preference.
	Refresh() ColorSchemePreference

	// OnChange registers a callback for color scheme changes.
	// The callback is invoked when Refresh() results in a different preference.
	// Returns a function to unregister the callback.
	OnChange(callback func(ColorSchemePreference)) func()
}
