package cef

import (
	"errors"
	"sync/atomic"

	purecef "github.com/bnema/purego-cef/cef"
	cef2gtk "github.com/bnema/purego-cef2gtk"
	"github.com/bnema/puregotk/v4/gtk"
)

// ErrAdapterDestroyed is returned when an operation is attempted on a destroyed
// Cef2gtkAdapter.
var ErrAdapterDestroyed = errors.New("cef2gtk_adapter: adapter is destroyed")

// Cef2gtkAdapter is a thin infrastructure adapter around *cef2gtk.View. It
// keeps purego-cef2gtk imports confined to internal/infrastructure/cef.
//
// GTK-thread affinity is a hard constraint: most methods touch GTK/CEF view
// lifecycle or input and MUST be called from Dumber's GTK scheduling path
// (e.g. runOnGTK or runOnGTKSync) unless the caller is already executing on
// the GTK main context. Methods are annotated below.
//
// The adapter does NOT own GL/PBO staging, dirty-rect uploads, or GTK input
// controller implementation — those belong to purego-cef2gtk.
type Cef2gtkAdapter struct {
	view       *cef2gtk.View
	destroyed  atomic.Bool
	destroyCnt atomic.Uint64
}

// NewCef2gtkAdapter creates an accelerated CEF view and wraps it in a thin
// Dumber adapter.
func NewCef2gtkAdapter() *Cef2gtkAdapter {
	v := cef2gtk.NewView()
	if v == nil {
		return nil
	}
	return &Cef2gtkAdapter{view: v}
}

// Widget returns the native GTK widget for embedding into containers.
// The returned pointer must be used from the GTK main thread for packing.
func (a *Cef2gtkAdapter) Widget() *gtk.Widget {
	if a == nil || a.destroyed.Load() || a.view == nil {
		return nil
	}
	return a.view.Widget()
}

// GLArea returns the underlying GtkGLArea when the GLArea backend is active.
// It returns nil for the GDK DMABUF backend.
func (a *Cef2gtkAdapter) GLArea() *gtk.GLArea {
	if a == nil || a.destroyed.Load() || a.view == nil {
		return nil
	}
	return a.view.GLArea()
}

// NativeWidget returns the uintptr of the active GTK widget for embedding via
// port.NativeWidgetProvider. Call on the GTK main thread.
func (a *Cef2gtkAdapter) NativeWidget() uintptr {
	widget := a.Widget()
	if widget == nil {
		return 0
	}
	return widget.GoPointer()
}

// Size returns the bridge's last observed positive widget size, or 1x1 before
// the GTK widget has been allocated. It is safe to call off the GTK thread.
func (a *Cef2gtkAdapter) Size() (int32, int32) {
	if a == nil || a.destroyed.Load() || a.view == nil {
		return 1, 1
	}
	return a.view.Size()
}

// DeviceScaleFactor returns the bridge's last observed GTK scale factor, or 1
// before the widget has reported a scale. It is safe to call off the GTK thread.
func (a *Cef2gtkAdapter) DeviceScaleFactor() float32 {
	if a == nil || a.destroyed.Load() || a.view == nil {
		return 1
	}
	return a.view.DeviceScaleFactor()
}

// AddSizeObserver registers a positive-size change callback. Register and
// unregister from the GTK main thread; callbacks are invoked by the bridge from
// GTK size notifications.
func (a *Cef2gtkAdapter) AddSizeObserver(fn func(width, height int32)) func() {
	if a == nil || a.destroyed.Load() || a.view == nil {
		return func() {}
	}
	return a.view.AddSizeObserver(fn)
}

// HasFocus reports whether the bridge widget currently has GTK focus. Must be
// called on the GTK main thread.
func (a *Cef2gtkAdapter) HasFocus() bool {
	if a == nil || a.destroyed.Load() || a.view == nil {
		return false
	}
	return a.view.HasFocus()
}

// SetCursorFromName applies a named cursor to the bridge widget. Must be called
// on the GTK main thread.
func (a *Cef2gtkAdapter) SetCursorFromName(name string) {
	if a == nil || a.destroyed.Load() || a.view == nil {
		return
	}
	a.view.SetCursorFromName(name)
}

