package ui

import (
	"context"
	"strings"

	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/ui/input"
	"github.com/bnema/dumber/internal/ui/layout"
	"github.com/bnema/puregotk/v4/glib"
	"github.com/bnema/puregotk/v4/gobject"
	"github.com/bnema/puregotk/v4/graphene"
	"github.com/bnema/puregotk/v4/gtk"
)

const (
	legendColumnSpacing  = 8
	legendMaxWidth       = 700
	legendCompactWidth   = 460
	legendTwoColumnWidth = 650
	legendInset          = 16
	legendHeaderSpace    = 80
	legendRefreshMs      = 100
)

// modeFrame owns the per-window mode border and its non-interactive legend.
// GTK access is restricted to the main thread, as with the rest of the UI.
type modeFrame struct {
	mode              input.Mode
	tabID             entity.TabID
	target            layout.Widget
	frameClass        string
	root              *gtk.Box
	panel             *gtk.Box
	content           *gtk.FlowBox
	scroller          *gtk.ScrolledWindow
	heading           *gtk.Label
	rows              map[string][]*gtk.Widget
	keycaps           map[string][]*gtk.Widget
	pending           string
	visible           bool
	style             string
	modeClass         string
	config            entity.WorkspaceStylingConfig
	showTimer         uint
	flashTimer        uint
	generation        uint64
	placement         func() (x, y, width, height int, ok bool)
	refreshSource     uint
	lastColumns       uint
	lastPanelWidth    int
	lastMaxHeight     int
	onShow            func(context.Context)
	overlay           *gtk.Overlay
	positionHandlerID uint
	pulseCycle        bool
}

func newModeFrame(overlay *gtk.Overlay) *modeFrame {
	if overlay == nil {
		return nil
	}
	f := &modeFrame{mode: input.ModeNormal, overlay: overlay}
	f.root = gtk.NewBox(gtk.OrientationVerticalValue, 0)
	f.root.SetHalign(gtk.AlignStartValue)
	f.root.SetValign(gtk.AlignStartValue)
	f.root.SetCanFocus(false)
	f.root.SetCanTarget(false)
	f.root.SetVisible(false)
	f.root.AddCssClass("mode-legend-anchor")
	f.panel = gtk.NewBox(gtk.OrientationVerticalValue, 0)
	f.panel.SetCanFocus(false)
	f.panel.SetCanTarget(false)
	f.panel.AddCssClass("omnibox-container")
	f.panel.AddCssClass("mode-legend-panel")
	f.root.Append(&f.panel.Widget)
	title := ""
	f.heading = gtk.NewLabel(&title)
	f.heading.SetCanTarget(false)
	f.heading.SetCanFocus(false)
	f.heading.SetHalign(gtk.AlignStartValue)
	f.heading.AddCssClass("mode-legend-title")
	f.panel.Append(&f.heading.Widget)
	f.initLegendContent()
	overlay.AddOverlay(&f.root.Widget)
	overlay.SetClipOverlay(&f.root.Widget, false)
	overlay.SetMeasureOverlay(&f.root.Widget, false)
	// A single positioned overlay tracks the active pane without reparenting it.
	cb := func(_ gtk.Overlay, widgetPtr uintptr, allocation *uintptr) bool {
		if widgetPtr != f.root.GoPointer() || f.placement == nil {
			return false
		}
		x, y, w, h, ok := f.placement()
		if !ok {
			return false
		}
		return writeOverlayAllocation(allocation, x, y, w, h)
	}
	f.positionHandlerID = overlay.ConnectGetChildPosition(&cb)
	return f
}

func (f *modeFrame) initLegendContent() {
	f.content = gtk.NewFlowBox()
	f.content.SetCanFocus(false)
	f.content.SetCanTarget(false)
	f.content.SetOrientation(gtk.OrientationHorizontalValue)
	f.content.SetSelectionMode(gtk.SelectionNoneValue)
	f.content.SetMinChildrenPerLine(1)
	f.content.SetMaxChildrenPerLine(3)
	f.content.SetColumnSpacing(legendColumnSpacing)
	f.content.SetRowSpacing(4)
	f.content.AddCssClass("mode-legend-content")
	f.scroller = gtk.NewScrolledWindow()
	f.scroller.SetPolicy(gtk.PolicyNeverValue, gtk.PolicyAutomaticValue)
	f.scroller.SetPropagateNaturalHeight(true)
	f.scroller.SetHasFrame(false)
	f.scroller.SetCanFocus(false)
	f.scroller.SetCanTarget(false)
	f.scroller.SetChild(&f.content.Widget)
	f.panel.Append(&f.scroller.Widget)
}

func (f *modeFrame) destroy() {
	if f == nil {
		return
	}
	f.cancelTimers()
	f.stopGeometryRefresh()
	f.setTarget(nil, "")
	if f.overlay != nil && f.positionHandlerID != 0 {
		gobject.SignalHandlerDisconnect(gobject.ObjectNewFromInternalPtr(f.overlay.GoPointer()), f.positionHandlerID)
		f.positionHandlerID = 0
	}
	f.placement = nil
	f.onShow = nil
	if f.overlay != nil && f.root != nil {
		f.overlay.RemoveOverlay(&f.root.Widget)
	}
	f.overlay = nil
	f.root = nil
}

