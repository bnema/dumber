// Package textinput provides implementations for text input targets and focus tracking.
package textinput

import (
	"sync"

	"github.com/bnema/dumber/internal/application/port"
)

// FocusProvider tracks which text input target currently has focus.
// It implements port.FocusedInputProvider.
type FocusProvider struct {
	target port.TextInputTarget
	mu     sync.RWMutex
}

// Compile-time interface check.
var _ port.FocusedInputProvider = (*FocusProvider)(nil)

// NewFocusProvider creates a new focus provider.
func NewFocusProvider() *FocusProvider {
	return &FocusProvider{}
}

// GetFocusedInput returns the currently focused text input target.
// Returns nil if no text input has focus.
func (fp *FocusProvider) GetFocusedInput() port.TextInputTarget {
	fp.mu.RLock()
	defer fp.mu.RUnlock()
	return fp.target
}

// SetFocusedInput sets the currently focused text input target.
// Pass nil to clear focus.
func (fp *FocusProvider) SetFocusedInput(target port.TextInputTarget) {
	fp.mu.Lock()
	defer fp.mu.Unlock()
	fp.target = target
}
