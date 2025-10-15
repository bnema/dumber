package webkit

/*
#cgo pkg-config: gtk4
#include <gtk/gtk.h>
#include <stdlib.h>

// Create a GtkAlertDialog with a message
static GtkAlertDialog* create_alert_dialog(const char* message) {
    return gtk_alert_dialog_new("%s", message);
}
*/
import "C"
import (
	"unsafe"

	"github.com/diamondburned/gotk4/pkg/core/glib"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// newAlertDialog creates a new GTK4 AlertDialog using CGO
func newAlertDialog(message string) *gtk.AlertDialog {
	cstr := C.CString(message)
	defer C.free(unsafe.Pointer(cstr))

	cDialog := C.create_alert_dialog(cstr)
	if cDialog == nil {
		return nil
	}

	// Wrap the C object in a Go AlertDialog
	obj := glib.AssumeOwnership(unsafe.Pointer(cDialog))
	return &gtk.AlertDialog{Object: obj}
}
