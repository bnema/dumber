package browser

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/bnema/dumber/pkg/webkit"
)

var (
	ErrWidgetDestroyed = errors.New("widget has been destroyed")
	ErrWidgetInvalid   = errors.New("widget is not valid")
	ErrWidgetNil       = errors.New("widget pointer is nil")
)

// SafeWidget provides thread-safe access to GTK widgets with validity checking
type SafeWidget struct {
	ptr       uintptr
	typeInfo  string
	destroyed int32 // atomic flag
	mu        sync.RWMutex
}

// NewSafeWidget creates a new SafeWidget wrapper
func NewSafeWidget(ptr uintptr, typeInfo string) *SafeWidget {
	if ptr == 0 {
		return nil
	}
	return &SafeWidget{
		ptr:      ptr,
		typeInfo: typeInfo,
	}
}

// IsValid checks if the widget is still valid
func (w *SafeWidget) IsValid() bool {
	if w == nil || atomic.LoadInt32(&w.destroyed) == 1 {
		return false
	}

	w.mu.RLock()
	defer w.mu.RUnlock()

	if w.ptr == 0 {
		return false
	}

	return webkit.WidgetIsValid(w.ptr)
}

// Execute performs an operation on the widget if it's valid
func (w *SafeWidget) Execute(op func(uintptr) error) error {
	if w == nil {
		return ErrWidgetNil
	}

	if atomic.LoadInt32(&w.destroyed) == 1 {
		return ErrWidgetDestroyed
	}

	w.mu.RLock()
	defer w.mu.RUnlock()

	if w.ptr == 0 {
		return ErrWidgetNil
	}

	// Double-check validity right before the operation
	if !webkit.WidgetIsValid(w.ptr) {
		atomic.StoreInt32(&w.destroyed, 1)
		return ErrWidgetInvalid
	}

	return op(w.ptr)
}

// ExecuteUnsafe performs an operation without validity checking (for performance-critical paths)
func (w *SafeWidget) ExecuteUnsafe(op func(uintptr) error) error {
	if w == nil {
		return ErrWidgetNil
	}

	w.mu.RLock()
	defer w.mu.RUnlock()

	if w.ptr == 0 || atomic.LoadInt32(&w.destroyed) == 1 {
		return ErrWidgetDestroyed
	}

	return op(w.ptr)
}

// Ptr returns the raw pointer (use with caution)
func (w *SafeWidget) Ptr() uintptr {
	if w == nil || atomic.LoadInt32(&w.destroyed) == 1 {
		return 0
	}

	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.ptr
}

// MarkDestroyed marks the widget as destroyed
func (w *SafeWidget) MarkDestroyed() {
	if w == nil {
		return
	}
	atomic.StoreInt32(&w.destroyed, 1)
}

// Destroy marks the widget as destroyed without actually destroying it
// GTK handles widget destruction automatically through parent-child relationships
func (w *SafeWidget) Destroy() error {
	if w == nil {
		return ErrWidgetNil
	}

	if !atomic.CompareAndSwapInt32(&w.destroyed, 0, 1) {
		return ErrWidgetDestroyed // Already destroyed
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	// Just clear the pointer, let GTK handle the actual destruction
	w.ptr = 0
	return nil
}

// String returns a string representation for debugging
func (w *SafeWidget) String() string {
	if w == nil {
		return "SafeWidget(nil)"
	}

	destroyed := atomic.LoadInt32(&w.destroyed) == 1
	return fmt.Sprintf("SafeWidget(%s:%#x, destroyed=%v)", w.typeInfo, w.ptr, destroyed)
}

// WidgetRegistry tracks all widgets to prevent accessing destroyed widgets
type WidgetRegistry struct {
	widgets map[uintptr]*SafeWidget
	mu      sync.RWMutex
}

// NewWidgetRegistry creates a new widget registry
func NewWidgetRegistry() *WidgetRegistry {
	return &WidgetRegistry{
		widgets: make(map[uintptr]*SafeWidget),
	}
}

// Register a widget with the registry
func (r *WidgetRegistry) Register(ptr uintptr, typeInfo string) *SafeWidget {
	if ptr == 0 {
		return nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if existing, ok := r.widgets[ptr]; ok {
		return existing
	}

	widget := NewSafeWidget(ptr, typeInfo)
	r.widgets[ptr] = widget
	return widget
}

// Get a widget from the registry
func (r *WidgetRegistry) Get(ptr uintptr) *SafeWidget {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.widgets[ptr]
}

// Unregister a widget from the registry
func (r *WidgetRegistry) Unregister(ptr uintptr) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if widget, ok := r.widgets[ptr]; ok {
		widget.MarkDestroyed()
		delete(r.widgets, ptr)
	}
}

// ValidateAll checks all registered widgets for validity
func (r *WidgetRegistry) ValidateAll() map[uintptr]error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	errors := make(map[uintptr]error)
	for ptr, widget := range r.widgets {
		if !widget.IsValid() {
			errors[ptr] = fmt.Errorf("widget %s is invalid", widget.String())
		}
	}
	return errors
}

// CleanupInvalid removes all invalid widgets from the registry
func (r *WidgetRegistry) CleanupInvalid() int {
	r.mu.Lock()
	defer r.mu.Unlock()

	cleaned := 0
	for ptr, widget := range r.widgets {
		if !widget.IsValid() {
			widget.MarkDestroyed()
			delete(r.widgets, ptr)
			cleaned++
		}
	}
	return cleaned
}

// WidgetManager handles widget lifecycle and operations
type WidgetManager struct {
	wm       *WorkspaceManager
	registry *WidgetRegistry
	mu       sync.Mutex
}

// NewWidgetManager creates a new widget manager
func NewWidgetManager(wm *WorkspaceManager) *WidgetManager {
	return &WidgetManager{
		wm:       wm,
		registry: wm.widgetRegistry,
	}
}

// InitializePaneWidgets sets up widget management for a pane node
func (wm *WidgetManager) InitializePaneWidgets(node *paneNode, container uintptr) {
	if node == nil || container == 0 {
		return
	}

	// Register the container widget
	node.container = wm.registry.Register(container, "pane_container")
}

// SafeWidgetOperation performs a widget operation with proper synchronization
func (wm *WidgetManager) SafeWidgetOperation(op func() error) error {
	wm.mu.Lock()
	defer wm.mu.Unlock()
	return op()
}

// ValidateWidgetsForReparenting checks if widgets are valid before reparenting operations
func (wm *WidgetManager) ValidateWidgetsForReparenting(widgets ...*SafeWidget) error {
	for i, widget := range widgets {
		if widget == nil {
			return fmt.Errorf("widget %d is nil", i)
		}
		if !widget.IsValid() {
			return fmt.Errorf("widget %d (%s) is not valid", i, widget.String())
		}
	}
	return nil
}

// SetContainer safely sets the container for a pane node
func (wm *WidgetManager) SetContainer(node *paneNode, container uintptr, typeInfo string) {
	if node == nil {
		return
	}
	node.container = wm.registry.Register(container, typeInfo)
}

// SetTitleBar safely sets the title bar for a pane node
func (wm *WidgetManager) SetTitleBar(node *paneNode, titleBar uintptr) {
	if node == nil {
		return
	}
	node.titleBar = wm.registry.Register(titleBar, "title_bar")
}

// SetStackWrapper safely sets the stack wrapper for a pane node
func (wm *WidgetManager) SetStackWrapper(node *paneNode, stackWrapper uintptr) {
	if node == nil {
		return
	}
	node.stackWrapper = wm.registry.Register(stackWrapper, "stack_wrapper")
}