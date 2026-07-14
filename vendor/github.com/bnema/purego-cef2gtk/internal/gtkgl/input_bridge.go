package gtkgl

import (
	"math"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"unicode"
	"unicode/utf16"

	"github.com/bnema/purego-cef/cef"
	internalprofile "github.com/bnema/purego-cef2gtk/internal/profile"
	"github.com/bnema/puregotk/v4/gdk"
	"github.com/bnema/puregotk/v4/gio"
	"github.com/bnema/puregotk/v4/gobject"
	"github.com/bnema/puregotk/v4/gtk"
)

const (
	gdkDeadKeyStart = 0xfe50
	gdkDeadKeyEnd   = 0xfe8c

	maxBMPCodepoint     = 0xFFFF
	minPrintable        = 0x20
	maxSingleByteKeyval = 0x100
)

// InputBridge translates GTK/GDK input events from a GTK widget into CEF OSR input.
type controllerBinding struct {
	controller *gtk.EventController
	handlers   []uint
}

type InputBridge struct {
	mu    sync.Mutex
	host  cef.BrowserHost
	scale float64

	lastX, lastY float64
	clipboard    *gdk.Clipboard
	imContext    *gtk.IMContextSimple
	detached     bool

	widget                 *gtk.Widget
	controllers            []controllerBinding
	callbacks              []any
	imContextCommitHandler uint

	onMiddleClick       func(x, y float64) bool
	middleClickConsumed bool
	scrollOptions       ScrollOptions
	onScroll            func(ScrollEvent) ScrollDecision
	navigationSwipe     navigationSwipeState
	selectionText       func() string
	onClipboardShortcut func(action, text string)
	profiler            atomic.Pointer[internalprofile.Recorder]
}

// ScrollPhase identifies the stage of a GTK scroll operation.
type ScrollPhase int

const (
	ScrollPhaseBegin ScrollPhase = iota
	ScrollPhaseUpdate
	ScrollPhaseEnd
	ScrollPhaseDecelerate
)

// ScrollDecision controls whether a scroll event should be forwarded to CEF.
type ScrollDecision int

const (
	ScrollForwardToCEF ScrollDecision = iota
	ScrollConsume
)

// ScrollOptions configures GTK scroll delta translation before forwarding to CEF.
// Wheel zero values keep legacy wheel behavior; precise zero values use a
// WebKitGTK-like touchpad/surface scale.
type ScrollOptions struct {
	WheelMultiplier      float64
	PreciseMultiplier    float64
	HorizontalMultiplier float64
	VerticalMultiplier   float64
	MaxDelta             int32
}

// ScrollEvent describes a GTK scroll event after CEF delta translation.
type ScrollEvent struct {
	Phase                ScrollPhase
	X, Y                 float64
	DX, DY               float64
	DeltaX, DeltaY       int32
	Modifiers            uint
	Unit                 gdk.ScrollUnit
	UnitKnown            bool
	VelocityX, VelocityY float64
}

// NavigationSwipeAction identifies a browser-history swipe action derived
// from precise horizontal touchpad scrolling.
type NavigationSwipeAction int

const (
	NavigationSwipeBack NavigationSwipeAction = iota
	NavigationSwipeForward
)

// NavigationSwipeOptions configures WebKitGTK-like back/forward swipe recognition.
type NavigationSwipeOptions struct {
	Enabled          bool
	MinDelta         float64
	MaxVerticalRatio float64
}

type navigationSwipeState struct {
	options            NavigationSwipeOptions
	canNavigateBack    func() bool
	canNavigateForward func() bool
	onNavigate         func(NavigationSwipeAction)
	cumulativeDX       float64
	cumulativeDY       float64
	recognized         bool
	verticalCanceled   bool
}

// NewInputBridge creates an input bridge. Scale values <= 0 are treated as 1.
func NewInputBridge(host cef.BrowserHost, scale float64) *InputBridge {
	return &InputBridge{host: host, scale: normalizeScale(scale)}
}

// SetScale updates the device scale used for pointer coordinate translation.
func (ib *InputBridge) SetScale(scale float64) {
	if ib == nil {
		return
	}
	ib.mu.Lock()
	ib.scale = normalizeScale(scale)
	ib.mu.Unlock()
}

// SetMiddleClickHandler configures a callback for middle-button press events.
// If the callback returns true, the press/release pair is consumed locally and
// is not forwarded to CEF.
func (ib *InputBridge) SetMiddleClickHandler(fn func(x, y float64) bool) {
	if ib == nil {
		return
	}
	ib.mu.Lock()
	ib.onMiddleClick = fn
	ib.mu.Unlock()
}

// SetScrollOptions configures scroll translation and an optional interception
// callback. If the callback returns ScrollConsume, the update event is not
// forwarded to CEF.
func (ib *InputBridge) SetScrollOptions(opts ScrollOptions, fn func(ScrollEvent) ScrollDecision) {
	if ib == nil {
		return
	}
	ib.mu.Lock()
	ib.scrollOptions = opts
	ib.onScroll = fn
	ib.mu.Unlock()
}

