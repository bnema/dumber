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
		c.showToast("No URL to copy", "error")
		return
	}

	if err := clipboard.CopyToClipboard(currentURL); err != nil {
		log.Printf("Failed to copy URL to clipboard: %v", err)
		c.showToast("Failed to copy URL", "error")
	} else {
		log.Printf("URL copied to clipboard: %s", currentURL)
		c.showToast("URL copied to clipboard", "success")
	}
}

func (c *ClipboardController) showToast(message, toastType string) {
	if c == nil || c.webView == nil {
		return
	}

	detail := map[string]any{
		"message":  message,
		"duration": 2000,
	}
	if toastType != "" {
		detail["type"] = toastType
	}

	if err := c.webView.DispatchCustomEvent("dumber:showToast", detail); err != nil {
		log.Printf("Failed to dispatch toast event: %v", err)
	}
}
