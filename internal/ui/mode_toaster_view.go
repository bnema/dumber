package ui

import (
	"github.com/bnema/dumber/internal/ui/input"
	"github.com/bnema/puregotk/v4/gdk"
	"github.com/bnema/puregotk/v4/glib"
	"github.com/bnema/puregotk/v4/gobject"
	"github.com/bnema/puregotk/v4/graphene"
	"github.com/bnema/puregotk/v4/gtk"
)

const (
	legendRefreshMs = 100
	// legendGap separates the legend band from the pill under it.
	legendGap = 6
	// legendScrollStep is how far one wheel notch over the pill scrolls a
	// legend taller than its height budget.
	legendScrollStep = 40
)

// modeToasterEvents carries window events to the toaster. The view only
// forwards them; modeToaster decides what they mean.
type modeToasterEvents struct {
	pillClicked  func()
	pointerMoved func(x, y float64)
	pointerLeft  func()
	keyPressed   func()
	// pressedOutside reports a pointer press anywhere but on the pill.
	pressedOutside func()
}

// gtkModeToasterView paints the mode toaster into the window's content
// overlay as two overlay children:
//
//   - the pill, the only targetable widget of the component. It is a sibling
//     of the legend rather than its parent because GTK skips the descendants of
//     a widget that cannot be targeted.
//   - the legend body, which never receives the pointer. Its rectangle is
//     measured outside GTK's allocation pass and handed to the overlay's
//     get-child-position callback, like the mode border's.
//
// Main thread only.
type gtkModeToasterView struct {
	overlay *gtk.Overlay
	window  *gtk.ApplicationWindow
	events  modeToasterEvents

	pill      *gtk.Box
	pillLabel *gtk.Label
	chevron   *gtk.Label
	root      *gtk.Box // overlay child of the body; its panel is what is measured
	panel     *gtk.Box
	scroller  *gtk.ScrolledWindow
	columnBox *gtk.Box
	hint      *gtk.Label

	pillClick   *gtk.GestureClick
	pillScroll  *gtk.EventControllerScroll
	motion      *gtk.EventControllerMotion
	keys        *gtk.EventControllerKey
	windowClick *gtk.GestureClick

	// Callbacks are retained here: GTK holds only a pointer to them.
	pillClickCb  func(gtk.GestureClick, int, float64, float64)
	pillScrollCb func(gtk.EventControllerScroll, float64, float64) bool
	motionCb     func(gtk.EventControllerMotion, float64, float64)
	enterCb      func(gtk.EventControllerMotion, float64, float64)
	leaveCb      func(gtk.EventControllerMotion)
	keyCb        func(gtk.EventControllerKey, uint, uint, gdk.ModifierType) bool
	windowClickC func(gtk.GestureClick, int, float64, float64)

	positionHandlerID uint
	refreshSource     uint

	// Painted state, kept to skip redundant GTK calls.
	headerText   string
	modeClass    string // CSS class of the pill color
	legendClass  string // CSS class of the panel color
	styleClass   string
	chevronState chevronState
	pillShown    bool
	interactive  bool
	animations   bool

	groups      []legendGroup
	groupBoxes  []*gtk.Box
	columns     [][]*gtk.Box // group boxes per column box, as placed
	columnBoxes []*gtk.Box
	rows        map[string][]*gtk.Widget
	keycaps     map[string][]*gtk.Widget

	bodyWanted bool
	lingering  bool
	// placement is the legend rectangle in overlay coordinates; nil hides it.
	placement                 func() (x, y, width, height int, ok bool)
	lastColumns               int
	lastWidth, lastMaxContent int
}

func newGTKModeToasterView(overlay *gtk.Overlay, window *gtk.ApplicationWindow, events modeToasterEvents) *gtkModeToasterView {
	if overlay == nil || window == nil {
		return nil
	}
	v := &gtkModeToasterView{overlay: overlay, window: window, events: events, animations: true}
	v.buildPill()
	v.buildBody()
	overlay.AddOverlay(&v.pill.Widget)
	overlay.SetClipOverlay(&v.pill.Widget, false)
	overlay.SetMeasureOverlay(&v.pill.Widget, false)
	overlay.AddOverlay(&v.root.Widget)
	overlay.SetClipOverlay(&v.root.Widget, false)
	overlay.SetMeasureOverlay(&v.root.Widget, false)
	cb := func(_ gtk.Overlay, widgetPtr uintptr, allocation *uintptr) bool {
		if v.root == nil || widgetPtr != v.root.GoPointer() || v.placement == nil {
			return false
		}
		x, y, w, h, ok := v.placement()
		return ok && writeOverlayAllocation(allocation, x, y, w, h)
	}
	v.positionHandlerID = overlay.ConnectGetChildPosition(&cb)
	v.installWindowControllers()
	return v
}

