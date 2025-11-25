package webkit

/*
#cgo pkg-config: webkitgtk-6.0
#include <webkit/webkit.h>
#include <stdlib.h>

// set_cors_allowlist sets the CORS allowlist on a WebView
// Takes a NULL-terminated array of strings (allowlist patterns)
static void set_cors_allowlist(WebKitWebView* view, const gchar** allowlist) {
	webkit_web_view_set_cors_allowlist(view, allowlist);
}
*/
import "C"
import (
	"fmt"
	"log"
	"unsafe"

	webkit "github.com/diamondburned/gotk4-webkitgtk/pkg/webkit/v6"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
)

// ExtensionViewType identifies the type of extension WebView to create
type ExtensionViewType int

const (
	// ExtensionViewBackground creates a background page WebView (no parent, extension mode + CSP)
	ExtensionViewBackground ExtensionViewType = iota
	// ExtensionViewPopup creates a popup/options page WebView (has parent via related-view, extension mode + CSP)
	ExtensionViewPopup
)

// ExtensionViewConfig configures creation of extension WebViews.
// Aligns with Epiphany's extension WebView creation pattern.
type ExtensionViewConfig struct {
	// Type specifies whether this is a background or popup view
	Type ExtensionViewType

	// CSP is the Content Security Policy from the extension manifest
	CSP string

	// ParentView is required for popups (used as related-view), nil for background pages
	ParentView *webkit.WebView

	// ExtensionID is used for logging and error messages
	ExtensionID string

	// CORSAllowlist contains URL patterns the extension has permission to access
	// This is set via webkit_web_view_set_cors_allowlist() to allow cross-origin requests
	// Example: []string{"https://*/*", "http://*/*", "dumb-extension://extension-id/*"}
	CORSAllowlist []string
}

// EnsureExtensionSchemeCorsEnabled ensures the dumb-extension:// scheme is marked as
// CORS-enabled on the given WebView's WebContext's SecurityManager.
// This is critical for ES6 module loading to work in extension WebViews.
//
// WebKit's ES6 module loader makes CORS decisions very early (before URI scheme requests),
// so the scheme must be registered as CORS-enabled on the specific WebContext instance
// that will host the extension page.
func EnsureExtensionSchemeCorsEnabled(view *webkit.WebView) error {
	if view == nil {
		return fmt.Errorf("webview is nil")
	}

	ctx := view.Context()
	if ctx == nil {
		return fmt.Errorf("webcontext is nil")
	}

	sm := ctx.SecurityManager()
	if sm == nil {
		return fmt.Errorf("security manager is nil")
	}

	// Check if already registered (avoid redundant calls)
	if sm.URISchemeIsCorsEnabled("dumb-extension") {
		log.Printf("[webkit] dumb-extension:// scheme already CORS-enabled on this WebContext")
		return nil
	}

	// Register as CORS-enabled for this WebContext's SecurityManager
	log.Printf("[webkit] Registering dumb-extension:// as CORS-enabled on WebView's WebContext")
	sm.RegisterURISchemeAsCorsEnabled("dumb-extension")

	// Verify it worked
	if !sm.URISchemeIsCorsEnabled("dumb-extension") {
		return fmt.Errorf("failed to register dumb-extension:// as CORS-enabled")
	}

	log.Printf("[webkit] Successfully registered dumb-extension:// as CORS-enabled")
	return nil
}

// SetCORSAllowlist sets the CORS allowlist on a WebView
// This allows the extension to make cross-origin requests to URLs matching the patterns
func SetCORSAllowlist(view *webkit.WebView, allowlist []string) {
	if view == nil || len(allowlist) == 0 {
		return
	}

	// Get native WebKitWebView pointer
	viewObj := glib.BaseObject(view)
	if viewObj == nil {
		log.Printf("[webkit] SetCORSAllowlist: failed to get view object")
		return
	}
	viewNative := (*C.WebKitWebView)(unsafe.Pointer(viewObj.Native()))
	if viewNative == nil {
		log.Printf("[webkit] SetCORSAllowlist: view native pointer is nil")
		return
	}

	// Convert Go string slice to NULL-terminated C string array
	// Allocate array with extra slot for NULL terminator
	cAllowlist := make([]*C.gchar, len(allowlist)+1)
	for i, pattern := range allowlist {
		cAllowlist[i] = (*C.gchar)(C.CString(pattern))
	}
	cAllowlist[len(allowlist)] = nil // NULL terminator

	// Set CORS allowlist
	C.set_cors_allowlist(viewNative, (**C.gchar)(unsafe.Pointer(&cAllowlist[0])))

	// Free C strings
	for i := 0; i < len(allowlist); i++ {
		C.free(unsafe.Pointer(cAllowlist[i]))
	}

	log.Printf("[webkit] CORS allowlist set for extension view: %v", allowlist)
}

// NewExtensionWebView creates a bare WebView configured for extension pages.
// Returns a bare WebView that must be wrapped and initialized with InitializeFromBare().
//
// For background pages (Type == ExtensionViewBackground):
//   - Sets web-extension-mode=MANIFESTV2
//   - Sets default-content-security-policy from manifest
//   - No related-view (background pages ARE the parent)
//
// For popup/options pages (Type == ExtensionViewPopup):
//   - Sets related-view to share session with ParentView (typically the background page)
//   - Sets web-extension-mode=MANIFESTV2
//   - Sets default-content-security-policy from manifest
//
// CRITICAL: This function also ensures the WebView's WebContext has the dumb-extension://
// scheme registered as CORS-enabled on its SecurityManager. This is essential for ES6
// module loading to work in extension pages.
//
// This mirrors Epiphany's ephy_web_extensions_manager_create_web_extensions_webview().
func NewExtensionWebView(cfg *ExtensionViewConfig) (*webkit.WebView, error) {
	if cfg == nil {
		return nil, fmt.Errorf("extension view config is nil")
	}

	var view *webkit.WebView

	switch cfg.Type {
	case ExtensionViewBackground:
		view = NewBareExtensionBackgroundWebView(cfg.CSP)
		if view == nil {
			return nil, fmt.Errorf("failed to create extension background WebView for %s", cfg.ExtensionID)
		}

	case ExtensionViewPopup:
		if cfg.ParentView != nil {
			// Use parent view for process sharing (traditional model)
			view = NewBareExtensionWebView(cfg.ParentView, cfg.CSP)
		} else {
			// No parent view (Goja-based background) - create standalone popup
			// This creates its own web context like a background page would
			view = NewBareExtensionBackgroundWebView(cfg.CSP)
		}
		if view == nil {
			return nil, fmt.Errorf("failed to create extension popup WebView for %s", cfg.ExtensionID)
		}

	default:
		return nil, fmt.Errorf("unknown extension view type: %d", cfg.Type)
	}

	// CRITICAL: Ensure dumb-extension:// scheme is CORS-enabled on this WebView's WebContext
	// This must be done immediately after WebView creation because WebKit's module loader
	// checks the SecurityManager's CORS-enabled flag very early (before URI scheme requests are made).
	// Without this, ES6 module imports will fail silently without ever reaching the scheme handler.
	if err := EnsureExtensionSchemeCorsEnabled(view); err != nil {
		log.Printf("[webkit] Warning: failed to register CORS for extension scheme: %v", err)
		// Non-fatal - continue but modules may not load
	}

	// Set CORS allowlist if provided (mirrors Epiphany's approach)
	if len(cfg.CORSAllowlist) > 0 {
		SetCORSAllowlist(view, cfg.CORSAllowlist)
	}

	return view, nil
}
