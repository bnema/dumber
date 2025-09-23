//go:build webkit_cgo

package webkit

/*
#cgo pkg-config: gtk4 webkitgtk-6.0
#include <gtk/gtk.h>
#include <gdk/gdk.h>
#include <webkit/webkit.h>

// Declarations for Go callbacks
extern gboolean goOnKeyPress(unsigned long id, unsigned int keyval, GdkModifierType state);
extern void goOnButtonPress(unsigned long id, unsigned int button, GdkModifierType state);
extern void goOnScroll(unsigned long id, double dx, double dy, GdkModifierType state);

// Key controller callback -> forward to Go
static gboolean on_key_pressed(GtkEventControllerKey* controller, guint keyval, guint keycode, GdkModifierType state, gpointer user_data) {
    (void)controller; (void)keycode;
    return goOnKeyPress((unsigned long)user_data, keyval, state);
}

// Mouse click gesture -> forward side buttons to Go
static void on_click_pressed(GtkGestureClick* gesture, gint n_press, gdouble x, gdouble y, gpointer user_data) {
    (void)n_press; (void)x; (void)y;
    guint button = gtk_gesture_single_get_current_button(GTK_GESTURE_SINGLE(gesture));
    // Pass 0 for modifier state (not provided via gesture API)
    goOnButtonPress((unsigned long)user_data, button, 0);
}

// Attach controllers to a widget (GTK4)
static void attach_key_controller(GtkWidget* widget, unsigned long id) {
    if (!widget) return;
    GtkEventController* keyc = gtk_event_controller_key_new();
    // Capture phase so we see keys even if child consumes them (e.g., WebKit)
    gtk_event_controller_set_propagation_phase(keyc, GTK_PHASE_CAPTURE);
    g_signal_connect_data(keyc, "key-pressed", G_CALLBACK(on_key_pressed), (gpointer)id, NULL, 0);
    gtk_widget_add_controller(widget, keyc);
}

static void attach_mouse_gesture(GtkWidget* widget, unsigned long id) {
    if (!widget) return;
    GtkGesture* click = gtk_gesture_click_new();
    g_signal_connect_data(click, "pressed", G_CALLBACK(on_click_pressed), (gpointer)id, NULL, 0);
    gtk_widget_add_controller(widget, GTK_EVENT_CONTROLLER(click));
}

// Legacy controller to capture side buttons (8/9) reliably in GTK4
static gboolean on_legacy_event(GtkEventController* controller, GdkEvent* event, gpointer user_data) {
    (void)controller;
    if (!event) return FALSE;
    GdkEventType type = gdk_event_get_event_type(event);
    if (type == GDK_BUTTON_PRESS) {
        guint button = gdk_button_event_get_button(event);
        GdkModifierType state = gdk_event_get_modifier_state(event);
        if (button == 8 || button == 9) {
            goOnButtonPress((unsigned long)user_data, button, state);
            return TRUE; // consume side buttons
        }
        return FALSE; // let WebKit handle normal clicks
    } else if (type == GDK_SCROLL) {
        double dx = 0.0, dy = 0.0;
        gdk_scroll_event_get_deltas(event, &dx, &dy);
        GdkModifierType state = gdk_event_get_modifier_state(event);
        goOnScroll((unsigned long)user_data, dx, dy, state);
        return FALSE; // don't steal regular scrolling
    }
    return FALSE;
}

static void attach_mouse_legacy(GtkWidget* widget, unsigned long id) {
    if (!widget) return;
    GtkEventController* legacy = gtk_event_controller_legacy_new();
    gtk_event_controller_set_propagation_phase(legacy, GTK_PHASE_CAPTURE);
    g_signal_connect_data(legacy, "event", G_CALLBACK(on_legacy_event), (gpointer)id, NULL, 0);
    gtk_widget_add_controller(widget, legacy);
}
*/
import "C"

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Registry of accelerators per view id.
type shortcutRegistry map[string]func()

var (
	regMu         sync.RWMutex
	viewShortcuts = make(map[uintptr]shortcutRegistry)
	viewIDCounter uint64
	viewByID      = make(map[uintptr]*WebView)
	lastKeyTime   = make(map[uintptr]map[string]time.Time)
)

const shortcutDebounceWindow = 120 * time.Millisecond

