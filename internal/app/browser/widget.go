package browser

import (
	"fmt"
	"sync"

	"github.com/bnema/dumber/pkg/webkit"
)

// SafeWidget provides thread-safe access to GTK widgets with validation
type SafeWidget struct {
	ptr      uintptr
	typeInfo string
	registry *WidgetRegistry
}

// Execute safely executes a function with the widget pointer, ensuring thread safety and validation
func (w *SafeWidget) Execute(fn func(uintptr) error) error {
	if w == nil || w.registry == nil {
		return fmt.Errorf("SafeWidget is nil or has no registry")
	}

	w.registry.mu.Lock()
	defer w.registry.mu.Unlock()

	if !w.IsValid() {
		return fmt.Errorf("widget %s is no longer valid", w.String())
	}

	return fn(w.ptr)
}

// IsValid checks if the underlying GTK widget is still valid
func (w *SafeWidget) IsValid() bool {
	if w == nil || w.ptr == 0 {
		return false
	}
	return webkit.WidgetIsValid(w.ptr)
}

// String returns a string representation of the SafeWidget for debugging
func (w *SafeWidget) String() string {
	if w == nil {
		return "SafeWidget(nil)"
	}
	return fmt.Sprintf("SafeWidget(%s@%#x)", w.typeInfo, w.ptr)
}

// Ptr returns the raw uintptr to the GTK widget
func (w *SafeWidget) Ptr() uintptr {
	if w == nil {
		return 0
	}
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

	delete(r.widgets, ptr)
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
