package browser

import (
	"log"

	"github.com/bnema/dumber/internal/app/control"
	"github.com/bnema/dumber/pkg/webkit"
)

// ShortcutHandler manages keyboard shortcuts for the browser
type ShortcutHandler struct {
	webView             *webkit.WebView
	clipboardController *control.ClipboardController
}

// NewShortcutHandler creates a new shortcut handler
func NewShortcutHandler(webView *webkit.WebView, clipboardController *control.ClipboardController) *ShortcutHandler {
	return &ShortcutHandler{
		webView:             webView,
		clipboardController: clipboardController,
	}
}

// RegisterShortcuts registers all keyboard shortcuts
func (s *ShortcutHandler) RegisterShortcuts() {
	// DevTools
	_ = s.webView.RegisterKeyboardShortcut("F12", func() {
		log.Printf("Shortcut: F12 (devtools)")
		_ = s.webView.ShowDevTools()
	})

	// Omnibox (Ctrl+L): rely on injected script listener
	_ = s.webView.RegisterKeyboardShortcut("cmdorctrl+l", func() {
		log.Printf("Shortcut: Omnibox toggle")
		_ = s.webView.InjectScript("window.__dumber_toggle && window.__dumber_toggle()")
	})

	// Copy URL (Ctrl+Shift+C)
	_ = s.webView.RegisterKeyboardShortcut("cmdorctrl+shift+c", func() {
		s.clipboardController.CopyCurrentURL()
	})

	// Page refresh shortcuts
	_ = s.webView.RegisterKeyboardShortcut("cmdorctrl+r", func() {
		log.Printf("Shortcut: Reload page")
		_ = s.webView.Reload()
	})

	_ = s.webView.RegisterKeyboardShortcut("cmdorctrl+shift+r", func() {
		log.Printf("Shortcut: Hard reload page")
		_ = s.webView.ReloadBypassCache()
	})

	_ = s.webView.RegisterKeyboardShortcut("F5", func() {
		log.Printf("Shortcut: F5 reload")
		_ = s.webView.Reload()
	})

	// Zoom handled natively in webkit package (built-in shortcuts)
}
