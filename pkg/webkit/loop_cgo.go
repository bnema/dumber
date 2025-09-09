//go:build webkit_cgo

package webkit

/*
#cgo pkg-config: gtk+-3.0
#include <gtk/gtk.h>
*/
import "C"

// RunMainLoop enters the GTK main loop and blocks until quit.
func RunMainLoop() {
    C.gtk_main()
}

// QuitMainLoop requests the GTK main loop to exit.
func QuitMainLoop() {
    C.gtk_main_quit()
}