// SetNavigationSwipeHandler configures browser-history navigation recognition
// from precise horizontal touchpad scroll streams.
func (ib *InputBridge) SetNavigationSwipeHandler(opts NavigationSwipeOptions, canBack, canForward func() bool, onNavigate func(NavigationSwipeAction)) {
	if ib == nil {
		return
	}
	ib.mu.Lock()
	ib.navigationSwipe = navigationSwipeState{
		options:            opts,
		canNavigateBack:    canBack,
		canNavigateForward: canForward,
		onNavigate:         onNavigate,
	}
	ib.mu.Unlock()
}

// SetClipboardShortcutHandler configures callbacks used to mirror explicit
// Ctrl+C/Ctrl+X shortcuts to application-level clipboard orchestration.
func (ib *InputBridge) SetProfiler(profiler *internalprofile.Recorder) {
	if ib == nil {
		return
	}
	ib.profiler.Store(profiler)
}

func (ib *InputBridge) SetClipboardShortcutHandler(selectionText func() string, onShortcut func(action, text string)) {
	if ib == nil {
		return
	}
	ib.mu.Lock()
	ib.selectionText = selectionText
	ib.onClipboardShortcut = onShortcut
	ib.mu.Unlock()
}

// SetHost updates the CEF browser host used for subsequent input dispatch.
func (ib *InputBridge) SetHost(host cef.BrowserHost) {
	if ib == nil {
		return
	}
	ib.mu.Lock()
	ib.host = host
	ib.mu.Unlock()
}

// Attach creates GTK event controllers and attaches them to the GLArea.
func (ib *InputBridge) Attach(area *gtk.GLArea) {
	if area == nil {
		return
	}
	ib.AttachToWidget(&area.Widget)
}