// PrepareOnGTKThread initializes renderer resources owned by purego-cef2gtk.
// Must be called on the GTK main thread.
func (a *Cef2gtkAdapter) PrepareOnGTKThread() error {
	if a == nil || a.destroyed.Load() || a.view == nil {
		return ErrAdapterDestroyed
	}
	return a.view.PrepareOnGTKThread()
}

// RenderHandler returns a CEF render handler that delegates accelerated paint
// handling to purego-cef2gtk. The returned handler satisfies cef.RenderHandler
// and uses the bridge's selected accelerated DMABUF import path.
//
// hooks may be a zero-value cef2gtk.Hooks if no custom callbacks are needed.
func (a *Cef2gtkAdapter) RenderHandler(hooks cef2gtk.Hooks) purecef.RenderHandler {
	if a == nil || a.destroyed.Load() || a.view == nil {
		return nil
	}
	return a.view.RenderHandler(hooks)
}

// ConfigureProfiling enables or disables purego-cef2gtk render profiling for this view.
func (a *Cef2gtkAdapter) ConfigureProfiling(opts cef2gtk.ProfileOptions) error {
	if a == nil || a.destroyed.Load() || a.view == nil {
		return ErrAdapterDestroyed
	}
	return a.view.ConfigureProfiling(opts)
}

// AttachInput attaches GTK event controllers to the view and forwards input
// to the given CEF browser host. Must be called on the GTK main thread.
//
// opts.Scale configures HiDPI scale for pointer coordinate translation.
func (a *Cef2gtkAdapter) AttachInput(host purecef.BrowserHost, opts cef2gtk.InputOptions) error {
	if a == nil || a.destroyed.Load() || a.view == nil {
		return ErrAdapterDestroyed
	}
	return a.view.AttachInput(host, opts)
}

// SetInputHost updates the CEF browser host used by the attached input bridge.
// Must be called on the GTK main thread. Callers must call AttachInput first or
// this returns an error from the underlying bridge.
func (a *Cef2gtkAdapter) SetInputHost(host purecef.BrowserHost) error {
	if a == nil || a.destroyed.Load() || a.view == nil {
		return ErrAdapterDestroyed
	}
	return a.view.SetInputHost(host)
}

// DetachInput removes GTK input controllers attached by AttachInput.
// Must be called on the GTK main thread.
func (a *Cef2gtkAdapter) DetachInput() error {
	if a == nil || a.destroyed.Load() || a.view == nil {
		return ErrAdapterDestroyed
	}
	return a.view.DetachInput()
}

// Diagnostics returns a point-in-time snapshot of bridge rendering diagnostics
// including accelerated paint counts, import/render failures, and events.
func (a *Cef2gtkAdapter) Diagnostics() cef2gtk.Diagnostics {
	if a == nil || a.destroyed.Load() || a.view == nil {
		return cef2gtk.Diagnostics{}
	}
	return a.view.Diagnostics()
}

// Destroy releases renderer resources owned by purego-cef2gtk and disconnects GTK
// signal handlers. Must be called on the GTK main thread. The atomic destroyed
// flag only communicates lifecycle state; accesses to view are not otherwise
// synchronized, so callers must ensure no concurrent adapter method is using the
// view while Destroy runs. After Destroy, operations return ErrAdapterDestroyed
// or a zero-value result.
func (a *Cef2gtkAdapter) Destroy() error {
	if a == nil {
		return ErrAdapterDestroyed
	}
	a.destroyed.Store(true)
	if a.view == nil {
		return ErrAdapterDestroyed
	}
	err := a.view.Destroy()
	a.view = nil
	a.destroyCnt.Add(1)
	return err
}

// IsDestroyed returns true if the adapter has been destroyed.
func (a *Cef2gtkAdapter) IsDestroyed() bool {
	return a == nil || a.destroyed.Load()
}

func (a *Cef2gtkAdapter) destroyCount() uint64 {
	if a == nil {
		return 0
	}
	return a.destroyCnt.Load()
}
