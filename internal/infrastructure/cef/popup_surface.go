package cef

import (
	"context"
	"sync/atomic"
	"unsafe"

	"github.com/bnema/puregotk/v4/gdk"

	purecef "github.com/bnema/purego-cef/cef"
	cef2gtk "github.com/bnema/purego-cef2gtk"
	"github.com/bnema/puregotk/v4/glib"
	"github.com/bnema/puregotk/v4/gtk"

	"github.com/bnema/dumber/internal/logging"
)

type popupBridgeSurface struct {
	ctx         context.Context
	root        *gtk.Widget
	mainWidget  *gtk.Widget
	bridge      *Cef2gtkAdapter
	popupWidget *gtk.Widget
	prepared    atomic.Uint32
	rect        purecef.Rect
}

// newPopupBridgeSurface must be called on the GTK thread because it creates
// and packs GTK widgets around the primary CEF widget.
func newPopupBridgeSurface(
	ctx context.Context,
	mainWidget *gtk.Widget,
	plan cef2gtk.RenderStackPlan,
	applicationScale float64,
) *popupBridgeSurface {
	mainContext := glib.MainContextDefault()
	if mainContext == nil || !mainContext.IsOwner() {
		logging.FromContext(popupSurfaceLogContext(ctx)).Error().Msg("cef: newPopupBridgeSurface called off GTK thread")
		return nil
	}
	if mainWidget == nil {
		return nil
	}
	bridge := NewCef2gtkAdapter(plan, applicationScale)
	if bridge == nil {
		return nil
	}
	popupWidget := bridge.Widget()
	if popupWidget == nil {
		_ = bridge.Destroy()
		return nil
	}
	overlay := gtk.NewOverlay()
	if overlay == nil {
		_ = bridge.Destroy()
		return nil
	}
	overlay.SetChild(mainWidget)
	popupWidget.SetVisible(false)
	popupWidget.SetHalign(gtk.AlignStartValue)
	popupWidget.SetValign(gtk.AlignStartValue)
	popupWidget.SetHexpand(false)
	popupWidget.SetVexpand(false)
	popupWidget.SetCanTarget(false)
	popupWidget.SetCanFocus(false)
	popupWidget.SetFocusable(false)
	overlay.AddOverlay(popupWidget)
	overlay.SetClipOverlay(popupWidget, true)
	overlay.SetMeasureOverlay(popupWidget, false)
	root := &overlay.Widget
	root.SetHexpand(true)
	root.SetVexpand(true)
	root.SetCanFocus(true)
	root.SetFocusable(true)
	root.SetFocusOnClick(true)
	surface := &popupBridgeSurface{
		ctx:         ctx,
		root:        root,
		mainWidget:  mainWidget,
		bridge:      bridge,
		popupWidget: popupWidget,
	}
	// Override GTK's natural-size/alignment placement. Margins are not a
	// position: they participate in size negotiation and can change the popup
	// allocation, which the bridge's fill-mode picture then scales to fit.
	position := func(_ gtk.Overlay, child uintptr, allocation *uintptr) bool {
		if child != popupWidget.GoPointer() || allocation == nil {
			return false
		}
		rect, valid := popupAllocation(surface.rect)
		if !valid {
			return false
		}
		// The generated binding calls GtkAllocation* *uintptr; the native
		// argument is the rectangle storage itself, not a pointer to a pointer.
		*(*gdk.Rectangle)(unsafe.Pointer(allocation)) = rect
		return true
	}
	overlay.ConnectGetChildPosition(&position)
	return surface
}

// popupAllocation deliberately does not intersect with the view bounds. The
// overlay clips overflow; shrinking the allocation would distort hit testing.
func popupAllocation(rect purecef.Rect) (gdk.Rectangle, bool) {
	return gdk.Rectangle{X: rect.X, Y: rect.Y, Width: rect.Width, Height: rect.Height},
		rect.Width > 0 && rect.Height > 0
}

func (s *popupBridgeSurface) RootWidget() *gtk.Widget {
	if s == nil {
		return nil
	}
	return s.root
}

func (s *popupBridgeSurface) RenderHandler(hooks cef2gtk.Hooks) purecef.RenderHandler {
	if s == nil || s.bridge == nil {
		return nil
	}
	return s.bridge.RenderHandler(hooks)
}

func (s *popupBridgeSurface) PrepareOnGTKThread() error {
	if s == nil || s.bridge == nil {
		return nil
	}
	if !s.prepared.CompareAndSwap(0, 1) {
		return nil
	}
	if err := s.bridge.PrepareOnGTKThread(); err != nil {
		s.prepared.Store(0)
		return err
	}
	return nil
}

func (s *popupBridgeSurface) SetPopupVisible(visible bool) {
	if s == nil || s.popupWidget == nil {
		return
	}
	if visible {
		if err := s.PrepareOnGTKThread(); err != nil {
			logging.FromContext(popupSurfaceLogContext(s.ctx)).Error().
				Err(err).
				Msg("cef: popupBridgeSurface.SetPopupVisible failed to PrepareOnGTKThread")
			return
		}
	}
	s.popupWidget.SetVisible(visible)
	if s.root != nil {
		s.root.QueueAllocate()
	}
}

func (s *popupBridgeSurface) SetPopupRect(rect purecef.Rect) {
	if s == nil || s.popupWidget == nil {
		return
	}
	s.rect = rect
	if _, valid := popupAllocation(rect); !valid {
		s.popupWidget.SetVisible(false)
		return
	}
	if s.root != nil {
		s.root.QueueAllocate()
	}
}

func popupSurfaceLogContext(ctx context.Context) context.Context {
	if ctx != nil {
		return ctx
	}
	return context.Background()
}

func (s *popupBridgeSurface) DestroyOnGTKThread() {
	if s == nil || s.bridge == nil {
		return
	}
	_ = s.bridge.Destroy()
	s.bridge = nil
	s.popupWidget = nil
	s.mainWidget = nil
	s.root = nil
	s.prepared.Store(0)
}