// AttachToWidget creates GTK event controllers and attaches them to widget.
func (ib *InputBridge) AttachToWidget(widget *gtk.Widget) {
	if ib == nil || widget == nil {
		return
	}
	ib.mu.Lock()
	ib.widget = widget
	ib.detached = false
	if display := gdk.DisplayGetDefault(); display != nil {
		ib.clipboard = display.GetClipboard()
	}
	ib.mu.Unlock()

	motion := gtk.NewEventControllerMotion()
	motionCb := func(g gtk.EventControllerMotion, x, y float64) {
		ib.onMouseMove(x, y, uint(g.GetCurrentEventState()), false)
	}
	motionHandlerID := motion.ConnectMotion(&motionCb)
	leaveCb := func(g gtk.EventControllerMotion) {
		ib.onMouseMove(0, 0, uint(g.GetCurrentEventState()), true)
	}
	leaveHandlerID := motion.ConnectLeave(&leaveCb)
	ib.addController(widget, &motion.EventController, []uint{motionHandlerID, leaveHandlerID}, &motionCb, &leaveCb)

	click := gtk.NewGestureClick()
	click.SetButton(0)
	pressedCb := func(g gtk.GestureClick, nPress int, x, y float64) {
		widget.GrabFocus()
		ib.onMousePress(x, y, g.GetCurrentButton(), uint(g.GetCurrentEventState()), nPress)
	}
	pressedHandlerID := click.ConnectPressed(&pressedCb)
	releasedCb := func(g gtk.GestureClick, nPress int, x, y float64) {
		ib.onMouseRelease(x, y, g.GetCurrentButton(), uint(g.GetCurrentEventState()), nPress)
	}
	releasedHandlerID := click.ConnectReleased(&releasedCb)
	ib.addController(widget, &click.EventController, []uint{pressedHandlerID, releasedHandlerID}, &pressedCb, &releasedCb)

	scroll := gtk.NewEventControllerScroll(gtk.EventControllerScrollBothAxesValue | gtk.EventControllerScrollKineticValue)
	scrollBeginCb := func(g gtk.EventControllerScroll) {
		unit, unitKnown := currentScrollUnit(g)
		ib.onScrollBoundary(ScrollPhaseBegin, unit, unitKnown, uint(g.GetCurrentEventState()))
	}
	scrollBeginHandlerID := scroll.ConnectScrollBegin(&scrollBeginCb)
	scrollCb := func(g gtk.EventControllerScroll, dx, dy float64) bool {
		unit, unitKnown := currentScrollUnit(g)
		ib.onScrollUpdate(dx, dy, unit, unitKnown, uint(g.GetCurrentEventState()))
		return true
	}
	scrollHandlerID := scroll.ConnectScroll(&scrollCb)
	scrollEndCb := func(g gtk.EventControllerScroll) {
		unit, unitKnown := currentScrollUnit(g)
		ib.onScrollBoundary(ScrollPhaseEnd, unit, unitKnown, uint(g.GetCurrentEventState()))
	}
	scrollEndHandlerID := scroll.ConnectScrollEnd(&scrollEndCb)
	scrollDecelerateCb := func(g gtk.EventControllerScroll, velocityX, velocityY float64) {
		unit, unitKnown := currentScrollUnit(g)
		ib.onScrollDecelerate(velocityX, velocityY, unit, unitKnown, uint(g.GetCurrentEventState()))
	}
	scrollDecelerateHandlerID := scroll.ConnectDecelerate(&scrollDecelerateCb)
	ib.addController(widget, &scroll.EventController, []uint{scrollBeginHandlerID, scrollHandlerID, scrollEndHandlerID, scrollDecelerateHandlerID}, &scrollBeginCb, &scrollCb, &scrollEndCb, &scrollDecelerateCb)

	focus := gtk.NewEventControllerFocus()
	focusEnterCb := func(_ gtk.EventControllerFocus) {
		if ib.imContext != nil {
			ib.imContext.FocusIn()
		}
		ib.onFocusIn()
	}
	focusEnterHandlerID := focus.ConnectEnter(&focusEnterCb)
	focusLeaveCb := func(_ gtk.EventControllerFocus) {
		if ib.imContext != nil {
			ib.imContext.Reset()
			ib.imContext.FocusOut()
		}
		ib.onFocusOut()
	}
	focusLeaveHandlerID := focus.ConnectLeave(&focusLeaveCb)
	ib.addController(widget, &focus.EventController, []uint{focusEnterHandlerID, focusLeaveHandlerID}, &focusEnterCb, &focusLeaveCb)

	key := gtk.NewEventControllerKey()
	ib.imContext = gtk.NewIMContextSimple()
	if ib.imContext != nil {
		commitCb := func(_ gtk.IMContext, text string) { ib.onIMCommit(text) }
		commitHandlerID := ib.imContext.ConnectCommit(&commitCb)
		key.SetImContext(&ib.imContext.IMContext)
		ib.imContext.SetClientWidget(widget)
		ib.callbacks = append(ib.callbacks, &commitCb)
		ib.imContextCommitHandler = commitHandlerID
	}
	keyPressCb := func(_ gtk.EventControllerKey, keyval, keycode uint, state gdk.ModifierType) bool {
		mods := uint(state)
		ib.mirrorClipboardShortcut(keyval, mods)
		if mods&(uint(gdk.ControlMaskValue)|uint(gdk.MetaMaskValue)) != 0 && (keyval == gdkKeyLowercaseV || keyval == gdkKeyUppercaseV) {
			ib.pasteFromClipboard()
			return true
		}
		ib.onKeyPress(keyval, keycode, mods)
		if mods&uint(gdk.ControlMaskValue) != 0 || mods&uint(gdk.AltMaskValue) != 0 {
			return false
		}
		if keyval >= gdkKeyF1Start && keyval <= gdkKeyF12End || keyval == gdkKeyEscape {
			return false
		}
		return true
	}
	keyPressHandlerID := key.ConnectKeyPressed(&keyPressCb)
	keyReleaseCb := func(_ gtk.EventControllerKey, keyval, keycode uint, state gdk.ModifierType) {
		ib.onKeyRelease(keyval, keycode, uint(state))
	}
	keyReleaseHandlerID := key.ConnectKeyReleased(&keyReleaseCb)
	ib.addController(widget, &key.EventController, []uint{keyPressHandlerID, keyReleaseHandlerID}, &keyPressCb, &keyReleaseCb)

	widget.SetFocusable(true)
	widget.SetCanFocus(true)
}

func (ib *InputBridge) addController(widget *gtk.Widget, controller *gtk.EventController, handlers []uint, callbacks ...any) {
	widget.AddController(controller)
	ib.mu.Lock()
	ib.controllers = append(ib.controllers, controllerBinding{controller: controller, handlers: handlers})
	ib.callbacks = append(ib.callbacks, callbacks...)
	ib.mu.Unlock()
}

// Detach removes controllers previously attached by this bridge.
func (ib *InputBridge) Detach() {
	if ib == nil {
		return
	}
	ib.mu.Lock()
	widget := ib.widget
	controllers := append([]controllerBinding(nil), ib.controllers...)
	imContext := ib.imContext
	imContextCommitHandler := ib.imContextCommitHandler
	ib.detached = true
	ib.controllers = nil
	ib.callbacks = nil
	ib.imContext = nil
	ib.imContextCommitHandler = 0
	ib.widget = nil
	ib.clipboard = nil
	ib.mu.Unlock()
	if imContext != nil && imContextCommitHandler != 0 {
		gobject.SignalHandlerDisconnect(&imContext.Object, imContextCommitHandler)
	}
	if widget == nil {
		return
	}
	for _, binding := range controllers {
		controller := binding.controller
		if controller == nil {
			continue
		}
		for _, handlerID := range binding.handlers {
			if handlerID != 0 {
				gobject.SignalHandlerDisconnect(&controller.Object, handlerID)
			}
		}
		widget.RemoveController(controller)
	}
}

