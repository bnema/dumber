package webkit

import "errors"

var (
	// ErrNotImplemented indicates a feature is not yet implemented
	ErrNotImplemented = errors.New("webkit: not implemented")

	// ErrWebViewNotInitialized indicates the WebView was not properly initialized
	ErrWebViewNotInitialized = errors.New("webkit: webview not initialized")

	// ErrWebViewDestroyed indicates the WebView has been destroyed
	ErrWebViewDestroyed = errors.New("webkit: webview destroyed")

	// ErrInvalidURL indicates an invalid URL was provided
	ErrInvalidURL = errors.New("webkit: invalid URL")

	// ErrMainThreadRequired indicates an operation must be called from the main thread
	ErrMainThreadRequired = errors.New("webkit: operation must be called from main thread")
)
