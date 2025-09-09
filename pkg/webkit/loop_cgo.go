//go:build webkit_cgo

package webkit

/*
#cgo pkg-config: gtk4
#include <gtk/gtk.h>
#include <glib.h>
*/
import "C"

var mainLoop *C.GMainLoop

// RunMainLoop enters the GLib main loop (GTK4) and blocks until quit.
func RunMainLoop() {
	if mainLoop == nil {
		mainLoop = C.g_main_loop_new(nil, C.gboolean(0))
	}
	C.g_main_loop_run(mainLoop)
}

// QuitMainLoop requests the main loop to exit.
func QuitMainLoop() {
	if mainLoop != nil {
		C.g_main_loop_quit(mainLoop)
	}
}
