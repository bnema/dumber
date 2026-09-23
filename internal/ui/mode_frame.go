package ui

import (
	"strings"

	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/ui/input"
	"github.com/bnema/dumber/internal/ui/layout"
	"github.com/bnema/puregotk/v4/gdk"
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
	mode               input.Mode
	tabID              entity.TabID
	target             layout.Widget
	frameClass         string
	root               *gtk.Box
	border             *gtk.Box
	borderStyle        cssClassTarget // border's CSS classes; a seam for tests
	borderRect         func() (x, y, width, height int, ok bool)
	panel              *gtk.Box
	content            *gtk.FlowBox
	scroller           *gtk.ScrolledWindow
	heading            *gtk.Label
	rows               map[string][]*gtk.Widget
	keycaps            map[string][]*gtk.Widget
	pending            string
	visible            bool
	style              string
	modeClass          string
	config             entity.WorkspaceStylingConfig
	showTimer          uint
	flashTimer         uint
	generation         uint64
	placement          func() (x, y, width, height int, ok bool)
	refreshSource      uint
	lastColumns        uint
	lastPanelWidth     int
	lastMaxHeight      int
	overlay            *gtk.Overlay
	window             *gtk.ApplicationWindow
	pressInFlight      bool
	pressIdle          uint
	positionHandlerID  uint
	pulseCycle         bool
	lingering          bool
	pointerX, pointerY float64
	pointerInside      bool
	motionController   *gtk.EventControllerMotion
	keyController      *gtk.EventControllerKey
	clickController    *gtk.GestureClick
	motionCb           func(gtk.EventControllerMotion, float64, float64)
	enterCb            func(gtk.EventControllerMotion, float64, float64)
	leaveCb            func(gtk.EventControllerMotion)
	keyCb              func(gtk.EventControllerKey, uint, uint, gdk.ModifierType) bool
	clickCb            func(gtk.GestureClick, int, float64, float64)
}

func newModeFrame(overlay *gtk.Overlay, window *gtk.ApplicationWindow) *modeFrame {
	if overlay == nil || window == nil {
		return nil
	}
	f := &modeFrame{mode: input.ModeNormal, overlay: overlay, window: window}
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
	overlay.AddOverlay(&f.root.Widget)
	overlay.SetClipOverlay(&f.root.Widget, false)
	overlay.SetMeasureOverlay(&f.root.Widget, false)
	// A single positioned overlay tracks the active pane without reparenting it.
	cb := func(_ gtk.Overlay, widgetPtr uintptr, allocation *uintptr) bool {
		placement := f.placement
		if f.border != nil && widgetPtr == f.border.GoPointer() {
			placement = f.borderRect
		} else if widgetPtr != f.root.GoPointer() {
			return false
		}
		if placement == nil {
			return false
		}
		x, y, w, h, ok := placement()
		if !ok {
			return false
		}
		return writeOverlayAllocation(allocation, x, y, w, h)
	}
	f.positionHandlerID = overlay.ConnectGetChildPosition(&cb)
	f.installDismissControllers()
	return f
}