func (ib *InputBridge) currentHost() cef.BrowserHost {
	ib.mu.Lock()
	defer ib.mu.Unlock()
	return ib.host
}

func (ib *InputBridge) currentHostAndMiddleClickHandler() (cef.BrowserHost, float64, func(x, y float64) bool) {
	ib.mu.Lock()
	defer ib.mu.Unlock()
	return ib.host, ib.scale, ib.onMiddleClick
}

func (ib *InputBridge) currentClipboardShortcutHandlers() (func() string, func(action, text string)) {
	ib.mu.Lock()
	defer ib.mu.Unlock()
	return ib.selectionText, ib.onClipboardShortcut
}

func (ib *InputBridge) setMiddleClickConsumed(consumed bool) {
	ib.mu.Lock()
	ib.middleClickConsumed = consumed
	ib.mu.Unlock()
}

func (ib *InputBridge) consumeMiddleClickRelease() bool {
	ib.mu.Lock()
	defer ib.mu.Unlock()
	consumed := ib.middleClickConsumed
	ib.middleClickConsumed = false
	return consumed
}

func (ib *InputBridge) onMouseMove(x, y float64, mods uint, leave bool) {
	ib.mu.Lock()
	host, scale := ib.host, ib.scale
	if !leave {
		ib.lastX, ib.lastY = x, y
	}
	ib.mu.Unlock()
	if host == nil {
		return
	}
	evt := BuildMouseEvent(x, y, mods, scale)
	var mouseLeave int32
	if leave {
		mouseLeave = 1
	}
	host.SendMouseMoveEvent(&evt, mouseLeave)
}

func (ib *InputBridge) onMousePress(x, y float64, button, mods uint, clickCount int) {
	host, scale, consumeMiddle := ib.currentHostAndMiddleClickHandler()
	if host == nil {
		return
	}
	if button == 1 {
		syncWindowlessBrowserFocus(host)
	}
	if button == 2 && consumeMiddle != nil && consumeMiddle(x, y) {
		ib.setMiddleClickConsumed(true)
		return
	}
	if button == 2 {
		ib.setMiddleClickConsumed(false)
	}
	evt := BuildMouseEvent(x, y, mods, scale)
	host.SendMouseClickEvent(&evt, TranslateMouseButton(button), 0, int32(clickCount))
}

func (ib *InputBridge) onMouseRelease(x, y float64, button, mods uint, clickCount int) {
	ib.mu.Lock()
	host, scale := ib.host, ib.scale
	ib.mu.Unlock()
	if host == nil {
		return
	}
	if button == 2 && ib.consumeMiddleClickRelease() {
		return
	}
	evt := BuildMouseEvent(x, y, mods, scale)
	host.SendMouseClickEvent(&evt, TranslateMouseButton(button), 1, int32(clickCount))
}

func (ib *InputBridge) currentScrollState() (cef.BrowserHost, float64, float64, float64, ScrollOptions, func(ScrollEvent) ScrollDecision) {
	ib.mu.Lock()
	defer ib.mu.Unlock()
	return ib.host, ib.lastX, ib.lastY, ib.scale, ib.scrollOptions, ib.onScroll
}

func (ib *InputBridge) currentNavigationSwipeState() navigationSwipeState {
	ib.mu.Lock()
	defer ib.mu.Unlock()
	return ib.navigationSwipe
}

func (ib *InputBridge) setNavigationSwipeState(state navigationSwipeState) {
	ib.mu.Lock()
	ib.navigationSwipe = state
	ib.mu.Unlock()
}

func (ib *InputBridge) resetNavigationSwipe() {
	ib.mu.Lock()
	ib.navigationSwipe.cumulativeDX = 0
	ib.navigationSwipe.cumulativeDY = 0
	ib.navigationSwipe.recognized = false
	ib.navigationSwipe.verticalCanceled = false
	ib.mu.Unlock()
}

func (ib *InputBridge) onScrollUpdate(dx, dy float64, unit gdk.ScrollUnit, unitKnown bool, mods uint) {
	host, x, y, scale, opts, handler := ib.currentScrollState()
	if profiler := ib.profiler.Load(); profiler != nil {
		profiler.RecordScroll(dx, dy)
	}
	effectiveUnit := unit
	if !unitKnown {
		effectiveUnit = gdk.ScrollUnitWheelValue
	}
	deltaX, deltaY := TranslateScrollDeltasWithOptions(dx, dy, effectiveUnit, opts)
	event := ScrollEvent{
		Phase:     ScrollPhaseUpdate,
		X:         x,
		Y:         y,
		DX:        dx,
		DY:        dy,
		DeltaX:    deltaX,
		DeltaY:    deltaY,
		Modifiers: mods,
		Unit:      unit,
		UnitKnown: unitKnown,
	}
	if ib.handleNavigationSwipe(event) {
		return
	}
	if handler != nil && handler(event) == ScrollConsume {
		return
	}
	if host == nil {
		return
	}
	evt := BuildMouseEvent(x, y, mods, scale)
	if unitKnown && unit == gdk.ScrollUnitSurfaceValue {
		evt.Modifiers |= uint32(cef.EventFlagsEventflagPrecisionScrollingDelta)
	}
	host.SendMouseWheelEvent(&evt, deltaX, deltaY)
}

