package colorscheme

import (
	"cmp"
	"slices"
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
	mu              sync.RWMutex
	config          ConfigProvider
	detectors       []port.ColorSchemeDetector
	sortedDetectors []port.ColorSchemeDetector // cached sorted detectors
	current         port.ColorSchemePreference
	callbacks       []*callbackWrapper
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

	// Try each detector in priority order (already sorted)
	for _, detector := range r.sortedDetectors {
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
	r.rebuildSortedDetectors()
}

// rebuildSortedDetectors creates a sorted copy of detectors by priority (highest first).
// Caller must hold the write lock.
func (r *Resolver) rebuildSortedDetectors() {
	r.sortedDetectors = make([]port.ColorSchemeDetector, len(r.detectors))
	copy(r.sortedDetectors, r.detectors)
	slices.SortFunc(r.sortedDetectors, func(a, b port.ColorSchemeDetector) int {
		return cmp.Compare(b.Priority(), a.Priority()) // descending order
	})
}

// Refresh implements port.ColorSchemeResolver.
func (r *Resolver) Refresh() port.ColorSchemePreference {
	r.mu.Lock()

	newPref := r.resolveInternal()

	// Prepare callbacks to invoke if preference changed
	var callbacks []*callbackWrapper
	if newPref.PrefersDark != r.current.PrefersDark {
		r.current = newPref
		callbacks = make([]*callbackWrapper, len(r.callbacks))
		copy(callbacks, r.callbacks)
	} else {
		r.current = newPref
	}

	// Release lock before invoking callbacks to avoid deadlocks
	r.mu.Unlock()

	// Invoke callbacks outside of lock
	for _, cb := range callbacks {
		cb.fn(newPref)
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
