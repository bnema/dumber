//go:build webkit_cgo

package webkit

/*
#cgo pkg-config: webkitgtk-6.0 gtk4
#include <webkit/webkit.h>
#include <gtk/gtk.h>
*/
import "C"

// This file documents intended CGO linkage. It is excluded from builds unless
// the 'webkit_cgo' build tag is specified. Real implementations will move APIs
// from the stubs into CGO-backed functions and types.