func (ib *InputBridge) onScrollBoundary(phase ScrollPhase, unit gdk.ScrollUnit, unitKnown bool, mods uint) {
	_, x, y, _, _, handler := ib.currentScrollState()
	switch phase {
	case ScrollPhaseBegin:
		ib.resetNavigationSwipe()
	case ScrollPhaseEnd:
		ib.finishNavigationSwipe()
		ib.resetNavigationSwipe()
	}
	if handler == nil {
		return
	}
	handler(ScrollEvent{
		Phase:     phase,
		X:         x,
		Y:         y,
		Modifiers: mods,
		Unit:      unit,
		UnitKnown: unitKnown,
	})
}

func (ib *InputBridge) onScrollDecelerate(velocityX, velocityY float64, unit gdk.ScrollUnit, unitKnown bool, mods uint) {
	_, x, y, _, _, handler := ib.currentScrollState()
	if handler == nil {
		return
	}
	handler(ScrollEvent{
		Phase:     ScrollPhaseDecelerate,
		X:         x,
		Y:         y,
		Modifiers: mods,
		Unit:      unit,
		UnitKnown: unitKnown,
		VelocityX: velocityX,
		VelocityY: velocityY,
	})
}

func (ib *InputBridge) handleNavigationSwipe(event ScrollEvent) bool {
	state := ib.currentNavigationSwipeState()
	if !state.options.Enabled || state.onNavigate == nil || !isPreciseScrollEvent(event) {
		return false
	}

	if state.verticalCanceled {
		state.cumulativeDX = 0
		state.cumulativeDY = 0
		ib.setNavigationSwipeState(state)
		return false
	}

	// GTK scroll deltas are inverted compared to WebKit's navigation swipe
	// direction model. Match WebKitGTK's ViewGestureController, which negates
	// scroll deltas before deciding Back vs Forward.
	state.cumulativeDX += -event.DX
	state.cumulativeDY += event.DY
	if navigationSwipeIsTooVertical(state) {
		state.cumulativeDX = 0
		state.cumulativeDY = 0
		state.verticalCanceled = true
	}
	ib.setNavigationSwipeState(state)
	return false
}

func (ib *InputBridge) finishNavigationSwipe() {
	state := ib.currentNavigationSwipeState()
	if !state.options.Enabled || state.onNavigate == nil || state.recognized || state.verticalCanceled {
		return
	}
	absDX := math.Abs(state.cumulativeDX)
	if absDX <= normalizedNavigationSwipeMinDelta(state.options.MinDelta) || navigationSwipeIsTooVertical(state) {
		return
	}
	action, ok := navigationSwipeActionForDelta(state.cumulativeDX, state.canNavigateBack, state.canNavigateForward)
	if !ok {
		return
	}
	state.recognized = true
	ib.setNavigationSwipeState(state)
	state.onNavigate(action)
}

func navigationSwipeIsTooVertical(state navigationSwipeState) bool {
	absDX, absDY := math.Abs(state.cumulativeDX), math.Abs(state.cumulativeDY)
	return absDX == 0 || absDY >= absDX*normalizedNavigationSwipeRatio(state.options.MaxVerticalRatio)
}

func isPreciseScrollEvent(event ScrollEvent) bool {
	return event.UnitKnown && event.Unit == gdk.ScrollUnitSurfaceValue
}

func navigationSwipeActionForDelta(dx float64, canBack, canForward func() bool) (NavigationSwipeAction, bool) {
	if dx > 0 && canBack != nil && canBack() {
		return NavigationSwipeBack, true
	}
	if dx < 0 && canForward != nil && canForward() {
		return NavigationSwipeForward, true
	}
	return NavigationSwipeBack, false
}

const defaultNavigationSwipeCommitDistance = 400 * 0.5

func normalizedNavigationSwipeMinDelta(value float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) || value <= 0 {
		// WebKitGTK tracks touchpad swipe progress as distance / 400 and commits
		// past 0.5 progress. In this bridge MinDelta represents that raw GTK
		// surface-unit commit distance, not translated CEF wheel deltas.
		return defaultNavigationSwipeCommitDistance
	}
	return value
}

func normalizedNavigationSwipeRatio(value float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) || value <= 0 {
		return 0.5
	}
	return value
}

func (ib *InputBridge) onFocusIn() { syncWindowlessBrowserFocus(ib.currentHost()) }
func (ib *InputBridge) onFocusOut() {
	if h := ib.currentHost(); h != nil {
		h.SetFocus(0)
	}
}