// shouldDebounceShortcut returns true when the given key should be skipped
// because it was delivered within the debounce window for the same view.
func shouldDebounceShortcut(uid uintptr, key string, now time.Time) bool {
	regMu.Lock()
	defer regMu.Unlock()

	if _, ok := lastKeyTime[uid]; !ok {
		lastKeyTime[uid] = make(map[string]time.Time)
	}
	last := lastKeyTime[uid][key]
	if now.Sub(last) < shortcutDebounceWindow {
		return true
	}
	lastKeyTime[uid][key] = now
	return false
}

// Locale hint function removed; no layout-specific remaps.

// RegisterKeyboardShortcut registers a callback under an accelerator string.
func (w *WebView) RegisterKeyboardShortcut(accel string, callback func()) error {
	if w == nil || w.destroyed {
		return ErrNotImplemented
	}
	if accel == "" || callback == nil {
		return fmt.Errorf("invalid shortcut registration")
	}
	regMu.Lock()
	if _, ok := viewShortcuts[w.id]; !ok {
		viewShortcuts[w.id] = make(shortcutRegistry)
	}
	viewShortcuts[w.id][accel] = callback
	regMu.Unlock()

	// Register in global shortcuts registry for automatic webpage blocking
	RegisterGlobalShortcut(accel)

	return nil
}

//export goOnKeyPress
func goOnKeyPress(id C.ulong, keyval C.guint, state C.GdkModifierType) C.gboolean {
	// Normalize and dispatch
	uid := uintptr(id)
	kv := uint(keyval)
	st := uint(state)
	if dispatchAccelerator(uid, kv, st) {
		return C.gboolean(1)
	}
	return C.gboolean(0)
}

func dispatchAccelerator(uid uintptr, keyval uint, state uint) bool {
	ctrl := (state & uint(C.GDK_CONTROL_MASK)) != 0
	alt := (state & uint(C.GDK_ALT_MASK)) != 0
	shift := (state & uint(C.GDK_SHIFT_MASK)) != 0
	// Map keyval to string names
	var keyName string
	logMinusEvent := false
	switch keyval {
	case uint(C.GDK_KEY_minus), uint(C.GDK_KEY_KP_Subtract), uint(C.GDK_KEY_underscore):
		// Normalize minus; log raw detection for diagnostics
		logMinusEvent = true
		keyName = "-"
	case uint(C.GDK_KEY_equal), uint(C.GDK_KEY_KP_Add):
		keyName = "="
	case uint(C.GDK_KEY_0), uint(C.GDK_KEY_KP_0):
		keyName = "0"
	case uint(C.GDK_KEY_Left):
		keyName = "ArrowLeft"
	case uint(C.GDK_KEY_Right):
		keyName = "ArrowRight"
	case uint(C.GDK_KEY_Up):
		keyName = "ArrowUp"
	case uint(C.GDK_KEY_Down):
		keyName = "ArrowDown"
	case uint(C.GDK_KEY_KP_Left):
		keyName = "ArrowLeft"
	case uint(C.GDK_KEY_KP_Right):
		keyName = "ArrowRight"
	case uint(C.GDK_KEY_KP_Up):
		keyName = "ArrowUp"
	case uint(C.GDK_KEY_KP_Down):
		keyName = "ArrowDown"
	case uint(C.GDK_KEY_F12):
		keyName = "F12"
	case uint(C.GDK_KEY_F5):
		keyName = "F5"
	case uint(C.GDK_KEY_c), uint(C.GDK_KEY_C):
		keyName = "c"
	case uint(C.GDK_KEY_r), uint(C.GDK_KEY_R):
		keyName = "r"
	case uint(C.GDK_KEY_l), uint(C.GDK_KEY_L):
		keyName = "l"
	case uint(C.GDK_KEY_f), uint(C.GDK_KEY_F):
		keyName = "f"
	case uint(C.GDK_KEY_p), uint(C.GDK_KEY_P):
		keyName = "p"
	default:
		// If Control held, log unknown keyval for diagnostics
		if ctrl {
			log.Printf("[accelerator-miss] ctrl keyval=0x%x", keyval)
		}
		return false
	}

	now := time.Now()
	rawKey := fmt.Sprintf("raw:%v:%v:%v:%s", ctrl, alt, shift, keyName)
	if shouldDebounceShortcut(uid, rawKey, now) {
		return false
	}

	if logMinusEvent {
		log.Printf("[accelerator-raw] minus key detected ctrl=%v alt=%v keyval=0x%x", ctrl, alt, keyval)
	}

	// Candidate accelerator strings in order
	handled := false
	candidates := []string{keyName}
	if ctrl && shift {
		// Ctrl+Shift combinations
		candidates = append([]string{"cmdorctrl+shift+" + keyName, "ctrl+shift+" + keyName}, candidates...)
	} else if ctrl {
		// Common style: modifier + '+' + keyName (e.g., cmdorctrl+=)
		candidates = append([]string{"cmdorctrl+" + keyName, "ctrl+" + keyName}, candidates...)
		// Also accept style without the extra '+' for punctuation keys (e.g., cmdorctrl-)
		candidates = append([]string{"cmdorctrl" + keyName, "ctrl" + keyName}, candidates...)
	}
	if alt {
		candidates = append([]string{"alt+" + keyName}, candidates...)
	}

	// Compute a single normalized shortcut string to forward to GUI (single source of truth)
	normalized := ""
	if ctrl && shift {
		normalized = "cmdorctrl+shift+" + keyName
	} else if ctrl {
		normalized = "cmdorctrl+" + keyName
	} else if alt {
		normalized = "alt+" + keyName
	} else if keyName != "" {
		// For future modes like Vim, we may want to forward non-modified keys
		// For now, only forward if a modifier is present to avoid noise
		normalized = ""
	}

	// Forward normalized shortcut to GUI KeyboardService via DOM event if present
	if normalized != "" && !shouldDebounceShortcut(uid, "dom:"+normalized, now) {
		regMu.RLock()
		vw := viewByID[uid]
		regMu.RUnlock()
		if vw != nil {
			_ = vw.InjectScript(fmt.Sprintf("document.dispatchEvent(new CustomEvent('dumber:key',{detail:{shortcut:'%s'}}));", normalized))
		}
	}

	regMu.RLock()
	reg := viewShortcuts[uid]
	regMu.RUnlock()
	if reg == nil {
		return handled
	}
	for _, name := range candidates {
		if cb, ok := reg[name]; ok {
			if shouldDebounceShortcut(uid, "shortcut:"+name, now) {
				return handled
			}
			log.Printf("[accelerator] %s", name)
			cb()
			handled = true
			break
		}
	}

	return handled
}