func (f *modeFrame) setTarget(target layout.Widget, class string) {
	if f == nil {
		return
	}
	if target == f.target && class == f.frameClass {
		return
	}
	if f.target != nil && f.frameClass != "" {
		f.target.RemoveCssClass(f.frameClass)
		if f.frameClass == "vim-mode-active" {
			for _, pulseClass := range []string{"vim-mode-pulse", "vim-mode-pulse-fast", "vim-mode-pulse-cycle-a", "vim-mode-pulse-cycle-b"} {
				f.target.RemoveCssClass(pulseClass)
			}
		}
	}
	f.target, f.frameClass = target, class
	if target != nil && class != "" {
		target.AddCssClass(class)
	}
}

func (f *modeFrame) pulse(fast bool) {
	if f == nil || f.mode != input.ModeVim || f.target == nil {
		return
	}
	for _, class := range []string{"vim-mode-pulse", "vim-mode-pulse-fast", "vim-mode-pulse-cycle-a", "vim-mode-pulse-cycle-b"} {
		f.target.RemoveCssClass(class)
	}
	class := "vim-mode-pulse"
	if fast {
		class = "vim-mode-pulse-fast"
	}
	f.target.AddCssClass(class)
	cycle := "vim-mode-pulse-cycle-a"
	if f.pulseCycle {
		cycle = "vim-mode-pulse-cycle-b"
	}
	f.pulseCycle = !f.pulseCycle
	f.target.AddCssClass(cycle)
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
	default:
		return ""
	}
}

func (f *modeFrame) setMode(
	ctx context.Context, mode input.Mode, target layout.Widget, cfg entity.WorkspaceStylingConfig,
	style string, actions map[string]entity.ActionBinding,
) {
	if f == nil {
		return
	}
	f.cancelTimers()
	f.generation++
	f.mode, f.config = mode, cfg
	f.pending = ""
	f.pulseCycle = false
	f.setTarget(target, modeFrameClass(mode))
	f.hide()
	if f.style != style {
		if f.style != "" {
			f.panel.RemoveCssClass("omnibox-style-" + f.style)
		}
		f.style = style
		if style != "" {
			f.panel.AddCssClass("omnibox-style-" + style)
		}
	}
	if f.modeClass != "" {
		f.panel.RemoveCssClass(f.modeClass)
	}
	f.modeClass = "mode-legend-" + mode.String()
	f.panel.AddCssClass(f.modeClass)
	if mode == input.ModeNormal || cfg.ModeLegend == "off" || target == nil {
		f.content.RemoveAll()
		f.rows = nil
		f.keycaps = nil
		return
	}
	f.build(actions)
	if cfg.ModeLegendAnimations {
		f.panel.RemoveCssClass("mode-legend-no-motion")
	} else {
		f.panel.AddCssClass("mode-legend-no-motion")
	}
	if cfg.ModeLegend == "delay" && cfg.ModeLegendDelayMs > 0 {
		generation := f.generation
		cb := glib.SourceFunc(func(_ uintptr) bool {
			f.showTimer = 0
			if generation == f.generation {
				f.show(ctx)
			}
			return false
		})
		f.showTimer = glib.TimeoutAdd(uint(cfg.ModeLegendDelayMs), &cb, 0)
		return
	}
	f.show(ctx)
}

func (f *modeFrame) cancelTimers() {
	if f == nil {
		return
	}
	if f.showTimer != 0 {
		glib.SourceRemove(f.showTimer)
		f.showTimer = 0
	}
	if f.flashTimer != 0 {
		glib.SourceRemove(f.flashTimer)
		f.flashTimer = 0
	}
}

func (f *modeFrame) hide() {
	if f == nil || f.root == nil {
		return
	}
	f.stopGeometryRefresh()
	f.visible = false
	f.placement = nil
	f.resetLegendHeight()
	f.root.SetVisible(false)
}

func (f *modeFrame) stopGeometryRefresh() {
	if f == nil || f.refreshSource == 0 {
		return
	}
	glib.SourceRemove(f.refreshSource)
	f.refreshSource = 0
}

func (f *modeFrame) show(ctx context.Context) {
	if f == nil || f.mode == input.ModeNormal || f.target == nil {
		return
	}
	f.visible = true
	f.root.SetVisible(true)
	f.startGeometryRefresh()
	if f.onShow != nil {
		f.onShow(ctx)
	}
}

// Measure outside GTK allocation while the legend is active, including window
// resizes and splitter drags after the initial layout has settled.
func (f *modeFrame) startGeometryRefresh() {
	f.stopGeometryRefresh()
	f.refreshGeometry()
	cb := glib.SourceFunc(func(_ uintptr) bool {
		if !f.visible || f.root == nil {
			f.refreshSource = 0
			return false
		}
		f.refreshGeometry()
		return true
	})
	f.refreshSource = glib.TimeoutAdd(legendRefreshMs, &cb, 0)
}

