package cef

import (
	"sync"
	"unsafe"

	purecef "github.com/bnema/purego-cef/cef"
	"github.com/jwijenbergh/puregotk/v4/gdk"
	"github.com/jwijenbergh/puregotk/v4/gtk"
)

// inputBridge translates GDK events from GTK event controllers into CEF input
// events and forwards them to the CEF BrowserHost. The host field is nil until
// the browser is created asynchronously (OnAfterCreated), so all dispatch
// methods guard against a nil host.
type inputBridge struct {
	host  purecef.BrowserHost
	scale int32
	mu    sync.Mutex

	// Last known pointer position, used for scroll events which don't
	// carry their own coordinates from GDK.
	lastX, lastY float64
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
		ib.onMousePress(x, y, btn, uint(0), nPress)
	}
	click.ConnectPressed(&pressedCb)

	releasedCb := func(g gtk.GestureClick, nPress int, x, y float64) {
		btn := g.GetCurrentButton()
		ib.onMouseRelease(x, y, btn, uint(0), nPress)
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

	// Focus — forward to CEF so it knows when it has/loses keyboard focus.
	focus := gtk.NewEventControllerFocus()
	focusEnterCb := func(_ gtk.EventControllerFocus) {
		ib.onFocusIn()
	}
	focus.ConnectEnter(&focusEnterCb)
	focusLeaveCb := func(_ gtk.EventControllerFocus) {
		ib.onFocusOut()
	}
	focus.ConnectLeave(&focusLeaveCb)

	glArea.AddController(&focus.EventController)

	// Keyboard
	key := gtk.NewEventControllerKey()
	keyPressCb := func(_ gtk.EventControllerKey, keyval, keycode uint, state gdk.ModifierType) bool {
		ib.onKeyPress(keyval, keycode, uint(state))
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
	ib.mu.Unlock()
	if host == nil {
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

	// Send RAWKEYDOWN first, then CHAR if printable.
	evt := buildKeyEvent(keyval, keycode, mods, purecef.KeyEventTypeKeyeventRawkeydown)
	host.SendKeyEvent(&evt)

	// Follow up with a CHAR event for printable characters.
	if ch := keyvalToChar(keyval); ch != 0 {
		charEvt := buildKeyEvent(keyval, keycode, mods, purecef.KeyEventTypeKeyeventChar)
		charEvt.Character = ch
		charEvt.UnmodifiedCharacter = ch
		host.SendKeyEvent(&charEvt)
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
	var evt purecef.KeyEvent
	evt.Size = unsafe.Sizeof(evt)
	// Use unsafe to set Type field — the underlying types are both int32 but
	// the struct field uses an internal (unexportable) named type.
	*(*int32)(unsafe.Pointer(&evt.Type)) = int32(eventType)
	evt.Modifiers = translateModifiers(gdkMods)
	evt.WindowsKeyCode = gdkKeyvalToWindowsVK(keyval)
	evt.NativeKeyCode = int32(keycode)
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

// keyvalToChar returns the UTF-16 character for a printable GDK keyval, or 0.
func keyvalToChar(keyval uint) uint16 {
	// Printable ASCII range.
	if keyval >= 0x020 && keyval <= 0x07e {
		return uint16(keyval)
	}
	switch keyval {
	case gdkKeyReturn:
		return '\r'
	case gdkKeyTab:
		return '\t'
	case gdkKeyBackSpace:
		return '\b'
	default:
		return 0
	}
}

// gdkKeyvalToWindowsVK translates a GDK keyval to a Windows virtual-key code.
func gdkKeyvalToWindowsVK(keyval uint) int32 {
	if vk, ok := gdkKeyvalToVKRange(keyval); ok {
		return vk
	}
	return gdkKeyvalToVKSwitch(keyval)
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

// gdkKeyvalToVKSwitch handles individual GDK keyvals that map to specific
// Windows virtual-key codes.
//
//nolint:mnd // GDK keyval→VK code lookup table; named constants would hurt readability.
func gdkKeyvalToVKSwitch(keyval uint) int32 {
	switch keyval {
	case gdkKeyReturn:
		return 0x0D
	case gdkKeyEscape:
		return 0x1B
	case gdkKeyTab:
		return 0x09
	case gdkKeyBackSpace:
		return 0x08
	case gdkKeyDelete:
		return 0x2E
	case gdkKeySpace:
		return 0x20
	case gdkKeyHome:
		return 0x24
	case gdkKeyEnd:
		return 0x23
	case gdkKeyPageUp:
		return 0x21
	case gdkKeyPageDown:
		return 0x22
	case 0xffe1, 0xffe2: // Shift_L, Shift_R
		return 0xA0
	case 0xffe3, 0xffe4: // Control_L, Control_R
		return 0xA2
	case 0xffe9, 0xffea: // Alt_L, Alt_R
		return 0xA4
	default:
		// For unmapped keys, return the keyval if it fits in a reasonable range.
		if keyval < 0x100 {
			return int32(keyval)
		}
		return 0
	}
}