// buildPill creates the header. It starts out inert: pointer input reaches the
// pill only while it can toggle a legend (setPillInteractive).
func (v *gtkModeToasterView) buildPill() {
	v.pill = gtk.NewBox(gtk.OrientationHorizontalValue, 0)
	v.pill.SetHalign(gtk.AlignStartValue)
	v.pill.SetValign(gtk.AlignEndValue)
	v.pill.SetHexpand(false)
	v.pill.SetVexpand(false)
	// Widget margins keep the allocation equal to the visible pill, which the
	// click filter and the legend anchor rely on.
	v.pill.SetMarginStart(modeToasterMargin)
	v.pill.SetMarginBottom(modeToasterMargin)
	v.pill.SetVisible(false)
	// A click must never move keyboard focus off the pane's WebView: nothing in
	// the pill can take focus, and a press is claimed by the pill's own gesture.
	v.pill.SetCanFocus(false)
	v.pill.SetFocusable(false)
	v.pill.SetFocusOnClick(false)
	v.pill.SetCanTarget(false)
	v.pill.AddCssClass("mode-toaster-pill")

	empty := ""
	v.pillLabel = gtk.NewLabel(&empty)
	v.pillLabel.SetCanTarget(false)
	v.pillLabel.SetCanFocus(false)
	v.pillLabel.SetFocusable(false)
	v.pillLabel.AddCssClass("mode-toaster-label")
	v.pill.Append(&v.pillLabel.Widget)

	v.chevron = gtk.NewLabel(&empty)
	v.chevron.SetCanTarget(false)
	v.chevron.SetCanFocus(false)
	v.chevron.SetFocusable(false)
	v.chevron.SetVisible(false)
	v.chevron.AddCssClass("mode-toaster-chevron")
	v.pill.Append(&v.chevron.Widget)

	v.pillClick = gtk.NewGestureClick()
	v.pillClick.SetButton(1) // GDK_BUTTON_PRIMARY
	v.pillClickCb = func(_ gtk.GestureClick, nPress int, _, _ float64) {
		// Claim the sequence so no ancestor controller sees this press.
		v.pillClick.SetState(gtk.EventSequenceClaimedValue)
		if nPress == 1 && v.events.pillClicked != nil {
			v.events.pillClicked()
		}
	}
	v.pillClick.ConnectPressed(&v.pillClickCb)
	v.pill.AddController(&v.pillClick.EventController)

	// The legend itself is out of reach of the pointer, so the wheel over the
	// pill scrolls a legend taller than its height budget.
	v.pillScroll = gtk.NewEventControllerScroll(gtk.EventControllerScrollVerticalValue)
	v.pillScrollCb = func(_ gtk.EventControllerScroll, _, dy float64) bool {
		return v.scrollLegend(dy * legendScrollStep)
	}
	v.pillScroll.ConnectScroll(&v.pillScrollCb)
	v.pill.AddController(&v.pillScroll.EventController)
}

func (v *gtkModeToasterView) buildBody() {
	v.root = gtk.NewBox(gtk.OrientationVerticalValue, 0)
	v.root.SetHalign(gtk.AlignStartValue)
	v.root.SetValign(gtk.AlignStartValue)
	v.root.SetCanFocus(false)
	v.root.SetCanTarget(false)
	v.root.SetVisible(false)
	v.root.AddCssClass("mode-legend-anchor")

	v.panel = gtk.NewBox(gtk.OrientationVerticalValue, 0)
	v.panel.SetCanFocus(false)
	v.panel.SetCanTarget(false)
	v.panel.AddCssClass("omnibox-container")
	v.panel.AddCssClass("mode-legend-panel")
	v.root.Append(&v.panel.Widget)

	v.columnBox = gtk.NewBox(gtk.OrientationHorizontalValue, legendColumnGap)
	v.columnBox.SetHomogeneous(true)
	v.columnBox.SetCanFocus(false)
	v.columnBox.SetCanTarget(false)
	v.columnBox.AddCssClass("mode-legend-columns")

	v.scroller = gtk.NewScrolledWindow()
	v.scroller.SetPolicy(gtk.PolicyNeverValue, gtk.PolicyAutomaticValue)
	v.scroller.SetPropagateNaturalHeight(true)
	v.scroller.SetHasFrame(false)
	v.scroller.SetCanFocus(false)
	v.scroller.SetCanTarget(false)
	v.scroller.SetChild(&v.columnBox.Widget)
	v.panel.Append(&v.scroller.Widget)

	hint := "esc to exit"
	v.hint = gtk.NewLabel(&hint)
	v.hint.SetCanFocus(false)
	v.hint.SetCanTarget(false)
	v.hint.SetHalign(gtk.AlignEndValue)
	v.hint.AddCssClass("mode-legend-hint")
	v.panel.Append(&v.hint.Widget)
}

