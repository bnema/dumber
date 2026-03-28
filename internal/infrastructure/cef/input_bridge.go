package cef

import (
	"context"
	"sync"

	purecef "github.com/bnema/purego-cef/cef"
	"github.com/bnema/puregotk/v4/gdk"
	"github.com/bnema/puregotk/v4/gio"
	"github.com/bnema/puregotk/v4/gtk"

	"github.com/bnema/dumber/internal/logging"
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
	ctx   context.Context
	mu    sync.Mutex

	// Last known pointer position, used for scroll events which don't
	// carry their own coordinates from GDK.
	lastX, lastY float64

	// GDK clipboard for paste support. CEF in OSR mode cannot access the
	// Wayland clipboard directly, so we read from GDK and inject via
	// ImeCommitText. Set once during attachTo, read-only afterwards.
	clipboard *gdk.Clipboard

	// IMContext for dead key / compose sequence support. Held as a field
	// to prevent garbage collection while the controller is alive.
	imContext *gtk.IMContextSimple
	// Prevent GC from collecting signal callbacks.
	commitCb func(gtk.IMContext, string)

	// glArea is stored so we can GrabFocus on click. Without explicit
	// focus grab, GTK may not fire the focus-enter signal and CEF won't
	// show the text caret.
	glArea *gtk.GLArea

	// onMiddleClick is called when button 2 (middle) is pressed on a link.
	// The callback receives the hovered URI. Set by the factory.
	onMiddleClick func(uri string)
}

// newInputBridge creates an input bridge with the given HiDPI scale factor.
func newInputBridge(ctx context.Context, scale int32) *inputBridge {
	return &inputBridge{ctx: ctx, scale: scale}
}

// setHost is called once the browser is created to enable event dispatch.
func (ib *inputBridge) setHost(host purecef.BrowserHost) {
	ib.mu.Lock()
	defer ib.mu.Unlock()
	ib.host = host
}

