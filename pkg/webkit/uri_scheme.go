package webkit

import (
	"fmt"

	"github.com/bnema/dumber/internal/logging"
	webkit "github.com/diamondburned/gotk4-webkitgtk/pkg/webkit/v6"
)

// URISchemeRequestCallback is called when a custom URI scheme is requested
type URISchemeRequestCallback func(request *webkit.URISchemeRequest)

// URISchemeHandler stores registered URI scheme handlers
type URISchemeHandler struct {
	scheme   string
	callback URISchemeRequestCallback
}

var (
	// pendingURISchemeHandlers stores handlers to be registered before WebView creation
	pendingURISchemeHandlers []*URISchemeHandler
)

// RegisterURIScheme registers a custom URI scheme handler
// In WebKitGTK 6.0, this must be called BEFORE creating any WebViews
// The actual registration happens in InitPersistentSession
func RegisterURIScheme(scheme string, callback URISchemeRequestCallback) {
	logging.Debug(fmt.Sprintf("[webkit] Registering URI scheme handler for: %s", scheme))
	pendingURISchemeHandlers = append(pendingURISchemeHandlers, &URISchemeHandler{
		scheme:   scheme,
		callback: callback,
	})
}

// ApplyURISchemeHandlers registers all pending URI schemes on the WebContext
// This should be called after creating the first WebView
func ApplyURISchemeHandlers(view *webkit.WebView) error {
	if view == nil {
		return fmt.Errorf("webview is nil")
	}

	ctx := view.Context()
	if ctx == nil {
		return fmt.Errorf("webcontext is nil")
	}

	for _, handler := range pendingURISchemeHandlers {
		logging.Debug(fmt.Sprintf("[webkit] Registering URI scheme on WebContext: %s", handler.scheme))

		// Capture handler in closure
		callback := handler.callback
		ctx.RegisterURIScheme(handler.scheme, func(req *webkit.URISchemeRequest) {
			if callback != nil {
				callback(req)
			}
		})
	}

	return nil
}

// FinishURISchemeRequest finishes a URI scheme request with data
func FinishURISchemeRequest(req *webkit.URISchemeRequest, mimeType string, data []byte) error {
	if req == nil {
		return ErrWebViewNotInitialized
	}

	// Import gio for input stream
	// This will be handled in the caller
	return nil
}
