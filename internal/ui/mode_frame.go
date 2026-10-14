package ui

import (
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/ui/input"
	"github.com/bnema/dumber/internal/ui/layout"
	"github.com/bnema/puregotk/v4/glib"
	"github.com/bnema/puregotk/v4/gobject"
	"github.com/bnema/puregotk/v4/graphene"
	"github.com/bnema/puregotk/v4/gtk"
)

const borderRefreshMs = 100

// modeFrame owns the per-window mode border drawn around its target pane or
// workspace, and the Vim pulse on it. The mode indicator and legend belong to
// modeToaster. GTK access is restricted to the main thread, as with the rest of
// the UI.
type modeFrame struct {
	mode        input.Mode
	tabID       entity.TabID
	target      layout.Widget
	frameClass  string
	border      *gtk.Box
	borderStyle cssClassTarget // border's CSS classes; a seam for tests
	borderRect  func() (x, y, width, height int, ok bool)
	overlay     *gtk.Overlay

	refreshSource     uint
	positionHandlerID uint
	pulseCycle        bool
}

func newModeFrame(overlay *gtk.Overlay) *modeFrame {
	if overlay == nil {
		return nil
	}
	f := &modeFrame{mode: input.ModeNormal, overlay: overlay}
	// The border is its own overlay: inset shadows on pane containers are
	// painted below web content and would be invisible.
	f.border = gtk.NewBox(gtk.OrientationVerticalValue, 0)
	f.border.SetCanFocus(false)
	f.border.SetCanTarget(false)
	f.border.SetVisible(false)
	f.border.AddCssClass("mode-frame-border")
	f.borderStyle = f.border
	overlay.AddOverlay(&f.border.Widget)
	overlay.SetClipOverlay(&f.border.Widget, false)
	overlay.SetMeasureOverlay(&f.border.Widget, false)
	// A single positioned overlay tracks the active pane without reparenting it.
	cb := func(_ gtk.Overlay, widgetPtr uintptr, allocation *uintptr) bool {
		if f.border == nil || widgetPtr != f.border.GoPointer() || f.borderRect == nil {
			return false
		}
		x, y, w, h, ok := f.borderRect()
		if !ok {
			return false
		}
		return writeOverlayAllocation(allocation, x, y, w, h)
	}
	f.positionHandlerID = overlay.ConnectGetChildPosition(&cb)
	return f
}

func (f *modeFrame) destroy() {
	if f == nil {
		return
	}
	f.stopGeometryRefresh()
	f.setTarget(nil, "")
	if f.overlay != nil && f.positionHandlerID != 0 {
		gobject.SignalHandlerDisconnect(gobject.ObjectNewFromInternalPtr(f.overlay.GoPointer()), f.positionHandlerID)
		f.positionHandlerID = 0
	}
	f.borderRect = nil
	if f.overlay != nil && f.border != nil {
		f.overlay.RemoveOverlay(&f.border.Widget)
		// NewBox's reference is not released by the removal.
		f.border.Unref()
	}
	f.overlay = nil
	f.border = nil
	f.borderStyle = nil
}

type cssClassTarget interface {
	AddCssClass(string)
	RemoveCssClass(string)
}

// setMode retargets the border for mode; it clears the border for Normal mode,
// which has no target.
func (f *modeFrame) setMode(mode input.Mode, target layout.Widget) {
	if f == nil {
		return
	}
	f.mode = mode
	f.pulseCycle = false
	f.setTarget(target, modeFrameClass(mode))
}

func (f *modeFrame) setTarget(target layout.Widget, class string) {
	if f == nil {
		return
	}
	if target == f.target && class == f.frameClass {
		return
	}
	if f.borderStyle != nil && f.frameClass != "" {
		f.borderStyle.RemoveCssClass(f.frameClass)
		for _, pulseClass := range vimPulseClasses {
			f.borderStyle.RemoveCssClass(pulseClass)
		}
	}
	f.target, f.frameClass = target, class
	if f.borderStyle != nil && target != nil && class != "" {
		f.borderStyle.AddCssClass(class)
	}
	if f.border == nil {
		return
	}
	if target != nil && class != "" {
		f.startGeometryRefresh()
		return
	}
	f.stopGeometryRefresh()
	f.borderRect = nil
	f.border.SetVisible(false)
}

var vimPulseClasses = []string{"vim-mode-pulse", "vim-mode-pulse-fast", "vim-mode-pulse-cycle-a", "vim-mode-pulse-cycle-b"}

func (f *modeFrame) pulse(fast bool) {
	if f == nil || f.mode != input.ModeVim || f.target == nil || f.borderStyle == nil {
		return
	}
	for _, class := range vimPulseClasses {
		f.borderStyle.RemoveCssClass(class)
	}
	class := "vim-mode-pulse"
	if fast {
		class = "vim-mode-pulse-fast"
	}
	f.borderStyle.AddCssClass(class)
	cycle := "vim-mode-pulse-cycle-a"
	if f.pulseCycle {
		cycle = "vim-mode-pulse-cycle-b"
	}
	f.pulseCycle = !f.pulseCycle
	f.borderStyle.AddCssClass(cycle)
}

func modeFrameClass(mode input.Mode) string {
	switch mode {
	case input.ModePane:
		return "pane-mode-active"
	case input.ModeResize:
		return "resize-mode-active"
	case input.ModeVim:
		return "vim-mode-active"
	case input.ModeTab:
		return "tab-mode-active"
	case input.ModeSession:
		return "session-mode-active"
	case input.ModePrefix:
		return "prefix-mode-active"
	default:
		return ""
	}
}

func (f *modeFrame) stopGeometryRefresh() {
	if f == nil || f.refreshSource == 0 {
		return
	}
	glib.SourceRemove(f.refreshSource)
	f.refreshSource = 0
}

// Measure outside GTK allocation while the border is active, including window
// resizes and splitter drags after the initial layout has settled.
func (f *modeFrame) startGeometryRefresh() {
	f.stopGeometryRefresh()
	f.refreshGeometry()
	cb := glib.SourceFunc(func(_ uintptr) bool {
		if f.target == nil || f.border == nil {
			f.refreshSource = 0
			return false
		}
		f.refreshGeometry()
		return true
	})
	f.refreshSource = glib.TimeoutAdd(borderRefreshMs, &cb, 0)
}

// clearPlacement hides the border: the target is gone.
func (f *modeFrame) clearPlacement() {
	if f.borderRect != nil {
		f.borderRect = nil
		f.border.SetVisible(false)
	}
}

// refreshGeometry measures the target outside GTK's positioning callback.
func (f *modeFrame) refreshGeometry() {
	if f.target == nil || f.overlay == nil {
		f.clearPlacement()
		return
	}
	point := &graphene.Point{X: 0, Y: 0}
	out := &graphene.Point{}
	target := f.target.GtkWidget()
	if target == nil || !target.ComputePoint(&f.overlay.Widget, point, out) {
		f.clearPlacement()
		return
	}
	width, height := target.GetAllocatedWidth(), target.GetAllocatedHeight()
	if width <= 0 || height <= 0 {
		f.clearPlacement()
		return
	}
	f.placeBorder(int(out.X), int(out.Y), width, height)
}

func (f *modeFrame) placeBorder(x, y, width, height int) {
	if f.borderRect != nil {
		if ox, oy, ow, oh, _ := f.borderRect(); ox == x && oy == y && ow == width && oh == height {
			return
		}
	}
	f.borderRect = func() (int, int, int, int, bool) { return x, y, width, height, true }
	f.border.SetVisible(true)
	f.border.QueueAllocate()
}
