//go:build webkit_cgo

package webkit

/*
#cgo pkg-config: gtk4 webkitgtk-6.0
#include <gtk/gtk.h>
#include <stdint.h>
#include <stdlib.h>

extern void goHandleWindowShortcut(uintptr_t handle);

static GtkShortcutController* create_global_shortcut_controller(GtkWindow* window) {
    if (!window) {
        return NULL;
    }
    GtkEventController* event_controller = gtk_shortcut_controller_new();
    if (!event_controller) {
        return NULL;
    }
    GtkShortcutController* controller = GTK_SHORTCUT_CONTROLLER(event_controller);
    gtk_shortcut_controller_set_scope(controller, GTK_SHORTCUT_SCOPE_GLOBAL);
    gtk_widget_add_controller(GTK_WIDGET(window), event_controller);
    return controller;
}

static gboolean shortcut_callback_bridge(GtkWidget* widget, GVariant* args, gpointer user_data) {
    (void)widget;
    (void)args;
    uintptr_t handle = (uintptr_t)user_data;
    if (handle == 0) {
        return FALSE;
    }
    goHandleWindowShortcut(handle);
    return TRUE;  // Indicate we handled the shortcut
}

static void add_shortcut_to_controller(GtkShortcutController* controller,
                                const char* trigger_str,
                                uintptr_t handle) {
    if (!controller || !trigger_str || handle == 0) {
        return;
    }
    GtkShortcutTrigger* trigger = gtk_shortcut_trigger_parse_string(trigger_str);
    if (!trigger) {
        return;
    }
    GtkShortcutAction* action = gtk_callback_action_new((GtkShortcutFunc)shortcut_callback_bridge,
                                                        (gpointer)handle,
                                                        NULL);
    if (!action) {
        g_object_unref(trigger);
        return;
    }
    GtkShortcut* shortcut = gtk_shortcut_new(trigger, action);
    gtk_shortcut_controller_add_shortcut(controller, shortcut);
}
*/
import "C"

import (
	"log"
	"sync"
	"sync/atomic"
	"unsafe"
)

var (
	globalShortcutMu        sync.Mutex
	globalShortcutCounter   uint64
	globalShortcutCallbacks = make(map[uintptr]func())
)

func registerWindowShortcutCallback(cb func()) uintptr {
	if cb == nil {
		return 0
	}
	id := uintptr(atomic.AddUint64(&globalShortcutCounter, 1))
	if id == 0 {
		// Should never happen, but guard against wrapping to 0
		id = uintptr(atomic.AddUint64(&globalShortcutCounter, 1))
	}
	globalShortcutMu.Lock()
	globalShortcutCallbacks[id] = cb
	globalShortcutMu.Unlock()
	return id
}

func unregisterWindowShortcutCallback(id uintptr) {
	if id == 0 {
		return
	}
	globalShortcutMu.Lock()
	delete(globalShortcutCallbacks, id)
	globalShortcutMu.Unlock()
}

func invokeWindowShortcutCallback(id uintptr) {
	if id == 0 {
		return
	}
	globalShortcutMu.Lock()
	cb := globalShortcutCallbacks[id]
	globalShortcutMu.Unlock()
	if cb == nil {
		log.Printf("[shortcuts] no handler for shortcut id=%d", id)
		return
	}
	cb()
}

//export goHandleWindowShortcut
func goHandleWindowShortcut(handle C.uintptr_t) {
	invokeWindowShortcutCallback(uintptr(handle))
}

// Go wrapper functions for CGO - these can be called from other Go files

// CreateGlobalShortcutController creates a GTK4 global shortcut controller for a window
func CreateGlobalShortcutController(window uintptr) uintptr {
	if window == 0 {
		return 0
	}
	controller := C.create_global_shortcut_controller((*C.GtkWindow)(unsafe.Pointer(window)))
	return uintptr(unsafe.Pointer(controller))
}

// AddShortcutToController adds a shortcut to the controller with a callback handle
func AddShortcutToController(controller uintptr, key string, handle uintptr) error {
	if controller == 0 || key == "" || handle == 0 {
		return ErrNotImplemented
	}

	keyStr := C.CString(key)
	defer C.free(unsafe.Pointer(keyStr))

	C.add_shortcut_to_controller(
		(*C.GtkShortcutController)(unsafe.Pointer(controller)),
		keyStr,
		C.uintptr_t(handle),
	)

	return nil
}
