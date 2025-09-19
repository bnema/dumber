//go:build webkit_cgo

package webkit

/*
#cgo pkg-config: webkitgtk-6.0 gtk4
#cgo CFLAGS: -I/usr/include/webkitgtk-6.0
#include <stdlib.h>
#include <gtk/gtk.h>
*/
import "C"

import (
	"sync"
	"unsafe"
)

type Window struct {
	win                *C.GtkWidget
	shortcutController *C.GtkShortcutController
	shortcutCallbacks  map[string]func()
	shortcutHandles    map[string]uintptr
	shortcutMu         sync.Mutex
}

func NewWindow(title string) (*Window, error) {
	if C.gtk_init_check() == 0 {
		return nil, ErrNotImplemented
	}
	// GTK4: gtk_window_new() returns GtkWindow*, no toplevel enum
	w := (*C.GtkWidget)(unsafe.Pointer(C.gtk_window_new()))
	if w == nil {
		return nil, ErrNotImplemented
	}
	ctitle := C.CString(title)
	defer C.free(unsafe.Pointer(ctitle))
	C.gtk_window_set_title((*C.GtkWindow)(unsafe.Pointer(w)), (*C.gchar)(ctitle))
	return &Window{win: w}, nil
}

func (w *Window) SetTitle(title string) {
	if w == nil || w.win == nil {
		return
	}
	ctitle := C.CString(title)
	defer C.free(unsafe.Pointer(ctitle))
	C.gtk_window_set_title((*C.GtkWindow)(unsafe.Pointer(w.win)), (*C.gchar)(ctitle))
}

// SetChild assigns the given widget as the GtkWindow child. Passing 0 removes the child.
func (w *Window) SetChild(child uintptr) {
	if w == nil || w.win == nil {
		return
	}
	var widget *C.GtkWidget
	if child != 0 {
		widget = (*C.GtkWidget)(unsafe.Pointer(child))
	}
	C.gtk_window_set_child((*C.GtkWindow)(unsafe.Pointer(w.win)), widget)
}
