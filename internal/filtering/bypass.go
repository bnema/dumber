// Package filtering provides filter management and compilation for content blocking.
package filtering

import (
	"sync"
)

// BypassRegistry tracks one-time URL bypasses that are allowed through content blocking.
// This is an in-memory only registry that is cleared on browser restart.
type BypassRegistry struct {
	mu      sync.RWMutex
	allowed map[string]bool // URL -> allowed once
}

// NewBypassRegistry creates a new bypass registry.
func NewBypassRegistry() *BypassRegistry {
	return &BypassRegistry{
		allowed: make(map[string]bool),
	}
}

// AllowOnce marks a URL as allowed for one-time bypass.
// The next time IsAllowed is checked for this URL, it will return true
// and remove the entry.
func (r *BypassRegistry) AllowOnce(url string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.allowed[url] = true
}

// IsAllowed checks if a URL is allowed for bypass.
// If the URL is in the registry, it returns true and removes the entry.
// This ensures the bypass is only valid for one navigation attempt.
func (r *BypassRegistry) IsAllowed(url string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.allowed[url] {
		delete(r.allowed, url)
		return true
	}
	return false
}

// Clear removes all entries from the registry.
func (r *BypassRegistry) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.allowed = make(map[string]bool)
}

// Count returns the number of pending bypasses.
func (r *BypassRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.allowed)
}
