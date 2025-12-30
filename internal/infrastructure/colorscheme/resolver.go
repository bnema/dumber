package colorscheme

import (
	"sort"
	"strings"
	"sync"

	"github.com/bnema/dumber/internal/application/port"
)

const (
	// sourceFallback indicates no detector provided the preference.
	sourceFallback = "fallback"
	// sourceConfig indicates the preference came from user config.
	sourceConfig = "config"
)

// ConfigProvider provides access to the color scheme configuration.
type ConfigProvider interface {
	// GetColorScheme returns the configured color scheme preference.
	// Expected values: "default", "prefer-dark", "prefer-light", "dark", "light"
	GetColorScheme() string
}

// callbackWrapper wraps a callback function to enable pointer comparison for removal.
type callbackWrapper struct {
	fn func(port.ColorSchemePreference)
}

// Resolver implements port.ColorSchemeResolver.
// It manages multiple detectors and respects config overrides.
type Resolver struct {
	mu        sync.RWMutex
	config    ConfigProvider
	detectors []port.ColorSchemeDetector
	current   port.ColorSchemePreference
	callbacks []*callbackWrapper
}

// NewResolver creates a new color scheme resolver.
// The config provider is used to check for explicit user preferences.
func NewResolver(config ConfigProvider) *Resolver {
	return &Resolver{
		config:    config,
		detectors: make([]port.ColorSchemeDetector, 0),
		current: port.ColorSchemePreference{
			PrefersDark: true, // Default to dark until first Resolve()
			Source:      sourceFallback,
		},
	}
}

// Resolve implements port.ColorSchemeResolver.
func (r *Resolver) Resolve() port.ColorSchemePreference {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.resolveInternal()
}

// resolveInternal performs the actual resolution without locking.
// Caller must hold at least a read lock.
func (r *Resolver) resolveInternal() port.ColorSchemePreference {
	// Check config for explicit override first
	if r.config != nil {
		scheme := r.config.GetColorScheme()
		switch strings.ToLower(scheme) {
		case "prefer-dark", "dark":
			return port.ColorSchemePreference{
				PrefersDark: true,
				Source:      sourceConfig,
			}
		case "prefer-light", "light":
			return port.ColorSchemePreference{
				PrefersDark: false,
				Source:      sourceConfig,
			}
			// "default" or empty falls through to detector chain
		}
	}

	// Sort detectors by priority (highest first)
	sorted := make([]port.ColorSchemeDetector, len(r.detectors))
	copy(sorted, r.detectors)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Priority() > sorted[j].Priority()
	})

	// Try each detector in priority order
	for _, detector := range sorted {
		if !detector.Available() {
			continue
		}
		if prefersDark, ok := detector.Detect(); ok {
			return port.ColorSchemePreference{
				PrefersDark: prefersDark,
				Source:      detector.Name(),
			}
		}
	}

	// Fallback to dark mode if all detectors fail
	return port.ColorSchemePreference{
		PrefersDark: true,
		Source:      sourceFallback,
	}
}

// RegisterDetector implements port.ColorSchemeResolver.
func (r *Resolver) RegisterDetector(detector port.ColorSchemeDetector) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.detectors = append(r.detectors, detector)
}

// Refresh implements port.ColorSchemeResolver.
func (r *Resolver) Refresh() port.ColorSchemePreference {
	r.mu.Lock()
	defer r.mu.Unlock()

	newPref := r.resolveInternal()

	// Only notify if preference changed
	if newPref.PrefersDark != r.current.PrefersDark {
		r.current = newPref
		// Copy callbacks to avoid holding lock during callback invocation
		callbacks := make([]*callbackWrapper, len(r.callbacks))
		copy(callbacks, r.callbacks)

		// Invoke callbacks outside of lock
		r.mu.Unlock()
		for _, cb := range callbacks {
			cb.fn(newPref)
		}
		r.mu.Lock()
	} else {
		r.current = newPref
	}

	return newPref
}

// OnChange implements port.ColorSchemeResolver.
func (r *Resolver) OnChange(callback func(port.ColorSchemePreference)) func() {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Wrap callback to enable pointer comparison for removal
	wrapper := &callbackWrapper{fn: callback}
	r.callbacks = append(r.callbacks, wrapper)

	// Return unregister function
	return func() {
		r.mu.Lock()
		defer r.mu.Unlock()

		// Find and remove callback by pointer equality
		for i, cb := range r.callbacks {
			if cb == wrapper {
				r.callbacks = append(r.callbacks[:i], r.callbacks[i+1:]...)
				return
			}
		}
	}
}
