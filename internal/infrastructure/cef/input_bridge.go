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
	ib.mu.Unlock()
	if host == nil {
		return
	}

	evt := buildMouseEvent(0, 0, 0, ib.scale)
	deltaX, deltaY := translateScrollDeltas(dx, dy)
	host.SendMouseWheelEvent(&evt, deltaX, deltaY)
}

func (ib *inputBridge) onKeyPress(keyval, keycode uint, mods uint) {
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

func (ib *inputBridge) onKeyRelease(keyval, keycode uint, mods uint) {
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

func buildKeyEvent(keyval, keycode uint, gdkMods uint, eventType purecef.KeyEventType) purecef.KeyEvent {
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

// translateScrollDeltas converts GDK scroll deltas to CEF units (120 per notch).
func translateScrollDeltas(dx, dy float64) (int32, int32) {
	return int32(dx * 120), int32(-dy * 120) // CEF: positive Y = scroll up
}

// keyvalToChar returns the UTF-16 character for a printable GDK keyval, or 0.
func keyvalToChar(keyval uint) uint16 {
	// Printable ASCII range.
	if keyval >= 0x020 && keyval <= 0x07e {
		return uint16(keyval)
	}
	// GDK_KEY_Return → carriage return
	if keyval == 0xff0d {
		return '\r'
	}
	// GDK_KEY_Tab
	if keyval == 0xff09 {
		return '\t'
	}
	// GDK_KEY_BackSpace
	if keyval == 0xff08 {
		return '\b'
	}
	return 0
}

// gdkKeyvalToWindowsVK translates a GDK keyval to a Windows virtual-key code.
func gdkKeyvalToWindowsVK(keyval uint) int32 {
	// Lowercase letters → uppercase VK codes (A-Z = 0x41-0x5A)
	if keyval >= 0x061 && keyval <= 0x07a {
		return int32(keyval-0x061) + 0x41
	}
	// Uppercase letters
	if keyval >= 0x041 && keyval <= 0x05a {
		return int32(keyval-0x041) + 0x41
	}
	// Digits 0-9 = 0x30-0x39
	if keyval >= 0x030 && keyval <= 0x039 {
		return int32(keyval)
	}
	// F1-F12: GDK 0xffbe-0xffc9 → VK 0x70-0x7B
	if keyval >= 0xffbe && keyval <= 0xffc9 {
		return int32(keyval-0xffbe) + 0x70
	}
	// Arrow keys: GDK 0xff51-0xff54 → VK Left=0x25, Up=0x26, Right=0x27, Down=0x28
	if keyval >= 0xff51 && keyval <= 0xff54 {
		return int32(keyval-0xff51) + 0x25
	}

	// Individual keys
	switch keyval {
	case 0xff0d: // Return
		return 0x0D
	case 0xff1b: // Escape
		return 0x1B
	case 0xff09: // Tab
		return 0x09
	case 0xff08: // BackSpace
		return 0x08
	case 0xffff: // Delete
		return 0x2E
	case 0x020: // space
		return 0x20
	case 0xff50: // Home
		return 0x24
	case 0xff57: // End
		return 0x23
	case 0xff55: // Page_Up
		return 0x21
	case 0xff56: // Page_Down
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
