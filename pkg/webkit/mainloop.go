package webkit

import (
	"runtime"

	glib "github.com/diamondburned/gotk4/pkg/glib/v2"
	gtk "github.com/diamondburned/gotk4/pkg/gtk/v4"
)

var (
	mainLoop      *glib.MainLoop
	isInitialized bool
)

// InitMainThread locks the current goroutine to the OS thread for GTK operations
// and initializes GTK. This must be called before any GTK operations.
func InitMainThread() {
	if !isInitialized {
		runtime.LockOSThread()

		// Initialize GTK - this is required before creating any GTK widgets
		gtk.Init()

		isInitialized = true
	}
}

// RunMainLoop starts the GTK main event loop.
// This function blocks until QuitMainLoop is called.
func RunMainLoop() {
	InitMainThread()

	if mainLoop == nil {
		mainLoop = glib.NewMainLoop(nil, false)
	}

	mainLoop.Run()
}

// QuitMainLoop stops the GTK main event loop.
func QuitMainLoop() {
	if mainLoop != nil {
		mainLoop.Quit()
	}
}

// IsMainThread returns true if GTK has been initialized.
// Note: This doesn't truly detect the main thread, but is used as a guard
// for whether GTK operations are safe. All GTK operations should use
// RunOnMainThread or IdleAdd to be thread-safe.
func IsMainThread() bool {
	return isInitialized
}

// RunOnMainThread schedules a function to run on the GTK main thread.
// Always uses glib.IdleAdd to ensure thread safety.
// The function will execute during the next main loop iteration.
func RunOnMainThread(fn func()) {
	glib.IdleAdd(func() bool {
		fn()
		return false // Remove the idle handler after execution
	})
}