// installWindowControllers watches the whole window, because a lingering
// legend ends on any key press or click, wherever it lands.
func (v *gtkModeToasterView) installWindowControllers() {
	v.motion = gtk.NewEventControllerMotion()
	v.motionCb = func(_ gtk.EventControllerMotion, x, y float64) {
		if v.events.pointerMoved != nil {
			v.events.pointerMoved(x, y)
		}
	}
	v.enterCb = v.motionCb
	v.leaveCb = func(_ gtk.EventControllerMotion) {
		if v.events.pointerLeft != nil {
			v.events.pointerLeft()
		}
	}
	v.motion.ConnectEnter(&v.enterCb)
	v.motion.ConnectMotion(&v.motionCb)
	v.motion.ConnectLeave(&v.leaveCb)
	v.overlay.AddController(&v.motion.EventController)

	v.keys = gtk.NewEventControllerKey()
	v.keys.SetPropagationPhase(gtk.PhaseCaptureValue)
	v.keyCb = func(_ gtk.EventControllerKey, _, _ uint, _ gdk.ModifierType) bool {
		if v.events.keyPressed != nil {
			v.events.keyPressed()
		}
		return false
	}
	v.keys.ConnectKeyPressed(&v.keyCb)
	v.window.AddController(&v.keys.EventController)

	v.windowClick = gtk.NewGestureClick()
	v.windowClick.SetButton(0)
	v.windowClick.SetPropagationPhase(gtk.PhaseCaptureValue)
	v.windowClickC = func(_ gtk.GestureClick, _ int, x, y float64) {
		// A press on the pill is the user driving the toaster, not leaving it:
		// it must not end a lingering legend or count as a click elsewhere.
		if v.pillContains(x, y) {
			return
		}
		if v.events.pressedOutside != nil {
			v.events.pressedOutside()
		}
	}
	v.windowClick.ConnectPressed(&v.windowClickC)
	v.window.AddController(&v.windowClick.EventController)
}

// pillContains reports whether window point (x, y) lies on the interactive pill.
func (v *gtkModeToasterView) pillContains(x, y float64) bool {
	if !v.pillShown || !v.interactive {
		return false
	}
	px, py, w, h, ok := v.pillBounds()
	if !ok {
		return false
	}
	// Window coordinates: the overlay sits below the window's own origin.
	origin, out := &graphene.Point{}, &graphene.Point{}
	if !v.overlay.ComputePoint(&v.window.Widget, origin, out) {
		return false
	}
	return pointInRect(x, y, px+int(out.X), py+int(out.Y), w, h)
}

// pillBounds returns the pill's rectangle in overlay coordinates. The pill is
// pinned to the bottom-left by alignment and a fixed margin, so the rectangle
// follows from the overlay size. gtk_widget_compute_point is not used: for a
// pill with CSS padding it reports the origin shifted by the padding, which
// disagrees with where GTK draws and picks it.
func (v *gtkModeToasterView) pillBounds() (x, y, width, height int, ok bool) {
	if v.pill == nil || v.overlay == nil {
		return 0, 0, 0, 0, false
	}
	width, height = v.pill.GetAllocatedWidth(), v.pill.GetAllocatedHeight()
	if width <= 0 || height <= 0 {
		return 0, 0, 0, 0, false
	}
	overlayHeight := v.overlay.GetAllocatedHeight()
	return modeToasterMargin, overlayHeight - modeToasterMargin - height, width, height, true
}

func (v *gtkModeToasterView) setHeader(text string) {
	if v.headerText == text {
		return
	}
	v.headerText = text
	v.pillLabel.SetText(text)
}