func nextViewID() uintptr {
	return uintptr(atomic.AddUint64(&viewIDCounter, 1))
}

func registerView(id uintptr, w *WebView) {
	regMu.Lock()
	viewByID[id] = w
	regMu.Unlock()
}

func unregisterView(id uintptr) {
	regMu.Lock()
	delete(viewByID, id)
	delete(viewShortcuts, id)
	regMu.Unlock()
}

// AttachKeyboardControllers attaches GTK4 key and mouse controllers to the view widget.
func AttachKeyboardControllers(w *WebView) {
	if w == nil || w.native == nil || w.native.view == nil {
		return
	}
	C.attach_key_controller(w.native.view, C.ulong(w.id))
	if w.native.win != nil {
		C.attach_key_controller(w.native.win, C.ulong(w.id))
	}
	C.attach_mouse_gesture(w.native.view, C.ulong(w.id))
	// Legacy controller captures raw button presses (8/9) for back/forward reliably
	C.attach_mouse_legacy(w.native.view, C.ulong(w.id))
	log.Printf("[input] GTK4 key/mouse controllers attached")
}

//export goOnScroll
func goOnScroll(id C.ulong, dx C.double, dy C.double, state C.GdkModifierType) {
	uid := uintptr(id)
	regMu.RLock()
	vw := viewByID[uid]
	regMu.RUnlock()
	if vw == nil {
		return
	}
	// Ctrl+scroll zooms
	if (uint(state) & uint(C.GDK_CONTROL_MASK)) != 0 {
		if float64(dy) < 0 {
			// scroll up -> zoom in
			nz := vw.zoom
			if nz <= 0 {
				nz = 1.0
			}
			nz *= 1.1
			if nz < 0.25 {
				nz = 0.25
			}
			if nz > 5.0 {
				nz = 5.0
			}
			_ = vw.SetZoom(nz)
		} else if float64(dy) > 0 {
			// scroll down -> zoom out
			nz := vw.zoom
			if nz <= 0 {
				nz = 1.0
			}
			nz /= 1.1
			if nz < 0.25 {
				nz = 0.25
			}
			if nz > 5.0 {
				nz = 5.0
			}
			_ = vw.SetZoom(nz)
		}
	}
}