func (f *modeFrame) resetLegendHeight() {
	if f.lastMaxHeight != 0 {
		f.scroller.SetSizeRequest(-1, -1)
		f.scroller.SetPropagateNaturalHeight(true)
		f.lastMaxHeight = 0
	}
}

func (f *modeFrame) clearPlacement() {
	if f.placement == nil {
		return
	}
	f.placement = nil
	f.root.SetVisible(false)
	f.root.QueueAllocate()
	if f.visible && f.onShow != nil {
		f.onShow(context.Background())
	}
}

func (f *modeFrame) measureLegendHeight(panelWidth int) int {
	maxHeight := f.overlay.GetAllocatedHeight() - legendInset
	if f.lastMaxHeight != 0 && maxHeight > f.lastMaxHeight {
		f.resetLegendHeight()
	}
	_, naturalHeight := 0, 0
	f.root.Measure(gtk.OrientationVerticalValue, panelWidth, nil, &naturalHeight, nil, nil)
	if naturalHeight > maxHeight && maxHeight > 0 {
		if f.lastMaxHeight != maxHeight {
			f.scroller.SetPropagateNaturalHeight(false)
			f.scroller.SetSizeRequest(-1, max(maxHeight-legendHeaderSpace, 1))
			f.lastMaxHeight = maxHeight
		}
		return maxHeight
	}
	return naturalHeight
}

// refreshGeometry measures the legend outside GTK's positioning callback.
// A narrow target falls back to the window width, keeping the legend reachable.
func (f *modeFrame) refreshGeometry() {
	previous := f.placement
	if f.target == nil || f.overlay == nil {
		f.clearPlacement()
		return
	}
	overlayWidget := &f.overlay.Widget
	point := &graphene.Point{X: 0, Y: 0}
	out := &graphene.Point{}
	target := f.target.GtkWidget()
	if target == nil || !target.ComputePoint(overlayWidget, point, out) {
		f.clearPlacement()
		return
	}
	width, height := target.GetAllocatedWidth(), target.GetAllocatedHeight()
	if width <= 0 || height <= 0 {
		f.clearPlacement()
		return
	}
	x, bottom := int(out.X), int(out.Y)+height
	// Keep every keycap visible even in a narrow split. No webview resize.
	panelWidth := min(width-legendInset, legendMaxWidth)
	columns := uint(3)
	if panelWidth < legendCompactWidth {
		panelWidth = min(f.overlay.GetAllocatedWidth()-legendInset, legendCompactWidth)
		x = (f.overlay.GetAllocatedWidth() - panelWidth) / 2
		columns = 1
	} else {
		if panelWidth < legendTwoColumnWidth {
			columns = 2
		}
		x += (width - panelWidth) / 2
	}
	if panelWidth <= 0 {
		f.clearPlacement()
		return
	}
	f.sizeLegendColumns(columns, panelWidth)
	naturalHeight := f.measureLegendHeight(panelWidth)
	if naturalHeight <= 0 {
		f.clearPlacement()
		return
	}
	y := bottom - naturalHeight - 4
	if y < 0 {
		y = 0
	}
	f.placement = func() (int, int, int, int, bool) { return x, y, panelWidth, naturalHeight, true }
	f.root.SetVisible(true)
	if previous != nil {
		oldX, oldY, oldWidth, oldHeight, _ := previous()
		if oldX == x && oldY == y && oldWidth == panelWidth && oldHeight == naturalHeight {
			return
		}
	}
	f.root.QueueAllocate()
	if previous == nil && f.onShow != nil {
		f.onShow(context.Background())
	}
}

func (f *modeFrame) sizeLegendColumns(columns uint, width int) {
	if columns != f.lastColumns {
		f.content.SetMaxChildrenPerLine(columns)
		f.lastColumns = columns
	}
	if width != f.lastPanelWidth {
		f.panel.SetSizeRequest(width, -1)
		f.lastPanelWidth = width
	}
}

func modeLegendGroup(mode input.Mode, name string) string {
	switch {
	case name == "confirm" || name == "cancel":
		return "EXIT"
	case strings.HasPrefix(name, "split-"):
		return "SPLIT"
	case strings.HasPrefix(name, "focus-"):
		return "FOCUS"
	case strings.HasPrefix(name, "resize-"):
		return "RESIZE"
	case strings.HasPrefix(name, "vim-scroll-") || strings.HasPrefix(name, "half-page-"):
		return "SCROLL"
	case mode == input.ModeVim && (strings.Contains(name, "next") || strings.Contains(name, "prev") || name == "outline"):
		return "JUMP"
	case mode == input.ModeTab && (name == "next-tab" || name == "previous-tab"):
		return "SWITCH"
	default:
		return "MANAGE"
	}
}