func (f *modeFrame) installDismissControllers() {
	overlay := f.overlay
	window := f.window
	f.motionController = gtk.NewEventControllerMotion()
	f.motionCb = func(_ gtk.EventControllerMotion, x, y float64) {
		f.pointerX, f.pointerY, f.pointerInside = x, y, true
	}
	f.leaveCb = func(_ gtk.EventControllerMotion) { f.pointerInside = false }
	f.enterCb = func(_ gtk.EventControllerMotion, x, y float64) {
		f.pointerX, f.pointerY, f.pointerInside = x, y, true
	}
	f.motionController.ConnectEnter(&f.enterCb)
	f.motionController.ConnectMotion(&f.motionCb)
	f.motionController.ConnectLeave(&f.leaveCb)
	overlay.AddController(&f.motionController.EventController)
	f.keyController = gtk.NewEventControllerKey()
	f.keyController.SetPropagationPhase(gtk.PhaseCaptureValue)
	f.keyCb = func(_ gtk.EventControllerKey, _, _ uint, _ gdk.ModifierType) bool {
		f.dismissLinger()
		return false
	}
	f.keyController.ConnectKeyPressed(&f.keyCb)
	window.AddController(&f.keyController.EventController)
	f.clickController = gtk.NewGestureClick()
	f.clickController.SetButton(0)
	f.clickController.SetPropagationPhase(gtk.PhaseCaptureValue)
	f.clickCb = func(_ gtk.GestureClick, _ int, _, _ float64) {
		f.dismissLinger()
		f.pressInFlight = true
		if f.pressIdle != 0 {
			glib.SourceRemove(f.pressIdle)
		}
		cb := glib.SourceFunc(func(_ uintptr) bool {
			f.pressInFlight = false
			f.pressIdle = 0
			return false
		})
		f.pressIdle = glib.IdleAdd(&cb, 0)
	}
	f.clickController.ConnectPressed(&f.clickCb)
	window.AddController(&f.clickController.EventController)
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
	f.dismissLinger()
	f.stopGeometryRefresh()
	if f.pressIdle != 0 {
		glib.SourceRemove(f.pressIdle)
		f.pressIdle = 0
	}
	if f.overlay != nil {
		f.overlay.RemoveController(&f.motionController.EventController)
	}
	if f.window != nil {
		f.window.RemoveController(&f.keyController.EventController)
		f.window.RemoveController(&f.clickController.EventController)
	}
	f.setTarget(nil, "")
	if f.overlay != nil && f.positionHandlerID != 0 {
		gobject.SignalHandlerDisconnect(gobject.ObjectNewFromInternalPtr(f.overlay.GoPointer()), f.positionHandlerID)
		f.positionHandlerID = 0
	}
	f.placement = nil
	f.borderRect = nil
	if f.overlay != nil && f.root != nil {
		f.overlay.RemoveOverlay(&f.root.Widget)
	}
	if f.overlay != nil && f.border != nil {
		f.overlay.RemoveOverlay(&f.border.Widget)
	}
	f.overlay = nil
	f.window = nil
	f.root = nil
	f.border = nil
	f.borderStyle = nil
}

type cssClassTarget interface {
	AddCssClass(string)
	RemoveCssClass(string)
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
	default:
		return ""
	}
}

func (f *modeFrame) setMode(
	mode input.Mode, target layout.Widget, cfg entity.WorkspaceStylingConfig,
	style string, actions map[string]entity.ActionBinding,
) {
	if f == nil {
		return
	}
	f.cancelTimers()
	// Normal-mode refreshes must preserve an already lingering legend.
	keepLinger := mode == input.ModeNormal && f.mode == input.ModeNormal && f.lingering
	linger := shouldStartLinger(mode == input.ModeNormal && f.visible, f.pressInFlight,
		cfg.ModeLegendLinger, f.pointerInside, f.pointerX, f.pointerY, f.placement)
	if !keepLinger {
		f.dismissLinger()
	}
	f.generation++
	f.mode, f.config = mode, cfg
	f.pending = ""
	f.pulseCycle = false
	if linger {
		f.lingering = true
		f.visible = false
		f.clearLegendFeedback()
		f.panel.AddCssClass("mode-legend-lingering")
	} else if !keepLinger {
		f.hide()
	}
	f.setTarget(target, modeFrameClass(mode))
	f.setLegendStyle(style, mode, linger || keepLinger)
	if mode == input.ModeNormal || cfg.ModeLegend == "off" || target == nil {
		if !linger && !keepLinger {
			f.content.RemoveAll()
		}
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
				f.show()
			}
			return false
		})
		f.showTimer = glib.TimeoutAdd(uint(cfg.ModeLegendDelayMs), &cb, 0)
		return
	}
	f.show()
}