func (v *gtkModeToasterView) setMode(mode input.Mode) {
	pillClass, panelClass := "", ""
	if mode != input.ModeNormal {
		pillClass = "mode-toaster-" + mode.String()
		panelClass = "mode-legend-" + mode.String()
	}
	if pillClass != v.modeClass {
		if v.modeClass != "" {
			v.pill.RemoveCssClass(v.modeClass)
		}
		if pillClass != "" {
			v.pill.AddCssClass(pillClass)
		}
		v.modeClass = pillClass
	}
	if panelClass != v.legendClass {
		if v.legendClass != "" {
			v.panel.RemoveCssClass(v.legendClass)
		}
		if panelClass != "" {
			v.panel.AddCssClass(panelClass)
		}
		v.legendClass = panelClass
	}
}

func (v *gtkModeToasterView) setChevron(state chevronState) {
	if v.chevronState == state {
		return
	}
	v.chevronState = state
	switch state {
	case chevronExpanded:
		v.chevron.SetText("▾")
		v.chevron.SetVisible(true)
		v.setTooltip("Collapse the key legend")
	case chevronCollapsed:
		v.chevron.SetText("▴")
		v.chevron.SetVisible(true)
		v.setTooltip("Expand the key legend")
	default:
		v.chevron.SetVisible(false)
		v.setTooltip("")
	}
}

func (v *gtkModeToasterView) setTooltip(text string) {
	if text == "" {
		v.pill.SetTooltipText(nil)
		return
	}
	v.pill.SetTooltipText(&text)
}

func (v *gtkModeToasterView) setPillVisible(visible bool) {
	if v.pillShown == visible {
		return
	}
	v.pillShown = visible
	v.pill.SetVisible(visible)
}

func (v *gtkModeToasterView) setPillInteractive(interactive bool) {
	if v.interactive == interactive {
		return
	}
	v.interactive = interactive
	v.pill.SetCanTarget(interactive)
	if interactive {
		v.pill.AddCssClass("mode-toaster-interactive")
		cursor := "pointer"
		v.pill.SetCursorFromName(&cursor)
		return
	}
	v.pill.RemoveCssClass("mode-toaster-interactive")
	v.pill.SetCursorFromName(nil)
}

func (v *gtkModeToasterView) setStyle(style string) {
	if v.styleClass == "" && style == "" {
		return
	}
	class := ""
	if style != "" {
		class = "omnibox-style-" + style
	}
	if class == v.styleClass {
		return
	}
	if v.styleClass != "" {
		v.panel.RemoveCssClass(v.styleClass)
	}
	if class != "" {
		v.panel.AddCssClass(class)
	}
	v.styleClass = class
}

func (v *gtkModeToasterView) setAnimations(enabled bool) {
	if v.animations == enabled {
		return
	}
	v.animations = enabled
	if enabled {
		v.panel.RemoveCssClass("mode-legend-no-motion")
	} else {
		v.panel.AddCssClass("mode-legend-no-motion")
	}
}

func (v *gtkModeToasterView) setBodyLingering(lingering bool) {
	if v.lingering == lingering {
		return
	}
	v.lingering = lingering
	if lingering {
		v.panel.AddCssClass("mode-legend-lingering")
		v.stopRefresh()
		return
	}
	v.panel.RemoveCssClass("mode-legend-lingering")
	if v.bodyWanted {
		// The frozen placement is stale; track the window again.
		v.startRefresh()
	}
}

// setLegend replaces the legend content; nil empties it.
func (v *gtkModeToasterView) setLegend(groups []legendGroup) {
	v.clearColumns()
	v.groupBoxes = nil
	v.rows = make(map[string][]*gtk.Widget)
	v.keycaps = make(map[string][]*gtk.Widget)
	v.groups = groups
	for _, group := range groups {
		v.groupBoxes = append(v.groupBoxes, v.buildGroup(group))
	}
	// Force the next geometry pass to place the groups.
	v.lastColumns, v.lastWidth = 0, 0
	if v.bodyWanted && !v.lingering {
		v.refreshGeometry()
	}
}

func (v *gtkModeToasterView) buildGroup(group legendGroup) *gtk.Box {
	box := gtk.NewBox(gtk.OrientationVerticalValue, 0)
	box.SetCanFocus(false)
	box.SetCanTarget(false)
	box.AddCssClass("mode-legend-group")
	name := group.Name
	title := gtk.NewLabel(&name)
	title.SetCanFocus(false)
	title.SetCanTarget(false)
	title.SetHalign(gtk.AlignStartValue)
	title.AddCssClass("mode-legend-group-title")
	box.Append(&title.Widget)
	for _, row := range group.Rows {
		box.Append(&v.buildRow(row).Widget)
	}
	return box
}

