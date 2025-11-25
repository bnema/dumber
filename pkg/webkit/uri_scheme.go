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
	// pendingCorsEnabledSchemes stores schemes that should be marked as CORS-enabled on the WebContext
	pendingCorsEnabledSchemes = make(map[string]bool)
	// uriSchemesApplied tracks whether URI schemes have been registered to prevent duplicate registration
	uriSchemesApplied = false
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

// RegisterCorsEnabledURIScheme marks a scheme to be registered as "CORS-enabled" on the WebKit SecurityManager.
// This is CRITICAL for ES6 module loading from custom URI schemes.
// Without this, module imports from the custom scheme will fail silently due to CORS restrictions.
func RegisterCorsEnabledURIScheme(scheme string) {
	log.Printf("[webkit] Marking URI scheme as CORS-enabled: %s", scheme)
	pendingCorsEnabledSchemes[scheme] = true
}

// ApplyURISchemeHandlers registers all pending URI schemes on the WebContext
// This should be called after creating the first WebView
// This function is idempotent - it will only register schemes once per application lifetime
func ApplyURISchemeHandlers(view *webkit.WebView) error {
	// Check if schemes have already been applied (idempotent)
	if uriSchemesApplied {
		log.Printf("[webkit] URI schemes already registered, skipping")
		return nil
	}

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

	// Mark secure and CORS-enabled schemes on the SecurityManager (if available)
	if sm := ctx.SecurityManager(); sm != nil {
		for scheme := range pendingSecureSchemes {
			log.Printf("[webkit] Registering URI scheme as secure: %s", scheme)
			sm.RegisterURISchemeAsSecure(scheme)
			log.Printf("[webkit] Verification - scheme %s is secure: %v", scheme, sm.URISchemeIsSecure(scheme))
		}
		for scheme := range pendingCorsEnabledSchemes {
			log.Printf("[webkit] Registering URI scheme as CORS-enabled: %s", scheme)
			sm.RegisterURISchemeAsCorsEnabled(scheme)
			log.Printf("[webkit] Verification - scheme %s is CORS-enabled: %v", scheme, sm.URISchemeIsCorsEnabled(scheme))
		}
	} else {
		log.Printf("[webkit] WARNING: SecurityManager is nil, cannot register secure/CORS schemes!")
	}

	// Mark as applied so we don't try to register again
	uriSchemesApplied = true
	log.Printf("[webkit] URI scheme registration completed successfully")

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
