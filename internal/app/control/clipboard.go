package control

import (
	"log"

	"github.com/bnema/dumber/pkg/clipboard"
	"github.com/bnema/dumber/pkg/webkit"
)

// ClipboardController handles clipboard operations
type ClipboardController struct {
	webView *webkit.WebView
}

// NewClipboardController creates a new clipboard controller
func NewClipboardController(webView *webkit.WebView) *ClipboardController {
	return &ClipboardController{
		webView: webView,
	}
}

// CopyCurrentURL copies the current WebView URL to the clipboard
func (c *ClipboardController) CopyCurrentURL() {
	log.Printf("Shortcut: Copy URL")
	currentURL := c.webView.GetCurrentURL()
	if currentURL == "" {
		log.Printf("No URL to copy")
		_ = c.webView.InjectScript(`(window.__dumber?.toast?.show ? window.__dumber.toast.show("No URL to copy", 2000) : (window.__dumber_showToast && window.__dumber_showToast("No URL to copy", 2000)))`)
		return
	}

	if err := clipboard.CopyToClipboard(currentURL); err != nil {
		log.Printf("Failed to copy URL to clipboard: %v", err)
		_ = c.webView.InjectScript(`(window.__dumber?.toast?.show ? window.__dumber.toast.show("Failed to copy URL", 2000) : (window.__dumber_showToast && window.__dumber_showToast("Failed to copy URL", 2000)))`)
	} else {
		log.Printf("URL copied to clipboard: %s", currentURL)
		_ = c.webView.InjectScript(`(window.__dumber?.toast?.show ? window.__dumber.toast.show("URL copied to clipboard", 2000) : (window.__dumber_showToast && window.__dumber_showToast("URL copied to clipboard", 2000)))`)
	}
}
