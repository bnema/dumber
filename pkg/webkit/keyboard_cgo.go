//go:build webkit_cgo

package webkit

/*
#cgo pkg-config: gtk+-3.0
#include <gtk/gtk.h>
#include <gdk/gdk.h>
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
    case uint(C.GDK_KEY_minus), uint(C.GDK_KEY_KP_Subtract):
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
