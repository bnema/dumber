//go:build webkit_cgo

package webkit

/*
#cgo pkg-config: webkit2gtk-4.0 gtk+-3.0
#include <stdlib.h>
#include <gtk/gtk.h>
*/
import "C"

import "unsafe"

type Window struct {
    win *C.GtkWidget
}

func NewWindow(title string) (*Window, error) {
    if C.gtk_init_check(nil, nil) == 0 {
        return nil, ErrNotImplemented
    }
    w := C.gtk_window_new(C.GTK_WINDOW_TOPLEVEL)
    if w == nil {
        return nil, ErrNotImplemented
    }
    ctitle := C.CString(title)
    defer C.free(unsafe.Pointer(ctitle))
    C.gtk_window_set_title((*C.GtkWindow)(unsafe.Pointer(w)), (*C.gchar)(ctitle))
    return &Window{win: w}, nil
}

func (w *Window) SetTitle(title string) {
    if w == nil || w.win == nil { return }
    ctitle := C.CString(title)
    defer C.free(unsafe.Pointer(ctitle))
    C.gtk_window_set_title((*C.GtkWindow)(unsafe.Pointer(w.win)), (*C.gchar)(ctitle))
}
