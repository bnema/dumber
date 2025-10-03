//go:build !webkit_cgo

package webkit

// IsNativeAvailable reports whether the native WebKit2GTK backend is compiled in.
// In non-CGO builds, this returns false and WebView methods are logical no-ops.
func IsNativeAvailable() bool { return false }

// InitMainThread is a no-op in non-CGO builds.
func InitMainThread() {}

// IsMainThread always returns true in non-CGO builds since there's no GTK main thread.
func IsMainThread() bool { return true }

// IterateMainLoop is a no-op in non-CGO builds and always returns false.
func IterateMainLoop() bool { return false }

// PanedGetStartChild is a no-op in non-CGO builds and returns 0.
func PanedGetStartChild(paned uintptr) uintptr { return 0 }

// PanedGetEndChild is a no-op in non-CGO builds and returns 0.
func PanedGetEndChild(paned uintptr) uintptr { return 0 }
