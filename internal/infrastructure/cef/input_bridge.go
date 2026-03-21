package cef

import (
	"sync"

	purecef "github.com/bnema/purego-cef/cef"
	"github.com/bnema/puregotk/v4/gdk"
	"github.com/bnema/puregotk/v4/gtk"
)

// GDK dead key range boundaries. Dead keys (0xfe50–0xfe8c) are compose
// modifiers (dead_grave, dead_acute, …) that should not generate CEF key
// events by themselves — the IMContext absorbs them and emits a commit
// with the composed character.
const (
	gdkDeadKeyStart = 0xfe50
	gdkDeadKeyEnd   = 0xfe8c

	// maxBMPCodepoint is the highest Unicode codepoint in the Basic
	// Multilingual Plane that fits in a single uint16 / UTF-16 code unit.
	maxBMPCodepoint = 0xFFFF
	// minPrintable is the lowest printable Unicode codepoint (space).
	minPrintable = 0x20
	// maxSingleByteKeyval is the upper bound for GDK keyvals that can be
	// used directly as Windows VK codes (when no explicit mapping exists).
	maxSingleByteKeyval = 0x100
)

// inputBridge translates GDK events from GTK event controllers into CEF input
// events and forwards them to the CEF BrowserHost. The host field is nil until
// the browser is created asynchronously (OnAfterCreated), so all dispatch
// methods guard against a nil host.
//
// Dead key / compose support: a GtkIMContextSimple is attached to the key
// controller. When the user types a compose sequence (e.g. dead_acute + e),
// the IMContext absorbs the individual key events and emits a "commit" signal
// with the composed string ("é"). The bridge sends that as CHAR events to CEF.
// Keys not consumed by the IMContext flow through key-pressed as before.
type inputBridge struct {
	host  purecef.BrowserHost
	scale int32
	mu    sync.Mutex

	// Last known pointer position, used for scroll events which don't
	// carry their own coordinates from GDK.
	lastX, lastY float64

	// IMContext for dead key / compose sequence support. Held as a field
	// to prevent garbage collection while the controller is alive.
	imContext *gtk.IMContextSimple
	// Prevent GC from collecting signal callbacks.
	commitCb func(gtk.IMContext, string)

	// onMiddleClick is called when button 2 (middle) is pressed on a link.
	// The callback receives the hovered URI. Set by the factory.
	onMiddleClick func(uri string)
}

// newInputBridge creates an input bridge with the given HiDPI scale factor.
func newInputBridge(scale int32) *inputBridge {
	return &inputBridge{scale: scale}
}

// setHost is called once the browser is created to enable event dispatch.
func (ib *inputBridge) setHost(host purecef.BrowserHost) {
	ib.mu.Lock()
	defer ib.mu.Unlock()
	ib.host = host
}

// attachTo creates GDK event controllers and connects them to the given GLArea.
func (ib *inputBridge) attachTo(glArea *gtk.GLArea) {
	// Motion (mouse move + enter/leave)
	motion := gtk.NewEventControllerMotion()
	motionCb := func(_ gtk.EventControllerMotion, x, y float64) {
		ib.onMouseMove(x, y, 0, false)
	}
	motion.ConnectMotion(&motionCb)

	leaveCb := func(_ gtk.EventControllerMotion) {
		ib.onMouseMove(0, 0, 0, true)
	}
	motion.ConnectLeave(&leaveCb)

	glArea.AddController(&motion.EventController)

	// Click (press / release)
	click := gtk.NewGestureClick()
	click.SetButton(0) // listen to all buttons

	pressedCb := func(g gtk.GestureClick, nPress int, x, y float64) {
		btn := g.GetCurrentButton()
		mods := uint(g.GetCurrentEventState())
		ib.onMousePress(x, y, btn, mods, nPress)
	}
	click.ConnectPressed(&pressedCb)

	releasedCb := func(g gtk.GestureClick, nPress int, x, y float64) {
		btn := g.GetCurrentButton()
		mods := uint(g.GetCurrentEventState())
		ib.onMouseRelease(x, y, btn, mods, nPress)
	}
	click.ConnectReleased(&releasedCb)

	glArea.AddController(&click.EventController)

	// Scroll
	scroll := gtk.NewEventControllerScroll(gtk.EventControllerScrollBothAxesValue)
	scrollCb := func(_ gtk.EventControllerScroll, dx, dy float64) bool {
		ib.onScroll(dx, dy)
		return true
	}
	scroll.ConnectScroll(&scrollCb)

	glArea.AddController(&scroll.EventController)

	// Focus, keyboard, and IMContext.
	ib.attachFocusAndKeyboard(glArea)
}