func (ib *InputBridge) mirrorClipboardShortcut(keyval, mods uint) {
	action, ok := clipboardShortcutAction(keyval, mods)
	if !ok {
		return
	}
	selectionText, onShortcut := ib.currentClipboardShortcutHandlers()
	if selectionText == nil || onShortcut == nil {
		return
	}
	text := selectionText()
	if text == "" {
		return
	}
	onShortcut(action, text)
}

func clipboardShortcutAction(keyval, mods uint) (string, bool) {
	if mods&uint(gdk.ControlMaskValue) == 0 {
		return "", false
	}
	if mods&uint(gdk.ShiftMaskValue) != 0 || mods&uint(gdk.AltMaskValue) != 0 {
		return "", false
	}
	switch keyval {
	case gdkKeyLowercaseC, gdkKeyUppercaseC:
		return "copy", true
	case gdkKeyLowercaseX, gdkKeyUppercaseX:
		return "cut", true
	default:
		return "", false
	}
}

func syncWindowlessBrowserFocus(host cef.BrowserHost) {
	if host == nil {
		return
	}
	host.WasHidden(0)
	host.SetFocus(1)
	host.Invalidate(cef.PaintElementTypePetView)
}

func (ib *InputBridge) onKeyPress(keyval, keycode, mods uint) {
	host := ib.currentHost()
	if host == nil {
		return
	}
	if keyval >= gdkDeadKeyStart && keyval <= gdkDeadKeyEnd {
		return
	}
	evt := BuildKeyEvent(keyval, keycode, mods, cef.KeyEventTypeKeyeventRawkeydown)
	host.SendKeyEvent(&evt)
	if ch := KeyvalToChar(keyval); ch != 0 {
		charEvt := BuildKeyEvent(keyval, keycode, mods, cef.KeyEventTypeKeyeventChar)
		charEvt.WindowsKeyCode = int32(ch)
		charEvt.Character = ch
		charEvt.UnmodifiedCharacter = ch
		host.SendKeyEvent(&charEvt)
	}
}

func (ib *InputBridge) onIMCommit(text string) {
	host := ib.currentHost()
	if host == nil {
		return
	}
	for _, r := range text {
		if r > maxBMPCodepoint {
			hi, lo := utf16.EncodeRune(r)
			if hi == unicode.ReplacementChar || lo == unicode.ReplacementChar {
				continue
			}
			ib.sendChar(host, uint16(hi))
			ib.sendChar(host, uint16(lo))
			continue
		}
		ib.sendChar(host, uint16(r))
	}
}

func (ib *InputBridge) sendChar(host cef.BrowserHost, ch uint16) {
	evt := cef.NewKeyEvent()
	evt.Type = cef.KeyEventTypeKeyeventChar
	evt.WindowsKeyCode = int32(ch)
	evt.Character = ch
	evt.UnmodifiedCharacter = ch
	host.SendKeyEvent(&evt)
}

func (ib *InputBridge) onKeyRelease(keyval, keycode, mods uint) {
	host := ib.currentHost()
	if host == nil {
		return
	}
	evt := BuildKeyEvent(keyval, keycode, mods, cef.KeyEventTypeKeyeventKeyup)
	host.SendKeyEvent(&evt)
}

func (ib *InputBridge) pasteFromClipboard() {
	ib.mu.Lock()
	cb := ib.clipboard
	detached := ib.detached
	ib.mu.Unlock()
	if detached || cb == nil {
		return
	}
	asyncCb := gio.AsyncReadyCallback(func(_, resultPtr, _ uintptr) {
		text, err := cb.ReadTextFinish(&gio.AsyncResultBase{Ptr: resultPtr})
		if err != nil || text == "" {
			return
		}
		ib.mu.Lock()
		detached := ib.detached
		host := ib.host
		ib.mu.Unlock()
		if detached || host == nil {
			return
		}
		browser := host.GetBrowser()
		if browser == nil {
			return
		}
		frame := browser.GetMainFrame()
		if frame == nil {
			return
		}
		frame.ExecuteJavaScript(pasteJavaScript(text), "", 0)
	})
	cb.ReadTextAsync(nil, &asyncCb, 0)
}

func pasteJavaScript(text string) string {
	quoted := jsString(text)
	return `(function(text){
const active=document.activeElement;
if(active && (active.tagName==='TEXTAREA' || (active.tagName==='INPUT' && 'value' in active))){
  const start=active.selectionStart ?? active.value.length;
  const end=active.selectionEnd ?? start;
  active.value=active.value.slice(0,start)+text+active.value.slice(end);
  const next=start+text.length;
  active.selectionStart=next; active.selectionEnd=next;
  active.dispatchEvent(new InputEvent('input',{bubbles:true,inputType:'insertText',data:text}));
  return;
}
const selection=window.getSelection && window.getSelection();
if(selection && selection.rangeCount){
  const range=selection.getRangeAt(0);
  range.deleteContents();
  const node=document.createTextNode(text);
  range.insertNode(node);
  range.setStartAfter(node); range.collapse(true);
  selection.removeAllRanges(); selection.addRange(range);
  return;
}
// document.execCommand is deprecated but retained as a compatibility fallback
// for older/locked-down CEF pages where Selection/Range insertion is unavailable.
if(document.execCommand){ document.execCommand('insertText',false,text); }
})(` + quoted + `);`
}

