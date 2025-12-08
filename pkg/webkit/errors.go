package webkit

import (
	"errors"

	webkit "github.com/diamondburned/gotk4-webkitgtk/pkg/webkit/v6"
	"github.com/diamondburned/gotk4/pkg/core/gerror"
)

var (
	ErrWebViewNotInitialized = errors.New("webkit: WebView not initialized")
	ErrWebViewDestroyed      = errors.New("webkit: WebView destroyed")
	ErrInvalidURL            = errors.New("webkit: invalid URL")
)

// IsExclusivelyCancelledError returns true if the error is ONLY a cancellation error.
// It checks both the error string and the GError code.
func IsExclusivelyCancelledError(err error) bool {
	if err == nil {
		return false
	}

	// 1. Check string message (fastest, covers many cases)
	if err.Error() == "Load request cancelled" {
		return true
	}

	// 2. Check GError code if available
	var gErr *gerror.GError
	if errors.As(err, &gErr) {
		// Calculate the expected quark/domain if possible, or just check the code.
		// WebKitNetworkErrorCancelled is 302 (WEBKIT_NETWORK_ERROR_CANCELLED)
		// We trust the code uniqueness within the WebKit domain context usually.
		if gErr.ErrorCode() == int(webkit.NetworkErrorCancelled) {
			return true
		}
	}

	return false
}
