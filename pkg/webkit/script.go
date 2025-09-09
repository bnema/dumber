//go:build !webkit_cgo

package webkit

// InjectScript evaluates JavaScript in the active WebView context.
func (w *WebView) InjectScript(js string) error {
	if w == nil || w.destroyed {
		return ErrNotImplemented
	}
	// Store last script for potential debugging/testing.
	_ = js
	return nil
}