func (v *gtkModeToasterView) buildRow(row legendRow) *gtk.Box {
	box := gtk.NewBox(gtk.OrientationHorizontalValue, 0)
	box.SetCanFocus(false)
	box.SetCanTarget(false)
	box.AddCssClass("mode-legend-row")
	v.rows[row.Action] = append(v.rows[row.Action], &box.Widget)
	for _, key := range row.Keys {
		display := displayLegendKey(key)
		keycap := gtk.NewLabel(&display)
		keycap.SetCanFocus(false)
		keycap.SetCanTarget(false)
		keycap.AddCssClass("mode-legend-keycap")
		box.Append(&keycap.Widget)
		v.keycaps[row.Action] = append(v.keycaps[row.Action], &keycap.Widget)
	}
	desc := row.Desc
	text := gtk.NewLabel(&desc)
	text.SetCanFocus(false)
	text.SetCanTarget(false)
	text.SetHalign(gtk.AlignStartValue)
	text.SetEllipsize(3) // PangoEllipsizeEnd
	text.AddCssClass("mode-legend-description")
	box.Append(&text.Widget)
	return box
}

// clearColumns detaches the group boxes and drops the column boxes, keeping
// the group boxes for the next layout.
func (v *gtkModeToasterView) clearColumns() {
	for i, col := range v.columnBoxes {
		for _, group := range v.columns[i] {
			col.Remove(&group.Widget)
		}
		v.columnBox.Remove(&col.Widget)
	}
	v.columnBoxes, v.columns = nil, nil
}

// layoutColumns distributes the groups over count columns.
func (v *gtkModeToasterView) layoutColumns(count int) {
	v.clearColumns()
	for _, indices := range distributeLegendGroups(legendGroupHeights(v.groups), count) {
		col := gtk.NewBox(gtk.OrientationVerticalValue, 0)
		col.SetCanFocus(false)
		col.SetCanTarget(false)
		col.SetHexpand(true)
		col.SetValign(gtk.AlignStartValue)
		col.AddCssClass("mode-legend-column")
		placed := make([]*gtk.Box, 0, len(indices))
		for _, i := range indices {
			col.Append(&v.groupBoxes[i].Widget)
			placed = append(placed, v.groupBoxes[i])
		}
		v.columnBox.Append(&col.Widget)
		v.columnBoxes = append(v.columnBoxes, col)
		v.columns = append(v.columns, placed)
	}
	v.lastColumns = count
}

func (v *gtkModeToasterView) setActionDimmed(action string, dimmed bool) {
	for _, row := range v.rows[action] {
		if dimmed {
			row.AddCssClass("mode-legend-dim")
		} else {
			row.RemoveCssClass("mode-legend-dim")
		}
	}
}

func (v *gtkModeToasterView) flashAction(action string) bool {
	caps := v.keycaps[action]
	if len(caps) == 0 {
		return false
	}
	v.clearFlash()
	for _, keycap := range caps {
		keycap.AddCssClass("mode-legend-flash")
	}
	return true
}

func (v *gtkModeToasterView) clearFlash() {
	for _, caps := range v.keycaps {
		for _, keycap := range caps {
			keycap.RemoveCssClass("mode-legend-flash")
		}
	}
}

// setBodyVisible shows or hides the legend body. Showing waits for the first
// successful measurement.
func (v *gtkModeToasterView) setBodyVisible(visible bool) {
	if visible == v.bodyWanted {
		return
	}
	v.bodyWanted = visible
	if !visible {
		v.hideBody()
		return
	}
	if !v.lingering {
		v.startRefresh()
	}
}

func (v *gtkModeToasterView) hideBody() {
	v.stopRefresh()
	v.placement = nil
	v.root.SetVisible(false)
}

func (v *gtkModeToasterView) lingerRect() (x, y, width, height int, ok bool) {
	if v.placement == nil {
		return 0, 0, 0, 0, false
	}
	return v.placement()
}

func (v *gtkModeToasterView) scrollLegend(delta float64) bool {
	adj := v.scroller.GetVadjustment()
	if adj == nil || adj.GetUpper() <= adj.GetPageSize() {
		return false
	}
	adj.SetValue(adj.GetValue() + delta)
	return true
}

func (v *gtkModeToasterView) stopRefresh() {
	if v.refreshSource != 0 {
		glib.SourceRemove(v.refreshSource)
		v.refreshSource = 0
	}
}