//export goOnThemeChanged
func goOnThemeChanged(id C.ulong, preferDark C.int) {
	uid := uintptr(id)
	regMu.RLock()
	vw := viewByID[uid]
	regMu.RUnlock()
	if vw == nil {
		return
	}
	// Inject runtime theme update into the page
	dval := "false"
	if int(preferDark) != 0 {
		dval = "true"
	}
	js := fmt.Sprintf(`(() => { try { const d=%s; const cs=d?'dark':'light'; console.log('[dumber] theme change:'+cs);
let m=document.querySelector('meta[name="color-scheme"]');
if(!m){ m=document.createElement('meta'); m.name='color-scheme'; document.head.appendChild(m); }
m.setAttribute('content', d ? 'dark light' : 'light dark');
let s=document.getElementById('__dumber_theme_style');
if(!s){ s=document.createElement('style'); s.id='__dumber_theme_style'; document.documentElement.appendChild(s); }
s.textContent=':root{color-scheme:' + (d?'dark':'light') + ';}';
// Call the unified theme setter for Tailwind dark mode
if (window.__dumber_setTheme) {
  window.__dumber_setTheme(cs);
} else {
  // Retry after a short delay in case bridge hasn't loaded yet
  setTimeout(() => {
    if (window.__dumber_setTheme) {
      window.__dumber_setTheme(cs);
    } else {
      // Fallback: directly apply dark class
      console.warn('[dumber] Theme setter not available, using fallback');
      if (d) {
        document.documentElement.classList.add('dark');
      } else {
        document.documentElement.classList.remove('dark');
      }
    }
  }, 100);
}
} catch(e) { console.warn('[dumber] theme runtime update failed', e); } })();`, dval)
	_ = vw.InjectScript(js)
}

//export goOnUcmMessage
func goOnUcmMessage(id C.ulong, json *C.char) {
	uid := uintptr(id)
	regMu.RLock()
	vw := viewByID[uid]
	regMu.RUnlock()
	if vw == nil || json == nil {
		return
	}
	goPayload := C.GoString(json)

	// Handle rendering backend detection messages specially
	if handleRenderingBackendMessage(goPayload) {
		return
	}

	// Handle keyboard blocking messages
	if handleKeyboardBlockingMessage(vw, goPayload) {
		return
	}

	vw.dispatchScriptMessage(goPayload)
}

//export goOnTitleChanged
func goOnTitleChanged(id C.ulong, ctitle *C.char) {
	uid := uintptr(id)
	regMu.RLock()
	vw := viewByID[uid]
	regMu.RUnlock()
	if vw == nil || ctitle == nil {
		return
	}
	title := C.GoString(ctitle)
	vw.dispatchTitleChanged(title)
}

//export goOnURIChanged
func goOnURIChanged(id C.ulong, curi *C.char) {
	uid := uintptr(id)
	regMu.RLock()
	vw := viewByID[uid]
	regMu.RUnlock()
	if vw == nil || curi == nil {
		return
	}
	uri := C.GoString(curi)
	vw.dispatchURIChanged(uri)
}

