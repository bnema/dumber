package handlers

import (
	"fmt"

	"github.com/bnema/dumber/internal/logging"
)

// NavigationMessage contains fields for navigation operations.
type NavigationMessage struct {
	URL string `json:"url"`
}

// HandleNavigation processes navigation requests from the frontend.
func HandleNavigation(c *Context, msg NavigationMessage) {
	if c.NavController != nil {
		if err := c.NavController.NavigateToURL(msg.URL); err != nil {
			logging.Error(fmt.Sprintf("[handlers] Navigation controller failed for input %q: %v", msg.URL, err))
			HandleLegacyNavigation(c, msg)
		}
		return
	}

	HandleLegacyNavigation(c, msg)
}

// HandleLegacyNavigation uses Parser for navigation when NavController is unavailable.
func HandleLegacyNavigation(c *Context, msg NavigationMessage) {
	if c.ParserService == nil {
		return
	}

	res, err := c.ParserService.ParseInput(c.Ctx(), msg.URL)
	if err != nil {
		logging.Error(fmt.Sprintf("[handlers] Legacy navigation parse failed for %q: %v", msg.URL, err))
		return
	}

	targetURL := res.URL

	if c.BrowserService != nil {
		if _, navErr := c.BrowserService.Navigate(c.Ctx(), targetURL); navErr != nil {
			logging.Warn(fmt.Sprintf("[handlers] Warning: failed to navigate to %s: %v", targetURL, navErr))
		}
	}

	if c.WebView == nil {
		return
	}

	if err := c.WebView.LoadURL(targetURL); err != nil {
		logging.Error(fmt.Sprintf("[handlers] Legacy LoadURL failed for %s: %v", targetURL, err))
	}

	if c.BrowserService != nil {
		if z, zerr := c.BrowserService.GetZoomLevel(c.Ctx(), targetURL); zerr == nil {
			if err := c.WebView.SetZoom(z); err != nil {
				logging.Error(fmt.Sprintf("[handlers] Legacy SetZoom failed for %s: %v", targetURL, err))
			}
		}
	}
}