// attachFocusAndKeyboard wires focus, keyboard, and IMContext controllers
// to the GLArea. Extracted from attachTo to keep function length manageable.
func (ib *inputBridge) attachFocusAndKeyboard(glArea *gtk.GLArea) {
	// Focus — forward to CEF and IMContext.
	focus := gtk.NewEventControllerFocus()
	focusEnterCb := func(_ gtk.EventControllerFocus) {
		if ib.imContext != nil {
			ib.imContext.FocusIn()
		}
		ib.onFocusIn()
	}
	focus.ConnectEnter(&focusEnterCb)
	focusLeaveCb := func(_ gtk.EventControllerFocus) {
		if ib.imContext != nil {
			ib.imContext.Reset()
			ib.imContext.FocusOut()
		}
		ib.onFocusOut()
	}
	focus.ConnectLeave(&focusLeaveCb)

	glArea.AddController(&focus.EventController)

	// Keyboard + IMContext for dead key / compose support.
	key := gtk.NewEventControllerKey()

	// Set up IMContext for dead key / compose support.
	// When set on the controller, GTK calls FilterKeypress on every key
	// event. If the IM absorbs the key (dead key, compose in-progress),
	// key-pressed is NOT emitted; the IM emits "commit" with the result.
	ib.imContext = gtk.NewIMContextSimple()
	if ib.imContext != nil {
		ib.commitCb = func(_ gtk.IMContext, text string) {
			ib.onIMCommit(text)
		}
		ib.imContext.ConnectCommit(&ib.commitCb)
		key.SetImContext(&ib.imContext.IMContext)
		ib.imContext.SetClientWidget(&glArea.Widget)
	}

	keyPressCb := func(_ gtk.EventControllerKey, keyval, keycode uint, state gdk.ModifierType) bool {
		mods := uint(state)
		ib.onKeyPress(keyval, keycode, mods)

		// Let Ctrl/Alt modified key combos propagate to the window's
		// ShortcutController so that app shortcuts (Ctrl+L, Ctrl+F, …)
		// still fire. Only consume unmodified / Shift-only keys that
		// are text input destined for CEF.
		if mods&uint(gdk.ControlMaskValue) != 0 || mods&uint(gdk.AltMaskValue) != 0 {
			return false
		}
		return true
	}
	key.ConnectKeyPressed(&keyPressCb)

	keyReleaseCb := func(_ gtk.EventControllerKey, keyval, keycode uint, state gdk.ModifierType) {
		ib.onKeyRelease(keyval, keycode, uint(state))
	}
	key.ConnectKeyReleased(&keyReleaseCb)

	glArea.AddController(&key.EventController)

	// Make the GLArea focusable so it can receive key events.
	glArea.SetFocusable(true)
	glArea.SetCanFocus(true)
}

// ---------------------------------------------------------------------------
// Event handlers
// ---------------------------------------------------------------------------

func (ib *inputBridge) onMouseMove(x, y float64, mods uint, leave bool) {
	ib.mu.Lock()
	host := ib.host
	if !leave {
		ib.lastX = x
		ib.lastY = y
	}
	ib.mu.Unlock()
	if host == nil {
		return
	}

	evt := buildMouseEvent(x, y, mods, ib.scale)
	mouseLeave := int32(0)
	if leave {
		mouseLeave = 1
	}
	host.SendMouseMoveEvent(&evt, mouseLeave)
}

func (ib *inputBridge) onMousePress(x, y float64, button, mods uint, clickCount int) {
	ib.mu.Lock()
	host := ib.host
	middleCB := ib.onMiddleClick
	ib.mu.Unlock()
	if host == nil {
		return
	}

	// Middle-click on a link opens in a new tab instead of sending to CEF.
	// The URI is resolved by the callback closure (factory wiring reads wv.lastHoverURI).
	if button == 2 && middleCB != nil {
		go middleCB("")
		return
	}

	evt := buildMouseEvent(x, y, mods, ib.scale)
	btn := translateMouseButton(button)
	host.SendMouseClickEvent(&evt, btn, 0, int32(clickCount))
}

