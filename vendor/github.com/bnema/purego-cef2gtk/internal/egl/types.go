// Package egl contains small EGL helpers shared by GTK/GL probing and rendering.
package egl

import "fmt"

// Display is an EGLDisplay handle. Zero is EGL_NO_DISPLAY.
type Display uintptr

const NoDisplay Display = 0

func (d Display) Valid() bool { return d != NoDisplay }

func (d Display) String() string {
	if !d.Valid() {
		return "EGL_NO_DISPLAY"
	}
	return fmt.Sprintf("EGLDisplay(0x%x)", uintptr(d))
}