// attachTo creates GDK event controllers and connects them to the given GLArea.
func (ib *inputBridge) attachTo(glArea *gtk.GLArea) {
	ib.glArea = glArea

	// Acquire GDK clipboard for paste support. CEF OSR can't access the
	// Wayland clipboard, so we bridge it via GDK → ImeCommitText.
	if display := gdk.DisplayGetDefault(); display != nil {
		ib.clipboard = display.GetClipboard()
		if ib.clipboard != nil {
			logging.FromContext(ib.ctx).Debug().Msg("cef: GDK clipboard acquired for paste bridge")
		} else {
			logging.FromContext(ib.ctx).Warn().Msg("cef: GDK display found but GetClipboard returned nil")
		}
	} else {
		logging.FromContext(ib.ctx).Warn().Msg("cef: no GDK display — clipboard paste will not work")
	}

	// Motion (mouse move + enter/leave)
	motion := gtk.NewEventControllerMotion()
	motionCb := func(g gtk.EventControllerMotion, x, y float64) {
		mods := uint(g.GetCurrentEventState())
		ib.onMouseMove(x, y, mods, false)
	}
	motion.ConnectMotion(&motionCb)

	leaveCb := func(g gtk.EventControllerMotion) {
		mods := uint(g.GetCurrentEventState())
		ib.onMouseMove(0, 0, mods, true)
	}
	motion.ConnectLeave(&leaveCb)

	glArea.AddController(&motion.EventController)

	// Click (press / release)
	click := gtk.NewGestureClick()
	click.SetButton(0) // listen to all buttons

	pressedCb := func(g gtk.GestureClick, nPress int, x, y float64) {
		// Ensure the GLArea has GTK focus so CEF receives SetFocus(1)
		// and renders the text caret. Without this, clicking an input
		// field may not show the blinking cursor.
		if ib.glArea != nil {
			ib.glArea.GrabFocus()
		}
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
	scroll := gtk.NewEventControllerScroll(gtk.EventControllerScrollBothAxesValue | gtk.EventControllerScrollKineticValue)
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

		// Intercept Ctrl+V / Ctrl+Shift+V: CEF OSR cannot access the
		// Wayland/X11 clipboard, so we read from GDK and inject via
		// ImeCommitText instead of letting CEF handle the paste.
		if mods&uint(gdk.ControlMaskValue) != 0 &&
			(keyval == gdkKeyLowercaseV || keyval == gdkKeyUppercaseV) {
			logging.FromContext(ib.ctx).Debug().
				Uint("keyval", keyval).
				Uint("mods", mods).
				Msg("cef: clipboard paste intercepted (Ctrl+V)")
			ib.pasteFromClipboard()
			return true
		}

		ib.onKeyPress(keyval, keycode, mods)

		// Let modifier combos and function keys propagate to the window's
		// ShortcutController so that app shortcuts (Ctrl+L, F12, Escape, …)
		// still fire. Only consume plain text input keys.
		if mods&uint(gdk.ControlMaskValue) != 0 || mods&uint(gdk.AltMaskValue) != 0 {
			return false
		}
		// F-keys (F1–F12) and Escape are app shortcuts, not text input.
		if keyval >= gdkKeyF1Start && keyval <= gdkKeyF12End {
			return false
		}
		if keyval == gdkKeyEscape {
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
	// The empty string parameter is unused — the callback closure resolves
	// the actual URI via wv.lastHoverURI at invocation time.
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
		evt := purecef.NewKeyEvent()
		evt.Type = purecef.KeyEventTypeKeyeventChar
		evt.WindowsKeyCode = int32(ch)
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

// pasteFromClipboard reads text from the GDK clipboard asynchronously and
// injects it into the focused CEF input via document.execCommand('insertText').
// ImeCommitText only works during active IME compositions, so we use JS
// injection which works for any focused editable element.
func (ib *inputBridge) pasteFromClipboard() {
	log := logging.FromContext(ib.ctx)

	cb := ib.clipboard
	if cb == nil {
		log.Warn().Msg("cef: paste aborted — no GDK clipboard reference")
		return
	}
	ib.mu.Lock()
	host := ib.host
	ib.mu.Unlock()
	if host == nil {
		log.Warn().Msg("cef: paste aborted — no browser host")
		return
	}

	log.Debug().Msg("cef: calling ReadTextAsync on GDK clipboard")

	asyncCb := gio.AsyncReadyCallback(func(_, resultPtr, _ uintptr) {
		log.Debug().Uint64("result_ptr", uint64(resultPtr)).Msg("cef: ReadTextAsync callback fired")

		result := &gio.AsyncResultBase{Ptr: resultPtr}
		text, err := cb.ReadTextFinish(result)
		if err != nil {
			log.Warn().Err(err).Msg("cef: ReadTextFinish failed")
			return
		}
		if text == "" {
			log.Debug().Msg("cef: clipboard text is empty, nothing to paste")
			return
		}

		log.Debug().Int("text_len", len(text)).Msg("cef: injecting clipboard text via JS execCommand")

		ib.mu.Lock()
		h := ib.host
		ib.mu.Unlock()
		if h == nil {
			log.Warn().Msg("cef: paste — host became nil before JS injection")
			return
		}

		browser := h.GetBrowser()
		if browser == nil {
			log.Warn().Msg("cef: paste — GetBrowser returned nil")
			return
		}
		frame := browser.GetMainFrame()
		if frame == nil {
			log.Warn().Msg("cef: paste — GetMainFrame returned nil")
			return
		}

		// Escape the text for embedding in a JS string literal.
		escaped := escapeForJSString(text)
		js := "document.execCommand('insertText',false,'" + escaped + "')"
		frame.ExecuteJavaScript(js, "", 0)
		log.Debug().Msg("cef: paste JS executed")
	})

	cb.ReadTextAsync(nil, &asyncCb, 0)
	log.Debug().Msg("cef: ReadTextAsync dispatched")
}

// escapeForJSString is defined in content_injector.go — reused here.

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
	evt := purecef.NewKeyEvent()
	evt.Type = eventType
	evt.WindowsKeyCode = gdkKeyvalToWindowsVK(keyval)
	evt.NativeKeyCode = int32(keycode)
	evt.Modifiers = translateModifiers(gdkMods)
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

	// Clipboard shortcut keyvals.
	gdkKeyLowercaseV = 0x076
	gdkKeyUppercaseV = 0x056
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
