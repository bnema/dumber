//go:build !webkit_cgo

package webkit

import "log"

// RunMainLoop is a no-op in non-CGO builds.
func RunMainLoop() {
	log.Printf("[webkit] RunMainLoop (non-CGO): no GUI loop available")
}

// QuitMainLoop is a no-op in non-CGO builds.
func QuitMainLoop() {}
