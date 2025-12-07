package webkit

import (
	"runtime"

	glib "github.com/diamondburned/gotk4/pkg/glib/v2"
	gtk "github.com/diamondburned/gotk4/pkg/gtk/v4"
)

var (
	mainLoop      *glib.MainLoop
	isInitialized bool
	mainContext   *glib.MainContext
)

// InitMainThread locks the current goroutine to the OS thread for GTK operations
// and initializes GTK. This must be called before any GTK operations.
func InitMainThread() {
	if !isInitialized {
		runtime.LockOSThread()

		// Initialize GTK - this is required before creating any GTK widgets
		gtk.Init()

		// Cache the default main context so we can check ownership later.
		mainContext = glib.MainContextDefault()

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

// IsMainThread returns true if called from the GTK main thread.
func IsMainThread() bool {
	if !isInitialized {
		return false
	}
	if mainContext == nil {
		mainContext = glib.MainContextDefault()
	}
	return mainContext != nil && mainContext.IsOwner()
}

// RunOnMainThread executes a function on the GTK main thread.
// If already on the main thread, executes immediately.
// Otherwise, schedules the function via glib.IdleAdd.
func RunOnMainThread(fn func()) {
	if IsMainThread() {
		fn()
		return
	}

	glib.IdleAdd(func() bool {
		fn()
		return false // Remove the idle handler after execution
	})
}
