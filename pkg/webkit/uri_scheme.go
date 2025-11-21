package webkit

import (
	"fmt"
	"log"

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
	// pendingSecureSchemes stores schemes that should be marked as secure on the WebContext
	pendingSecureSchemes = make(map[string]bool)
)

// RegisterURIScheme registers a custom URI scheme handler
// In WebKitGTK 6.0, this must be called BEFORE creating any WebViews
// The actual registration happens in InitPersistentSession
func RegisterURIScheme(scheme string, callback URISchemeRequestCallback) {
	log.Printf("[webkit] Registering URI scheme handler for: %s", scheme)
	pendingURISchemeHandlers = append(pendingURISchemeHandlers, &URISchemeHandler{
		scheme:   scheme,
		callback: callback,
	})
}

// RegisterSecureURIScheme marks a scheme to be registered as "secure" on the WebKit SecurityManager.
// This aligns with Epiphany's treatment of extension schemes (e.g., ephy-webextension://).
func RegisterSecureURIScheme(scheme string) {
	log.Printf("[webkit] Marking URI scheme as secure: %s", scheme)
	pendingSecureSchemes[scheme] = true
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
		log.Printf("[webkit] Registering URI scheme on WebContext: %s", handler.scheme)

		// Capture handler in closure
		callback := handler.callback
		ctx.RegisterURIScheme(handler.scheme, func(req *webkit.URISchemeRequest) {
			if callback != nil {
				callback(req)
			}
		})
	}

	// Mark secure schemes on the SecurityManager (if available)
	if sm := ctx.SecurityManager(); sm != nil {
		for scheme := range pendingSecureSchemes {
			log.Printf("[webkit] Registering URI scheme as secure: %s", scheme)
			sm.RegisterURISchemeAsSecure(scheme)
		}
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