func jsString(s string) string {
	q := strconv.Quote(s)
	return strings.ReplaceAll(q, "</", "<\\/")
}

func BuildMouseEvent(x, y float64, gdkMods uint, scale float64) cef.MouseEvent {
	scale = normalizeScale(scale)
	return cef.MouseEvent{
		X:         logicalToDeviceCoord(x, scale),
		Y:         logicalToDeviceCoord(y, scale),
		Modifiers: TranslateModifiers(gdkMods),
	}
}

func logicalToDeviceCoord(value float64, scale float64) int32 {
	return int32(math.Floor(value * normalizeScale(scale)))
}

func normalizeScale(scale float64) float64 {
	if math.IsNaN(scale) || math.IsInf(scale, 0) || scale <= 0 {
		return 1
	}
	return scale
}

func BuildKeyEvent(keyval, keycode, gdkMods uint, eventType cef.KeyEventType) cef.KeyEvent {
	evt := cef.NewKeyEvent()
	evt.Type = eventType
	evt.WindowsKeyCode = GDKKeyvalToWindowsVK(keyval)
	evt.NativeKeyCode = int32(keycode)
	evt.Modifiers = TranslateModifiers(gdkMods)
	return evt
}

func TranslateModifiers(gdkState uint) uint32 {
	var flags uint32
	if gdkState&uint(gdk.ShiftMaskValue) != 0 {
		flags |= uint32(cef.EventFlagsEventflagShiftDown)
	}
	if gdkState&uint(gdk.ControlMaskValue) != 0 {
		flags |= uint32(cef.EventFlagsEventflagControlDown)
	}
	if gdkState&uint(gdk.AltMaskValue) != 0 {
		flags |= uint32(cef.EventFlagsEventflagAltDown)
	}
	if gdkState&uint(gdk.Button1MaskValue) != 0 {
		flags |= uint32(cef.EventFlagsEventflagLeftMouseButton)
	}
	if gdkState&uint(gdk.Button2MaskValue) != 0 {
		flags |= uint32(cef.EventFlagsEventflagMiddleMouseButton)
	}
	if gdkState&uint(gdk.Button3MaskValue) != 0 {
		flags |= uint32(cef.EventFlagsEventflagRightMouseButton)
	}
	return flags
}

func TranslateMouseButton(gdkButton uint) cef.MouseButtonType {
	switch gdkButton {
	case 1:
		return cef.MouseButtonTypeMbtLeft
	case 2:
		return cef.MouseButtonTypeMbtMiddle
	case 3:
		return cef.MouseButtonTypeMbtRight
	default:
		return cef.MouseButtonTypeMbtLeft
	}
}

const cefScrollUnitsPerNotch = 240

func TranslateScrollDeltas(dx, dy float64) (int32, int32) {
	return int32(dx * cefScrollUnitsPerNotch), int32(-dy * cefScrollUnitsPerNotch)
}

func TranslateScrollDeltasWithOptions(dx, dy float64, unit gdk.ScrollUnit, opts ScrollOptions) (int32, int32) {
	multiplier := normalizeMultiplier(opts.WheelMultiplier)
	unitScale := float64(cefScrollUnitsPerNotch)
	round := false
	if unit == gdk.ScrollUnitSurfaceValue {
		multiplier = normalizePreciseMultiplier(opts.PreciseMultiplier)
		unitScale = 1
		round = true
	}
	horizontal := normalizeMultiplier(opts.HorizontalMultiplier)
	vertical := normalizeMultiplier(opts.VerticalMultiplier)
	deltaX := clampScrollDelta(dx*unitScale*multiplier*horizontal, opts.MaxDelta, round)
	deltaY := clampScrollDelta(-dy*unitScale*multiplier*vertical, opts.MaxDelta, round)
	return deltaX, deltaY
}

func normalizeMultiplier(value float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) || value <= 0 {
		return 1
	}
	return value
}

func normalizePreciseMultiplier(value float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) || value <= 0 {
		return 2.5
	}
	return value
}

func clampScrollDelta(value float64, maxAbs int32, round bool) int32 {
	limit := maxInt32Float
	if maxAbs > 0 {
		limit = float64(maxAbs)
	}
	if value > limit {
		value = limit
	}
	if value < -limit {
		value = -limit
	}
	if round {
		value = math.Round(value)
	}
	return int32(value)
}

