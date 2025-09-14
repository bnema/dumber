// Package webkit provides a WebKit2GTK-backed browser view and window.
package webkit

import "errors"

// ErrNotImplemented is returned by stub implementations pending WebKit2GTK migration.
var ErrNotImplemented = errors.New("webkit: not implemented")

// ErrWebViewNotInitialized is returned when WebView operations are attempted on uninitialized WebView
var ErrWebViewNotInitialized = errors.New("webkit: webview not initialized")

// ErrContentManagerNotFound is returned when WebKit user content manager cannot be retrieved
var ErrContentManagerNotFound = errors.New("webkit: user content manager not found")