func (ib *inputBridge) onMouseRelease(x, y float64, button, mods uint, clickCount int) {
	ib.mu.Lock()
	host := ib.host
	ib.mu.Unlock()
	if host == nil {
		return
	}

	evt := buildMouseEvent(x, y, mods, ib.scale)
	btn := translateMouseButton(button)
	host.SendMouseClickEvent(&evt, btn, 1, int32(clickCount))
}

func (ib *inputBridge) onScroll(dx, dy float64) {
	ib.mu.Lock()
	host := ib.host
	x, y := ib.lastX, ib.lastY
	ib.mu.Unlock()
	if host == nil {
		return
	}

	evt := buildMouseEvent(x, y, 0, ib.scale)
	deltaX, deltaY := translateScrollDeltas(dx, dy)
	host.SendMouseWheelEvent(&evt, deltaX, deltaY)
}

func (ib *inputBridge) onFocusIn() {
	ib.mu.Lock()
	host := ib.host
	ib.mu.Unlock()
	if host == nil {
		return
	}
	host.SetFocus(1)
}

func (ib *inputBridge) onFocusOut() {
	ib.mu.Lock()
	host := ib.host
	ib.mu.Unlock()
	if host == nil {
		return
	}
	host.SetFocus(0)
}

func (ib *inputBridge) onKeyPress(keyval, keycode, mods uint) {
	ib.mu.Lock()
	host := ib.host
	ib.mu.Unlock()
	if host == nil {
		return
	}

	// Dead keys should have been absorbed by the IMContext. If one leaks
	// through (e.g. no IMContext), drop it to avoid sending a bogus VK=0
	// RAWKEYDOWN that confuses CEF.
	if keyval >= gdkDeadKeyStart && keyval <= gdkDeadKeyEnd {
		return
	}

	// Send RAWKEYDOWN first, then CHAR if printable.
	evt := buildKeyEvent(keyval, keycode, mods, purecef.KeyEventTypeKeyeventRawkeydown)
	host.SendKeyEvent(&evt)

	// Follow up with a CHAR event for printable characters.
	if ch := keyvalToChar(keyval); ch != 0 {
		charEvt := buildKeyEvent(keyval, keycode, mods, purecef.KeyEventTypeKeyeventChar)
		// CHAR events use the character code as WindowsKeyCode (not the VK code).
		charEvt.WindowsKeyCode = int32(ch)
		charEvt.Character = ch
		charEvt.UnmodifiedCharacter = ch
		host.SendKeyEvent(&charEvt)
	}
}

// onIMCommit is called when the IMContext finishes a compose sequence and
// commits text (e.g. dead_acute + e → "é"). We send a CHAR event for each
// rune to CEF. No RAWKEYDOWN is needed for composed text — this matches
// Chromium's internal IME integration behavior on Linux.
func (ib *inputBridge) onIMCommit(text string) {
	ib.mu.Lock()
	host := ib.host
	ib.mu.Unlock()
	if host == nil {
		return
	}

	for _, r := range text {
		if r > maxBMPCodepoint {
			// Non-BMP character (emoji, etc.) — skip for now.
			// Full UTF-16 surrogate pair support can be added later.
			continue
		}
		ch := uint16(r)
		evt := purecef.NewKeyEvent(purecef.KeyEventTypeKeyeventChar, int32(ch), 0, 0)
		evt.Character = ch
		evt.UnmodifiedCharacter = ch
		host.SendKeyEvent(&evt)
	}
}

func (ib *inputBridge) onKeyRelease(keyval, keycode, mods uint) {
	ib.mu.Lock()
	host := ib.host
	ib.mu.Unlock()
	if host == nil {
		return
	}

	evt := buildKeyEvent(keyval, keycode, mods, purecef.KeyEventTypeKeyeventKeyup)
	host.SendKeyEvent(&evt)
}

// ---------------------------------------------------------------------------
// Translation helpers
// ---------------------------------------------------------------------------

func buildMouseEvent(x, y float64, gdkMods uint, scale int32) purecef.MouseEvent {
	return purecef.MouseEvent{
		X:         int32(x * float64(scale)),
		Y:         int32(y * float64(scale)),
		Modifiers: translateModifiers(gdkMods),
	}
}

func buildKeyEvent(keyval, keycode, gdkMods uint, eventType purecef.KeyEventType) purecef.KeyEvent {
	evt := purecef.NewKeyEvent(
		eventType,
		gdkKeyvalToWindowsVK(keyval),
		int32(keycode),
		translateModifiers(gdkMods),
	)
	return evt
}

