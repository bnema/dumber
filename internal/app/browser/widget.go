package browser

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/bnema/dumber/pkg/webkit"
)

// SafeWidget provides thread-safe access to GTK widgets with validation
type SafeWidget struct {
	ptr           uintptr
	typeInfo      string
	registry      *WidgetRegistry
	isInvalidated bool
	mu            sync.RWMutex
}

// Execute safely executes a function with the widget pointer, ensuring thread safety and validation
// CRITICAL FIX: Don't hold mutex during GTK operations to prevent deadlock
func (w *SafeWidget) Execute(fn func(uintptr) error) error {
	if w == nil || w.registry == nil {
		return fmt.Errorf("SafeWidget is nil or has no registry")
	}

	w.mu.RLock()
	if w.isInvalidated {
		w.mu.RUnlock()
		return fmt.Errorf("widget %s permanently invalidated", w.String())
	}
	ptr := w.ptr
	w.mu.RUnlock()

	if !w.IsValid() {
		return fmt.Errorf("widget %s is no longer valid", w.String())
	}

	if err := fn(ptr); err != nil {
		w.markInvalidIfNeeded(ptr)
		return err
	}

	if !w.IsValid() {
		w.markInvalidIfNeeded(ptr)
		return fmt.Errorf("widget %s became invalid during execution", w.String())
	}

	return nil
}

// ExecuteWithTimeout executes a function with timeout to prevent indefinite blocking
func (w *SafeWidget) ExecuteWithTimeout(fn func(uintptr) error, timeout time.Duration) error {
	if w == nil || w.registry == nil {
		return fmt.Errorf("SafeWidget is nil or has no registry")
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Channel for result
	resultChan := make(chan error, 1)

	w.mu.RLock()
	if w.isInvalidated {
		w.mu.RUnlock()
		return fmt.Errorf("widget %s permanently invalidated", w.String())
	}
	ptr := w.ptr
	w.mu.RUnlock()

	if !w.IsValid() {
		return fmt.Errorf("widget %s is no longer valid", w.String())
	}

	// Phase 2: Execute function in goroutine with timeout
	go func() {
		resultChan <- fn(ptr)
	}()

	// Wait for result or timeout
	select {
	case err := <-resultChan:
		if err != nil {
			w.markInvalidIfNeeded(ptr)
			return err
		}

		if !w.IsValid() {
			w.markInvalidIfNeeded(ptr)
			return fmt.Errorf("widget %s became invalid during execution", w.String())
		}

		return nil
	case <-ctx.Done():
		return fmt.Errorf("widget operation timed out after %v", timeout)
	}
}

// IsValid checks if the underlying GTK widget is still valid
func (w *SafeWidget) IsValid() bool {
	if w == nil || w.ptr == 0 {
		return false
	}

	w.mu.RLock()
	if w.isInvalidated {
		w.mu.RUnlock()
		return false
	}
	ptr := w.ptr
	w.mu.RUnlock()

	if !webkit.WidgetIsValid(ptr) {
		w.Invalidate()
		return false
	}

	return true
}

// Invalidate marks the widget as permanently invalid
func (w *SafeWidget) Invalidate() {
	if w == nil {
		return
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	w.isInvalidated = true
}

// markInvalidIfNeeded cleans up registry state when a widget becomes invalid during execution
func (w *SafeWidget) markInvalidIfNeeded(ptr uintptr) {
	if w == nil {
		return
	}

	if w.IsValid() {
		return
	}

	w.Invalidate()
	if w.registry != nil {
		w.registry.Unregister(ptr)
	}
}

// String returns a string representation of the SafeWidget for debugging
func (w *SafeWidget) String() string {
	if w == nil {
		return "SafeWidget(nil)"
	}

	w.mu.RLock()
	typeInfo := w.typeInfo
	ptr := w.ptr
	w.mu.RUnlock()

	return fmt.Sprintf("SafeWidget(%s@%#x)", typeInfo, ptr)
}

// Ptr returns the raw uintptr to the GTK widget
func (w *SafeWidget) Ptr() uintptr {
	if w == nil {
		return 0
	}

	w.mu.RLock()
	defer w.mu.RUnlock()

	return w.ptr
}

// WidgetRegistry manages a collection of SafeWidget instances with thread safety
type WidgetRegistry struct {
	mu      sync.RWMutex
	widgets map[uintptr]*SafeWidget
}

// NewWidgetRegistry creates a new widget registry
func NewWidgetRegistry() *WidgetRegistry {
	return &WidgetRegistry{
		widgets: make(map[uintptr]*SafeWidget),
	}
}

// Register creates a new SafeWidget and registers it in the registry
func (r *WidgetRegistry) Register(ptr uintptr, typeInfo string) *SafeWidget {
	if r == nil || ptr == 0 {
		return nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Check if widget is already registered
	if existing, exists := r.widgets[ptr]; exists {
		return existing
	}

	// Create new SafeWidget
	widget := &SafeWidget{
		ptr:      ptr,
		typeInfo: typeInfo,
		registry: r,
	}

	r.widgets[ptr] = widget
	return widget
}

// Unregister removes a widget from the registry
func (r *WidgetRegistry) Unregister(ptr uintptr) {
	if r == nil || ptr == 0 {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if widget, exists := r.widgets[ptr]; exists {
		widget.Invalidate()
		delete(r.widgets, ptr)
	}
}

// Cleanup removes all invalid widgets from the registry
func (r *WidgetRegistry) Cleanup() {
	if r == nil {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	for ptr, widget := range r.widgets {
		if !widget.IsValid() {
			delete(r.widgets, ptr)
		}
	}
}

// Count returns the number of registered widgets
func (r *WidgetRegistry) Count() int {
	if r == nil {
		return 0
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.widgets)
}