//export goHandleNewWindowPolicy
func goHandleNewWindowPolicy(id C.ulong, curi *C.char, navType C.int, cframeName *C.char, isUserGesture C.gboolean, modifiers C.guint, mouseButton C.guint) C.gboolean {
	uid := uintptr(id)
	regMu.RLock()
	vw := viewByID[uid]
	regMu.RUnlock()

	if vw == nil {
		log.Printf("[webkit] Policy decision: no WebView found for ID %d", uid)
		return 0 // FALSE
	}

	uri := ""
	if curi != nil {
		uri = C.GoString(curi)
	}

	frameName := ""
	if cframeName != nil {
		frameName = C.GoString(cframeName)
	}

	// Navigation type constants from WebKit headers
	navTypeNames := map[int]string{
		0: "LINK_CLICKED",
		1: "FORM_SUBMITTED",
		2: "BACK_FORWARD",
		3: "RELOAD",
		4: "FORM_RESUBMITTED",
		5: "OTHER",
	}

	navTypeName := navTypeNames[int(navType)]
	if navTypeName == "" {
		navTypeName = fmt.Sprintf("UNKNOWN(%d)", int(navType))
	}

	// Convert parameters for analysis
	userGesture := isUserGesture != 0
	mods := uint(modifiers)
	button := uint(mouseButton)

	// Check for modifier keys that indicate new tab intent
	const (
		GDK_CONTROL_MASK = 0x04
		GDK_META_MASK    = 0x10000000 // Command key on macOS
	)
	hasCtrlOrCmd := (mods&GDK_CONTROL_MASK != 0) || (mods&GDK_META_MASK != 0)

	log.Printf("[webkit] Policy decision for new window: URI=%s, NavType=%s, FrameName=%s, UserGesture=%v, Modifiers=0x%x, Button=%d, CtrlOrCmd=%v",
		uri, navTypeName, frameName, userGesture, mods, button, hasCtrlOrCmd)

	// Enhanced detection logic for _blank vs popup using reliable heuristics
	isRegularPane := false

	// Criteria for treating as regular pane (new panel):
	// 1. Explicit _blank frame name (target="_blank" links)
	// 2. User gesture with Ctrl/Cmd+click (open in new panel intent)
	// 3. User-initiated OTHER navigation with no frame name (window.open("url", "_blank"))
	if frameName == "_blank" {
		isRegularPane = true
		log.Printf("[webkit] _blank frame detected - will handle as new panel")
	} else if userGesture && hasCtrlOrCmd && (navType == 0 || navType == 5) {
		// WEBKIT_NAVIGATION_TYPE_LINK_CLICKED = 0, WEBKIT_NAVIGATION_TYPE_OTHER = 5
		isRegularPane = true
		log.Printf("[webkit] Ctrl/Cmd+click detected - will handle as new panel")
	} else if userGesture && navType == 5 && frameName == "" {
		// window.open("url", "_blank") typically: user gesture + OTHER + no frame name
		isRegularPane = true
		log.Printf("[webkit] User-initiated window.open() detected - will handle as new panel")
	}

	// Handle _blank targets by creating new panels directly (prevent WindowFeatures crash)
	if isRegularPane {
		log.Printf("[webkit] Creating new panel for _blank target: %s", uri)

		// Create new panel through workspace manager using existing popup handler
		if vw.popupHandler != nil {
			// Call popup handler to create a new panel
			// The workspace manager will handle this as a regular panel based on URI markers
			vw.popupHandler(uri + "#__dumber_frame_blank")
		}

		return 1 // TRUE - we handled it by creating a new panel
	}

	// For actual popups, let the create signal handle it
	if vw.popupHandler != nil {
		log.Printf("[webkit] Letting popup go through create signal for WindowFeatures handling: %s", uri)
		return 0 // FALSE - let WebKit call create signal for popup handling
	} else {
		log.Printf("[webkit] No popup handler registered - allowing native popup for: %s", uri)
		return 0 // FALSE - no handler, allow default popup
	}
}

//export goHandleLoadChanged
func goHandleLoadChanged(id C.ulong, curi *C.char, loadEvent C.int) {
	uid := uintptr(id)
	regMu.RLock()
	vw := viewByID[uid]
	regMu.RUnlock()

	if vw == nil {
		return
	}

	// OAuth auto-close is handled via WebKit's "close" signal when providers call window.close()
	// No need to detect OAuth callback URLs - let the providers handle it
	_ = curi // Suppress unused parameter warning
}

//export goHandleWebViewClose
func goHandleWebViewClose(id C.ulong) {
	uid := uintptr(id)
	regMu.RLock()
	vw := viewByID[uid]
	regMu.RUnlock()

	if vw == nil {
		log.Printf("[webkit] WebView close signal: no WebView found for ID %d", uid)
		return
	}

	log.Printf("[webkit] WebView close signal received for view_id: %d", uid)

	if vw.closeHandler != nil {
		log.Printf("[webkit] Calling close handler for view_id: %d", uid)
		vw.closeHandler()
	} else {
		log.Printf("[webkit] No close handler registered for view_id: %d", uid)
	}
}

//export goCreateNewPanel
func goCreateNewPanel(id C.ulong, curi *C.char) {
	uid := uintptr(id)
	regMu.RLock()
	vw := viewByID[uid]
	regMu.RUnlock()

	if vw == nil {
		log.Printf("[webkit] Create panel: no WebView found for ID %d", uid)
		return
	}

	uri := ""
	if curi != nil {
		uri = C.GoString(curi)
	}

	log.Printf("[webkit] Creating new panel for _blank target: %s", uri)

	// Create new panel through popup handler if available
	if vw.popupHandler != nil {
		vw.popupHandler(uri)
		log.Printf("[webkit] New panel creation handled by popup handler")
	} else {
		log.Printf("[webkit] No popup handler available for panel creation")
	}
}