// translateModifiers maps GDK modifier masks to CEF EventFlags values.
func translateModifiers(gdkState uint) uint32 {
	var flags uint32

	if gdkState&uint(gdk.ShiftMaskValue) != 0 {
		flags |= uint32(purecef.EventFlagsEventflagShiftDown)
	}
	if gdkState&uint(gdk.ControlMaskValue) != 0 {
		flags |= uint32(purecef.EventFlagsEventflagControlDown)
	}
	if gdkState&uint(gdk.AltMaskValue) != 0 {
		flags |= uint32(purecef.EventFlagsEventflagAltDown)
	}
	if gdkState&uint(gdk.Button1MaskValue) != 0 {
		flags |= uint32(purecef.EventFlagsEventflagLeftMouseButton)
	}
	if gdkState&uint(gdk.Button2MaskValue) != 0 {
		flags |= uint32(purecef.EventFlagsEventflagMiddleMouseButton)
	}
	if gdkState&uint(gdk.Button3MaskValue) != 0 {
		flags |= uint32(purecef.EventFlagsEventflagRightMouseButton)
	}

	return flags
}

// translateMouseButton maps a GDK button number to a CEF MouseButtonType.
func translateMouseButton(gdkButton uint) purecef.MouseButtonType {
	switch gdkButton {
	case 1:
		return purecef.MouseButtonTypeMbtLeft
	case 2:
		return purecef.MouseButtonTypeMbtMiddle
	case 3:
		return purecef.MouseButtonTypeMbtRight
	default:
		return purecef.MouseButtonTypeMbtLeft
	}
}

// cefScrollUnitsPerNotch is the number of scroll units CEF expects per mouse wheel notch.
const cefScrollUnitsPerNotch = 120

// translateScrollDeltas converts GDK scroll deltas to CEF units.
func translateScrollDeltas(dx, dy float64) (int32, int32) {
	return int32(dx * cefScrollUnitsPerNotch), int32(-dy * cefScrollUnitsPerNotch) // CEF: positive Y = scroll up
}

// GDK keyval constants for special keys.
const (
	gdkKeyReturn    = 0xff0d
	gdkKeyTab       = 0xff09
	gdkKeyBackSpace = 0xff08
	gdkKeyEscape    = 0xff1b
	gdkKeyDelete    = 0xffff
	gdkKeySpace     = 0x020
	gdkKeyHome      = 0xff50
	gdkKeyEnd       = 0xff57
	gdkKeyPageUp    = 0xff55
	gdkKeyPageDown  = 0xff56
)

// keyvalToChar converts a GDK keyval to a UTF-16 character for CEF CHAR events.
// Returns 0 for non-printable keys (arrows, F-keys, modifiers, dead keys, …).
// Uses gdk.KeyvalToUnicode for full Unicode coverage (accented Latin, Cyrillic, …).
func keyvalToChar(keyval uint) uint16 {
	switch keyval {
	case gdkKeyReturn:
		return '\r'
	case gdkKeyTab:
		return '\t'
	case gdkKeyBackSpace:
		return '\b'
	}

	cp := gdk.KeyvalToUnicode(keyval)
	if cp == 0 || cp > maxBMPCodepoint {
		return 0
	}
	// Filter control characters (except the three handled above).
	if cp < minPrintable {
		return 0
	}
	return uint16(cp)
}

// gdkKeyvalToWindowsVK translates a GDK keyval to a Windows virtual-key code.
func gdkKeyvalToWindowsVK(keyval uint) int32 {
	if vk, ok := gdkKeyvalToVKRange(keyval); ok {
		return vk
	}
	return gdkKeyvalToVKLookup(keyval)
}

// GDK keyval range boundaries for contiguous mappings.
const (
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

	vkA         = 0x41
	vkF1        = 0x70
	vkArrowLeft = 0x25
)

