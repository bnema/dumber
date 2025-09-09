//go:build webkit_cgo

package webkit

// IsNativeAvailable reports whether the native WebKit2GTK backend is compiled in.
func IsNativeAvailable() bool { return true }
