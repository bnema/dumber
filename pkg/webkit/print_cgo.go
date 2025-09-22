//go:build webkit_cgo

package webkit

/*
#cgo pkg-config: webkitgtk-6.0 gtk4
#cgo CFLAGS: -I/usr/include/webkitgtk-6.0
#include <webkit/webkit.h>
#include <gtk/gtk.h>
*/
import "C"
import "unsafe"

// ShowPrintDialog shows the print dialog for the WebView
func (w *WebView) ShowPrintDialog() error {
	if w == nil || w.native == nil || w.native.wv == nil {
		return ErrNotImplemented
	}

	// Create print operation
	printOp := C.webkit_print_operation_new(w.native.wv)
	if printOp == nil {
		return ErrNotImplemented
	}

	// Pass nil for parent window - print dialog will work without it
	var parentWindow *C.GtkWindow = nil

	// Show print dialog
	response := C.webkit_print_operation_run_dialog(printOp, parentWindow)

	// Clean up print operation
	C.g_object_unref(C.gpointer(unsafe.Pointer(printOp)))

	// Handle the response
	switch response {
	case C.WEBKIT_PRINT_OPERATION_RESPONSE_PRINT:
		// User clicked print - the operation will proceed
		return nil
	case C.WEBKIT_PRINT_OPERATION_RESPONSE_CANCEL:
		// User cancelled - not an error
		return nil
	default:
		return ErrNotImplemented
	}
}