func currentScrollUnit(controller gtk.EventControllerScroll) (gdk.ScrollUnit, bool) {
	// Do not call GtkEventController.GetCurrentEvent here. The current puregotk
	// binding treats the returned GdkEvent as a GObject and refs it with
	// g_object_ref_sink(), but GdkEvent is a boxed type. That produces a GLib
	// assertion on every scroll event. GtkEventControllerScroll.GetUnit() exposes
	// the scroll unit we need without wrapping the current event.
	return controller.GetUnit(), true
}

const maxInt32Float = float64(int32(1<<31 - 1))

const (
	gdkKeyReturn          = 0xff0d
	gdkKeyTab             = 0xff09
	gdkKeyBackSpace       = 0xff08
	gdkKeyEscape          = 0xff1b
	gdkKeyDelete          = 0xffff
	gdkKeySpace           = 0x020
	gdkKeyHome            = 0xff50
	gdkKeyEnd             = 0xff57
	gdkKeyPageUp          = 0xff55
	gdkKeyPageDown        = 0xff56
	gdkKeyLowercaseC      = 0x063
	gdkKeyUppercaseC      = 0x043
	gdkKeyLowercaseV      = 0x076
	gdkKeyUppercaseV      = 0x056
	gdkKeyLowercaseX      = 0x078
	gdkKeyUppercaseX      = 0x058
	gdkKeyLowercaseAStart = 0x061
	gdkKeyLowercaseAEnd   = 0x07a
	gdkKeyUppercaseAStart = 0x041
	gdkKeyUppercaseAEnd   = 0x05a
	gdkKeyDigit0Start     = 0x030
	gdkKeyDigit9End       = 0x039
	gdkKeyF1Start         = 0xffbe
	gdkKeyF12End          = 0xffc9
	gdkKeyArrowStart      = 0xff51
	gdkKeyArrowEnd        = 0xff54
	vkA                   = 0x41
	vkF1                  = 0x70
	vkArrowLeft           = 0x25
)

func KeyvalToChar(keyval uint) uint16 {
	switch keyval {
	case gdkKeyReturn:
		return '\r'
	case gdkKeyTab:
		return '\t'
	case gdkKeyBackSpace:
		return '\b'
	}
	cp := gdk.KeyvalToUnicode(keyval)
	if cp == 0 || cp > maxBMPCodepoint || cp < minPrintable {
		return 0
	}
	return uint16(cp)
}

func GDKKeyvalToWindowsVK(keyval uint) int32 {
	if vk, ok := gdkKeyvalToVKRange(keyval); ok {
		return vk
	}
	if vk, ok := gdkKeyvalToVKMap[keyval]; ok {
		return vk
	}
	if keyval < maxSingleByteKeyval {
		return int32(keyval)
	}
	return 0
}

func gdkKeyvalToVKRange(keyval uint) (int32, bool) {
	switch {
	case keyval >= gdkKeyLowercaseAStart && keyval <= gdkKeyLowercaseAEnd:
		return int32(keyval-gdkKeyLowercaseAStart) + vkA, true
	case keyval >= gdkKeyUppercaseAStart && keyval <= gdkKeyUppercaseAEnd:
		return int32(keyval-gdkKeyUppercaseAStart) + vkA, true
	case keyval >= gdkKeyDigit0Start && keyval <= gdkKeyDigit9End:
		return int32(keyval), true
	case keyval >= gdkKeyF1Start && keyval <= gdkKeyF12End:
		return int32(keyval-gdkKeyF1Start) + vkF1, true
	case keyval >= gdkKeyArrowStart && keyval <= gdkKeyArrowEnd:
		return int32(keyval-gdkKeyArrowStart) + vkArrowLeft, true
	default:
		return 0, false
	}
}

var gdkKeyvalToVKMap = map[uint]int32{
	gdkKeyReturn: 0x0D, gdkKeyEscape: 0x1B, gdkKeyTab: 0x09, gdkKeyBackSpace: 0x08, gdkKeyDelete: 0x2E, gdkKeySpace: 0x20, gdkKeyHome: 0x24, gdkKeyEnd: 0x23, gdkKeyPageUp: 0x21, gdkKeyPageDown: 0x22, 0xff63: 0x2D,
	0xffe1: 0xA0, 0xffe2: 0xA1, 0xffe3: 0xA2, 0xffe4: 0xA3, 0xffe9: 0xA4, 0xffea: 0xA5,
	'.': 0xBE, '>': 0xBE, ',': 0xBC, '<': 0xBC, '-': 0xBD, '_': 0xBD, '=': 0xBB, '+': 0xBB, ';': 0xBA, ':': 0xBA, '/': 0xBF, '?': 0xBF, '`': 0xC0, '~': 0xC0, '[': 0xDB, '{': 0xDB, '\\': 0xDC, '|': 0xDC, ']': 0xDD, '}': 0xDD, '\'': 0xDE, '"': 0xDE,
	'!': 0x31, '@': 0x32, '#': 0x33, '$': 0x34, '%': 0x35, '^': 0x36, '&': 0x37, '*': 0x38, '(': 0x39, ')': 0x30,
}
