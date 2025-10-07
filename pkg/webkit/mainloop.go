package webkit

import (
	"runtime"

	"github.com/diamondburned/gotk4/pkg/glib/v2"
)

var (
	mainLoop      *glib.MainLoop
	mainThreadID  = runtime.LockOSThread // Store the main thread
	isInitialized bool
)

// InitMainThread locks the current goroutine to the OS thread for GTK operations.
// This must be called before any GTK operations.
func InitMainThread() {
	if !isInitialized {
		runtime.LockOSThread()
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
	// In gotk4, we're always on the main thread since we lock it
	return isInitialized
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
