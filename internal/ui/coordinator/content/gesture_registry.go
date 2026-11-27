package content

import (
	"sync"

	"bnema.dev/dumber/internal/application/port"
	"bnema.dev/dumber/internal/ui/input"
	"bnema.dev/puregotk/v4/gtk"
)

// gestureHandle is the part of input.GestureHandler the registry drives.
type gestureHandle interface {
	AttachTo(widget *gtk.Widget)
	AttachedTo(widget *gtk.Widget) bool
	Detach()
	SetNavigator(nav input.DirectNavigator)
	SetOnAction(fn input.ActionHandler)
}

type gestureBinding struct {
	handler gestureHandle
	// widget and destroyID identify the destroy signal that drops this
	// binding; destroyCb is retained so the Go GC keeps the callback alive.
	widget    *gtk.Widget
	destroyID uint
	destroyCb func(gtk.Widget)
}

// gestureRegistry keeps one mouse gesture handler per web view, so wrapping
// the same web view again never stacks a second click controller on it.
// The zero value is ready to use.
//
// Entries are dropped by remove, or when the attached widget is destroyed;
// the latter covers web views wrapped outside a pane (native popups, popup
// staging) that never go through the coordinator's release path.
type gestureRegistry struct {
	mu       sync.Mutex
	bindings map[port.WebViewID]*gestureBinding

	// Test seams for the widget destroy signal; nil uses GTK.
	connectDestroy    func(widget *gtk.Widget, cb *func(gtk.Widget)) uint
	disconnectDestroy func(widget *gtk.Widget, id uint)
}

// bind attaches the web view's handler to widget, creating it on first use.
// It does nothing when the handler is already attached to that native widget
// and re-attaches the same handler when the native widget changed. It reports
// whether a controller was attached.
//
// bind must run on the GTK main thread: it drops r.mu before calling into GTK.
func (r *gestureRegistry) bind(
	id port.WebViewID,
	widget *gtk.Widget,
	nav input.DirectNavigator,
	onAction input.ActionHandler,
	newHandler func() gestureHandle,
) bool {
	r.mu.Lock()
	if r.bindings == nil {
		r.bindings = make(map[port.WebViewID]*gestureBinding)
	}
	b := r.bindings[id]
	if b != nil && b.handler.AttachedTo(widget) {
		r.mu.Unlock()
		b.handler.SetNavigator(nav)
		b.handler.SetOnAction(onAction)
		return false
	}
	if b == nil {
		b = &gestureBinding{handler: newHandler()}
		r.bindings[id] = b
	}
	oldWidget, oldDestroyID := b.widget, b.destroyID
	b.widget, b.destroyID, b.destroyCb = nil, 0, nil
	r.mu.Unlock()

	r.disconnect(oldWidget, oldDestroyID)

	// AttachTo detaches the previous controller, which also clears the
	// navigator and action handler, so set them after attaching.
	handler := b.handler
	handler.AttachTo(widget)
	handler.SetNavigator(nav)
	handler.SetOnAction(onAction)

	// Like input.GestureHandler, hand GTK a pointer to the retained field;
	// clearing the field later turns a late emission into a no-op.
	r.mu.Lock()
	b.widget = widget
	b.destroyCb = func(gtk.Widget) { r.dropDestroyed(id, b) }
	r.mu.Unlock()
	destroyID := r.connect(widget, &b.destroyCb)
	r.mu.Lock()
	b.destroyID = destroyID
	r.mu.Unlock()
	return true
}

// remove forgets the web view's handler and returns it so the caller can
// detach it outside its own locks. It returns nil when none was bound.
//
// remove must run on the GTK main thread.
func (r *gestureRegistry) remove(id port.WebViewID) gestureHandle {
	r.mu.Lock()
	b := r.bindings[id]
	if b == nil {
		r.mu.Unlock()
		return nil
	}
	delete(r.bindings, id)
	widget, destroyID := b.widget, b.destroyID
	b.widget, b.destroyID, b.destroyCb = nil, 0, nil
	r.mu.Unlock()

	r.disconnect(widget, destroyID)
	return b.handler
}

// dropDestroyed removes the entry when its widget is destroyed, but only if
// the entry still holds the same binding. The handler cleans up its own
// controller on destroy, so it is not detached here.
func (r *gestureRegistry) dropDestroyed(id port.WebViewID, b *gestureBinding) {
	r.mu.Lock()
	if r.bindings[id] != b {
		r.mu.Unlock()
		return
	}
	delete(r.bindings, id)
	widget, destroyID := b.widget, b.destroyID
	b.widget, b.destroyID, b.destroyCb = nil, 0, nil
	r.mu.Unlock()

	r.disconnect(widget, destroyID)
}

func (r *gestureRegistry) connect(widget *gtk.Widget, cb *func(gtk.Widget)) uint {
	if r.connectDestroy != nil {
		return r.connectDestroy(widget, cb)
	}
	return widget.ConnectDestroy(cb)
}

func (r *gestureRegistry) disconnect(widget *gtk.Widget, id uint) {
	if widget == nil || id == 0 {
		return
	}
	if r.disconnectDestroy != nil {
		r.disconnectDestroy(widget, id)
		return
	}
	widget.DisconnectSignal(id)
}
