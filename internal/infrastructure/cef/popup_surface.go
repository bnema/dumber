package cef

import (
	"context"
	"sync/atomic"

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
}

// newPopupBridgeSurface must be called on the GTK thread because it creates
// and packs GTK widgets around the primary CEF widget.
func newPopupBridgeSurface(ctx context.Context, mainWidget *gtk.Widget, plan cef2gtk.RenderStackPlan) *popupBridgeSurface {
	mainContext := glib.MainContextDefault()
	if mainContext == nil || !mainContext.IsOwner() {
		logging.FromContext(popupSurfaceLogContext(ctx)).Error().Msg("cef: newPopupBridgeSurface called off GTK thread")
		return nil
	}
	if mainWidget == nil {
		return nil
	}
	bridge := NewCef2gtkAdapter(plan)
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
	root.SetFocusOnClick(true)
	root.SetFocusChild(mainWidget)
	return &popupBridgeSurface{
		ctx:         ctx,
		root:        root,
		mainWidget:  mainWidget,
		bridge:      bridge,
		popupWidget: popupWidget,
	}
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
	if rect.Width <= 0 || rect.Height <= 0 {
		s.popupWidget.SetVisible(false)
		return
	}
	s.popupWidget.SetMarginStart(int(rect.X))
	s.popupWidget.SetMarginTop(int(rect.Y))
	s.popupWidget.SetSizeRequest(int(rect.Width), int(rect.Height))
	s.popupWidget.QueueResize()
	if s.root != nil {
		s.root.QueueResize()
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
