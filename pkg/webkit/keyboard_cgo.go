//go:build webkit_cgo

package webkit

/*
#cgo pkg-config: gtk4
#include <gtk/gtk.h>
#include <gdk/gdk.h>

// Declarations for Go callbacks
extern void goOnKeyPress(unsigned long id, unsigned int keyval, GdkModifierType state);
extern void goOnButtonPress(unsigned long id, unsigned int button, GdkModifierType state);

// Key controller callback -> forward to Go
static gboolean on_key_pressed(GtkEventControllerKey* controller, guint keyval, guint keycode, GdkModifierType state, gpointer user_data) {
    (void)controller; (void)keycode;
    goOnKeyPress((unsigned long)user_data, keyval, state);
    return FALSE; // do not stop other handlers
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
    g_signal_connect_data(keyc, "key-pressed", G_CALLBACK(on_key_pressed), (gpointer)id, NULL, 0);
    gtk_widget_add_controller(widget, keyc);
}

static void attach_mouse_gesture(GtkWidget* widget, unsigned long id) {
    if (!widget) return;
    GtkGesture* click = gtk_gesture_click_new();
    g_signal_connect_data(click, "pressed", G_CALLBACK(on_click_pressed), (gpointer)id, NULL, 0);
    gtk_widget_add_controller(widget, GTK_EVENT_CONTROLLER(click));
}
*/
import "C"

import (
    "fmt"
    "log"
    "sync/atomic"
    "sync"
    "time"
)

// Registry of accelerators per view id.
type shortcutRegistry map[string]func()

var (
    regMu         sync.RWMutex
    viewShortcuts = make(map[uintptr]shortcutRegistry)
    viewIDCounter  uint64
    viewByID      = make(map[uintptr]*WebView)
    lastKeyTime   = make(map[uintptr]map[string]time.Time)
)

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
    return nil
}

//export goOnKeyPress
func goOnKeyPress(id C.ulong, keyval C.guint, state C.GdkModifierType) {
    // Normalize and dispatch
    uid := uintptr(id)
    kv := uint(keyval)
    st := uint(state)
    dispatchAccelerator(uid, kv, st)
}

func dispatchAccelerator(uid uintptr, keyval uint, state uint) {
    ctrl := (state & uint(C.GDK_CONTROL_MASK)) != 0
    // Map keyval to string names
    var keyName string
    switch keyval {
    case uint(C.GDK_KEY_plus), uint(C.GDK_KEY_KP_Add):
        keyName = "plus"
    case uint(C.GDK_KEY_equal):
        keyName = "="
    case uint(C.GDK_KEY_minus), uint(C.GDK_KEY_KP_Subtract), uint(C.GDK_KEY_underscore):
        // Treat underscore as minus for layouts where '-' requires Shift (e.g., AZERTY)
        keyName = "-"
    case uint(C.GDK_KEY_0), uint(C.GDK_KEY_KP_0):
        keyName = "0"
    case uint(C.GDK_KEY_F12):
        keyName = "F12"
    default:
        return
    }

    // Candidate accelerator strings in order
    candidates := []string{keyName}
    if ctrl {
        candidates = append([]string{"cmdorctrl+" + keyName, "ctrl+" + keyName}, candidates...)
    }

    regMu.RLock()
    reg := viewShortcuts[uid]
    regMu.RUnlock()
    if reg == nil {
        return
    }
    for _, name := range candidates {
        if cb, ok := reg[name]; ok {
            // Debounce duplicates within 120ms for the same view+key
            regMu.Lock()
            if _, ok := lastKeyTime[uid]; !ok { lastKeyTime[uid] = make(map[string]time.Time) }
            last := lastKeyTime[uid][name]
            now := time.Now()
            if now.Sub(last) < 120*time.Millisecond {
                // Skip duplicate
                regMu.Unlock()
                return
            }
            lastKeyTime[uid][name] = now
            regMu.Unlock()
            log.Printf("[accelerator] %s", name)
            cb()
            break
        }
    }
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
    log.Printf("[input] GTK4 key/mouse controllers attached")
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
    if int(preferDark) != 0 { dval = "true" }
    js := fmt.Sprintf(`(() => { try { const d=%s; const cs=d?'dark':'light'; console.log('[dumber] theme change:'+cs);
let m=document.querySelector('meta[name="color-scheme"]');
if(!m){ m=document.createElement('meta'); m.name='color-scheme'; document.head.appendChild(m); }
m.setAttribute('content', d ? 'dark light' : 'light dark');
let s=document.getElementById('__dumber_theme_style');
if(!s){ s=document.createElement('style'); s.id='__dumber_theme_style'; document.documentElement.appendChild(s); }
s.textContent=':root{color-scheme:' + (d?'dark':'light') + ';}';
} catch(e) { console.warn('[dumber] theme runtime update failed', e); } })();`, dval)
    _ = vw.InjectScript(js)
}

//export goOnUcmMessage
func goOnUcmMessage(id C.ulong, json *C.char) {
    uid := uintptr(id)
    regMu.RLock()
    vw := viewByID[uid]
    regMu.RUnlock()
    if vw == nil || json == nil { return }
    goPayload := C.GoString(json)
    vw.dispatchScriptMessage(goPayload)
}

//export goOnTitleChanged
func goOnTitleChanged(id C.ulong, ctitle *C.char) {
    uid := uintptr(id)
    regMu.RLock()
    vw := viewByID[uid]
    regMu.RUnlock()
    if vw == nil || ctitle == nil { return }
    title := C.GoString(ctitle)
    vw.dispatchTitleChanged(title)
}

//export goOnURIChanged
func goOnURIChanged(id C.ulong, curi *C.char) {
    uid := uintptr(id)
    regMu.RLock()
    vw := viewByID[uid]
    regMu.RUnlock()
    if vw == nil || curi == nil { return }
    uri := C.GoString(curi)
    vw.dispatchURIChanged(uri)
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
