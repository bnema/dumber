package control

import (
	"fmt"

	"github.com/bnema/dumber/internal/logging"
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
	logging.Debug(fmt.Sprintf("Shortcut: Copy URL"))
	currentURL := c.webView.GetCurrentURL()
	if currentURL == "" {
		logging.Warn(fmt.Sprintf("No URL to copy"))
		c.showToast("No URL to copy", "error")
		return
	}

	if err := clipboard.CopyToClipboard(currentURL); err != nil {
		logging.Error(fmt.Sprintf("Failed to copy URL to clipboard: %v", err))
		c.showToast("Failed to copy URL", "error")
	} else {
		logging.Debug(fmt.Sprintf("URL copied to clipboard: %s", currentURL))
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

	if err := c.webView.DispatchCustomEvent("dumber:toast", detail); err != nil {
		logging.Error(fmt.Sprintf("Failed to dispatch toast event: %v", err))
	}
}

// Detach releases the WebView reference when the pane is closed.
func (c *ClipboardController) Detach() {
	if c == nil {
		return
	}
	c.webView = nil
}
