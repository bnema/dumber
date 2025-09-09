//go:build !webkit_cgo

package webkit

// IsNativeAvailable reports whether the native WebKit2GTK backend is compiled in.
// In non-CGO builds, this returns false and WebView methods are logical no-ops.
func IsNativeAvailable() bool { return false }