// startRefresh measures outside GTK's allocation pass while the legend is
// shown, which also follows window resizes after the initial layout settles.
func (v *gtkModeToasterView) startRefresh() {
	v.stopRefresh()
	v.refreshGeometry()
	cb := glib.SourceFunc(func(_ uintptr) bool {
		if v.root == nil || !v.bodyWanted || v.lingering {
			v.refreshSource = 0
			return false
		}
		v.refreshGeometry()
		return true
	})
	v.refreshSource = glib.TimeoutAdd(legendRefreshMs, &cb, 0)
}

// refreshGeometry places the legend above the pill, from the pill's left edge,
// as wide as the window allows up to legendMaxWidth.
func (v *gtkModeToasterView) refreshGeometry() {
	if v.root == nil || v.overlay == nil || !v.bodyWanted || v.lingering {
		return
	}
	width, height := v.overlay.GetAllocatedWidth(), v.overlay.GetAllocatedHeight()
	if width <= 0 || height <= 0 || len(v.groups) == 0 {
		v.clearPlacement()
		return
	}
	left, bottom := modeToasterMargin, height-modeToasterMargin
	if v.pillShown {
		px, py, _, _, ok := v.pillBounds()
		if !ok {
			// The pill has not been allocated yet; the next tick retries.
			v.clearPlacement()
			return
		}
		left, bottom = px, py-legendGap
	}
	bandWidth, columns := legendBandWidth(width-left-modeToasterMargin, len(v.groups))
	if bandWidth <= 0 {
		v.clearPlacement()
		return
	}
	if columns != v.lastColumns {
		v.layoutColumns(columns)
	}
	if bandWidth != v.lastWidth {
		v.panel.SetSizeRequest(bandWidth, -1)
		v.lastWidth = bandWidth
	}
	if maxContent := legendBodyMaxHeight(height); maxContent != v.lastMaxContent {
		v.scroller.SetMaxContentHeight(maxContent)
		v.lastMaxContent = maxContent
	}
	// Measure the panel: GTK reports 0 for the hidden root.
	naturalHeight := 0
	v.panel.Measure(gtk.OrientationVerticalValue, bandWidth, nil, &naturalHeight, nil, nil)
	if naturalHeight <= 0 {
		v.clearPlacement()
		return
	}
	y := max(bottom-naturalHeight, 0)
	previous := v.placement
	v.placement = func() (int, int, int, int, bool) { return left, y, bandWidth, naturalHeight, true }
	v.root.SetVisible(true)
	if previous != nil {
		if ox, oy, ow, oh, _ := previous(); ox == left && oy == y && ow == bandWidth && oh == naturalHeight {
			return
		}
	}
	v.root.QueueAllocate()
}

func (v *gtkModeToasterView) clearPlacement() {
	if v.placement == nil {
		return
	}
	v.placement = nil
	v.root.SetVisible(false)
	v.root.QueueAllocate()
}

func (v *gtkModeToasterView) destroy() {
	if v == nil || v.overlay == nil {
		return
	}
	v.stopRefresh()
	v.overlay.RemoveController(&v.motion.EventController)
	v.window.RemoveController(&v.keys.EventController)
	v.window.RemoveController(&v.windowClick.EventController)
	if v.positionHandlerID != 0 {
		gobject.SignalHandlerDisconnect(gobject.ObjectNewFromInternalPtr(v.overlay.GoPointer()), v.positionHandlerID)
		v.positionHandlerID = 0
	}
	v.placement = nil
	v.overlay.RemoveOverlay(&v.pill.Widget)
	v.overlay.RemoveOverlay(&v.root.Widget)
	v.overlay, v.window = nil, nil
	v.pill, v.root = nil, nil
}

// newModeToaster builds the mode toaster of a window inside its content
// overlay. collapsed is the configured initial state of the legend; onToggle
// receives the state a click on the pill asks for.
func newModeToaster(overlay *gtk.Overlay, window *gtk.ApplicationWindow, collapsed bool, onToggle func(bool)) *modeToaster {
	t := newModeToasterWithView(nil, onToggle)
	t.collapsed = collapsed
	t.cfg.ModeLegendCollapsed = collapsed
	view := newGTKModeToasterView(overlay, window, modeToasterEvents{
		pillClicked:    t.pillClicked,
		pointerMoved:   t.pointerMoved,
		pointerLeft:    t.pointerLeft,
		keyPressed:     t.keyPressed,
		pressedOutside: t.pressStarted,
	})
	if view == nil {
		return nil
	}
	t.view = view
	return t
}