func (f *modeFrame) setLegendStyle(style string, mode input.Mode, preserveModeClass bool) {
	if f.style != style {
		if f.style != "" {
			f.panel.RemoveCssClass("omnibox-style-" + f.style)
		}
		f.style = style
		if style != "" {
			f.panel.AddCssClass("omnibox-style-" + style)
		}
	}
	if f.modeClass != "" && !preserveModeClass {
		f.panel.RemoveCssClass(f.modeClass)
	}
	if !preserveModeClass {
		f.modeClass = "mode-legend-" + mode.String()
		f.panel.AddCssClass(f.modeClass)
	}
}

func (f *modeFrame) clearLegendFeedback() {
	for _, rows := range f.rows {
		for _, row := range rows {
			row.RemoveCssClass("mode-legend-dim")
		}
	}
	for _, caps := range f.keycaps {
		for _, cap := range caps {
			cap.RemoveCssClass("mode-legend-flash")
		}
	}
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

func pointInRect(x, y float64, rx, ry, width, height int) bool {
	return x >= float64(rx) && y >= float64(ry) && x < float64(rx+width) && y < float64(ry+height)
}

func shouldLinger(enabled, inside bool, x, y float64, placement func() (int, int, int, int, bool)) bool {
	if !enabled || !inside || placement == nil {
		return false
	}
	rx, ry, w, h, ok := placement()
	return ok && pointInRect(x, y, rx, ry, w, h)
}

func shouldStartLinger(exiting, pressInFlight, enabled, inside bool, x, y float64, placement func() (int, int, int, int, bool)) bool {
	return exiting && !pressInFlight && shouldLinger(enabled, inside, x, y, placement)
}

func (f *modeFrame) dismissLinger() {
	if f == nil || !f.lingering {
		return
	}
	f.lingering = false
	f.panel.RemoveCssClass("mode-legend-lingering")
	f.hide()
}

func (f *modeFrame) hide() {
	if f == nil || f.root == nil {
		return
	}
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

func (f *modeFrame) show() {
	if f == nil || f.mode == input.ModeNormal || f.target == nil {
		return
	}
	f.visible = true
	f.startGeometryRefresh()
}

// Measure outside GTK allocation while the legend is active, including window
// resizes and splitter drags after the initial layout has settled.
func (f *modeFrame) startGeometryRefresh() {
	f.stopGeometryRefresh()
	f.refreshGeometry()
	cb := glib.SourceFunc(func(_ uintptr) bool {
		if f.target == nil || f.root == nil || f.lingering {
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

// clearPlacement hides both the border and the legend: the target is gone.
func (f *modeFrame) clearPlacement() {
	if f.borderRect != nil {
		f.borderRect = nil
		f.border.SetVisible(false)
	}
	f.clearLegend()
}

// clearLegend hides only the legend; the border keeps tracking its target.
func (f *modeFrame) clearLegend() {
	if f.placement == nil {
		return
	}
	f.placement = nil
	f.root.SetVisible(false)
	f.root.QueueAllocate()
}

func (f *modeFrame) measureLegendHeight(panelWidth int) int {
	maxHeight := f.overlay.GetAllocatedHeight() - legendInset
	if f.lastMaxHeight != 0 && maxHeight > f.lastMaxHeight {
		f.resetLegendHeight()
	}
	// Measure the panel: GTK reports 0 for the root while it is still hidden.
	_, naturalHeight := 0, 0
	f.panel.Measure(gtk.OrientationVerticalValue, panelWidth, nil, &naturalHeight, nil, nil)
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
	if f.lingering {
		return
	}
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
	f.placeBorder(x, int(out.Y), width, height)
	if !f.visible {
		f.clearLegend()
		return
	}
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
		f.clearLegend()
		return
	}
	f.sizeLegendColumns(columns, panelWidth)
	naturalHeight := f.measureLegendHeight(panelWidth)
	if naturalHeight <= 0 {
		f.clearLegend()
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