// gdkKeyvalToVKRange handles contiguous GDK keyval ranges that map linearly
// to Windows virtual-key codes (letters, digits, F-keys, arrows).
func gdkKeyvalToVKRange(keyval uint) (int32, bool) {
	switch {
	// Lowercase letters -> uppercase VK codes (A-Z = 0x41-0x5A)
	case keyval >= gdkKeyLowercaseAStart && keyval <= gdkKeyLowercaseAEnd:
		return int32(keyval-gdkKeyLowercaseAStart) + vkA, true
	// Uppercase letters
	case keyval >= gdkKeyUppercaseAStart && keyval <= gdkKeyUppercaseAEnd:
		return int32(keyval-gdkKeyUppercaseAStart) + vkA, true
	// Digits 0-9 = 0x30-0x39
	case keyval >= gdkKeyDigit0Start && keyval <= gdkKeyDigit9End:
		return int32(keyval), true
	// F1-F12: GDK 0xffbe-0xffc9 -> VK 0x70-0x7B
	case keyval >= gdkKeyF1Start && keyval <= gdkKeyF12End:
		return int32(keyval-gdkKeyF1Start) + vkF1, true
	// Arrow keys: GDK 0xff51-0xff54 -> VK Left=0x25..Down=0x28
	case keyval >= gdkKeyArrowStart && keyval <= gdkKeyArrowEnd:
		return int32(keyval-gdkKeyArrowStart) + vkArrowLeft, true
	default:
		return 0, false
	}
}

// gdkKeyvalToVKMap maps individual GDK keyvals to Windows virtual-key codes.
//
// Punctuation ASCII values collide with Windows VK codes for navigation keys
// (e.g. '.' = 0x2E = VK_DELETE, '#' = 0x23 = VK_END) so they MUST be mapped
// explicitly. Shifted digit symbols (!, @, #, …) are mapped to their base
// digit VK code to avoid triggering navigation actions.
var gdkKeyvalToVKMap = map[uint]int32{
	// Navigation / editing
	gdkKeyReturn:    0x0D, // VK_RETURN
	gdkKeyEscape:    0x1B, // VK_ESCAPE
	gdkKeyTab:       0x09, // VK_TAB
	gdkKeyBackSpace: 0x08, // VK_BACK
	gdkKeyDelete:    0x2E, // VK_DELETE
	gdkKeySpace:     0x20, // VK_SPACE
	gdkKeyHome:      0x24, // VK_HOME
	gdkKeyEnd:       0x23, // VK_END
	gdkKeyPageUp:    0x21, // VK_PRIOR
	gdkKeyPageDown:  0x22, // VK_NEXT
	0xff63:          0x2D, // Insert → VK_INSERT

	// Modifiers
	0xffe1: 0xA0, // Shift_L → VK_LSHIFT
	0xffe2: 0xA1, // Shift_R → VK_RSHIFT
	0xffe3: 0xA2, // Control_L → VK_LCONTROL
	0xffe4: 0xA3, // Control_R → VK_RCONTROL
	0xffe9: 0xA4, // Alt_L → VK_LMENU
	0xffea: 0xA5, // Alt_R → VK_RMENU

	// OEM punctuation — unshifted + shifted share the same VK
	'.': 0xBE, '>': 0xBE, // VK_OEM_PERIOD
	',': 0xBC, '<': 0xBC, // VK_OEM_COMMA
	'-': 0xBD, '_': 0xBD, // VK_OEM_MINUS
	'=': 0xBB, '+': 0xBB, // VK_OEM_PLUS
	';': 0xBA, ':': 0xBA, // VK_OEM_1
	'/': 0xBF, '?': 0xBF, // VK_OEM_2
	'`': 0xC0, '~': 0xC0, // VK_OEM_3
	'[': 0xDB, '{': 0xDB, // VK_OEM_4
	'\\': 0xDC, '|': 0xDC, // VK_OEM_5
	']': 0xDD, '}': 0xDD, // VK_OEM_6
	'\'': 0xDE, '"': 0xDE, // VK_OEM_7

	// Shifted digit symbols (Shift+1→'!' etc.)
	'!': 0x31, '@': 0x32, '#': 0x33, '$': 0x34, '%': 0x35,
	'^': 0x36, '&': 0x37, '*': 0x38, '(': 0x39, ')': 0x30,
}

// gdkKeyvalToVKLookup handles individual GDK keyvals that don't fall into
// contiguous ranges (letters, digits, F-keys, arrows).
func gdkKeyvalToVKLookup(keyval uint) int32 {
	if vk, ok := gdkKeyvalToVKMap[keyval]; ok {
		return vk
	}
	// For unmapped keys, return the keyval if it fits in a reasonable range.
	// This covers misc keys whose ASCII value doesn't collide with
	// dangerous VK codes (e.g. space=0x20=VK_SPACE — correct match).
	if keyval < maxSingleByteKeyval {
		return int32(keyval)
	}
	return 0
}
