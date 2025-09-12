//go:build !webkit_cgo

package webkit

// InitWebKitLogCapture initializes the WebKit log capture system (stub version)
func InitWebKitLogCapture() error {
	// No-op for stub version
	return nil
}

// StopWebKitLogCapture stops the WebKit log capture (stub version)
func StopWebKitLogCapture() {
	// No-op for stub version
}

// StartWebKitOutputCapture starts capturing webkit stdout/stderr (stub version)
func StartWebKitOutputCapture() {
	// No-op for stub version
}

// IsWebKitLogCaptureActive returns whether WebKit log capture is active (stub version)
func IsWebKitLogCaptureActive() bool {
	return false
}
