package webkit

import (
	"fmt"

	webkit "github.com/diamondburned/gotk4-webkitgtk/pkg/webkit/v6"
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
// This mirrors Epiphany's ephy_web_extensions_manager_create_web_extensions_webview().
func NewExtensionWebView(cfg *ExtensionViewConfig) (*webkit.WebView, error) {
	if cfg == nil {
		return nil, fmt.Errorf("extension view config is nil")
	}

	switch cfg.Type {
	case ExtensionViewBackground:
		view := NewBareExtensionBackgroundWebView(cfg.CSP)
		if view == nil {
			return nil, fmt.Errorf("failed to create extension background WebView for %s", cfg.ExtensionID)
		}
		return view, nil

	case ExtensionViewPopup:
		if cfg.ParentView == nil {
			return nil, fmt.Errorf("extension popup WebView requires ParentView for %s", cfg.ExtensionID)
		}
		view := NewBareExtensionWebView(cfg.ParentView, cfg.CSP)
		if view == nil {
			return nil, fmt.Errorf("failed to create extension popup WebView for %s", cfg.ExtensionID)
		}
		return view, nil

	default:
		return nil, fmt.Errorf("unknown extension view type: %d", cfg.Type)
	}
}