//export goHandlePopupGeometry
func goHandlePopupGeometry(id C.ulong, x, y, width, height C.int) {
	// Store geometry for workspace manager to use
	uid := uintptr(id)
	regMu.Lock()
	if vw := viewByID[uid]; vw != nil {
		// For now, we'll store this in a simple way
		// The workspace manager will handle the geometry
		log.Printf("[webkit] Popup geometry: %dx%d at (%d,%d)",
			int(width), int(height), int(x), int(y))
	}
	regMu.Unlock()
}

//export goOnButtonPress
func goOnButtonPress(id C.ulong, button C.guint, state C.GdkModifierType) {
	uid := uintptr(id)
	btn := uint(button)
	log.Printf("[mouse] button=%d state=0x%x", btn, uint(state))
	regMu.RLock()
	vw := viewByID[uid]
	regMu.RUnlock()
	if vw == nil {
		return
	}
	switch btn {
	case 8: // Back button on many mice
		_ = vw.GoBack()
	case 9: // Forward button on many mice
		_ = vw.GoForward()
	}
}

//export goQuitMainLoop
func goQuitMainLoop() {
	log.Printf("[webkit] Window close requested - quitting main loop")
	QuitMainLoop()
}

// handleRenderingBackendMessage processes rendering backend detection messages
func handleRenderingBackendMessage(payload string) bool {
	// Parse JSON to check if this is a rendering backend detection message
	if !strings.Contains(payload, "rendering_backend_detection") {
		return false
	}

	// Simple JSON parsing for the specific structure we expect
	// {"type":"rendering_backend_detection","data":{...}}
	var msg struct {
		Type string `json:"type"`
		Data struct {
			WebGLAvailable         bool   `json:"webgl_available"`
			Renderer               string `json:"renderer"`
			Vendor                 string `json:"vendor"`
			Version                string `json:"version"`
			ShadingLanguageVersion string `json:"shading_language_version"`
			MaxTextureSize         int    `json:"max_texture_size"`
			HardwareAccelerated    bool   `json:"hardware_accelerated"`
		} `json:"data"`
	}

	if err := json.Unmarshal([]byte(payload), &msg); err != nil {
		log.Printf("[webkit] Failed to parse rendering backend message: %v", err)
		return false
	}

	if msg.Type != "rendering_backend_detection" {
		return false
	}

	// Log the detected rendering backend information
	backendType := "SOFTWARE"
	if msg.Data.HardwareAccelerated {
		backendType = "HARDWARE"
	}

	log.Printf("[webkit] Detected rendering backend: %s", backendType)
	log.Printf("[webkit] WebGL available: %v", msg.Data.WebGLAvailable)
	log.Printf("[webkit] Renderer: %s", msg.Data.Renderer)
	log.Printf("[webkit] Vendor: %s", msg.Data.Vendor)
	log.Printf("[webkit] Version: %s", msg.Data.Version)
	log.Printf("[webkit] Max texture size: %d", msg.Data.MaxTextureSize)

	return true
}

// handleKeyboardBlockingMessage handles keyboard blocking control messages from omnibox
func handleKeyboardBlockingMessage(vw *WebView, payload string) bool {
	if vw == nil {
		return false
	}

	var msg struct {
		Type   string `json:"type"`
		Action string `json:"action"`
	}

	if err := json.Unmarshal([]byte(payload), &msg); err != nil {
		return false
	}

	if msg.Type != "keyboard_blocking" {
		return false
	}

	switch msg.Action {
	case "enable":
		log.Printf("[webkit] Enabling page keyboard blocking for omnibox")
		if err := vw.EnablePageKeyboardBlocking(); err != nil {
			log.Printf("[webkit] Failed to enable keyboard blocking: %v", err)
		}
	case "disable":
		log.Printf("[webkit] Disabling page keyboard blocking")
		if err := vw.DisablePageKeyboardBlocking(); err != nil {
			log.Printf("[webkit] Failed to disable keyboard blocking: %v", err)
		}
	default:
		log.Printf("[webkit] Unknown keyboard blocking action: %s", msg.Action)
	}

	return true
}
