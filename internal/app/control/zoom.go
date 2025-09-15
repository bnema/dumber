package control

import (
	"context"
	"fmt"
	"log"

	"github.com/bnema/dumber/pkg/webkit"
	"github.com/bnema/dumber/services"
)

// ZoomController manages zoom functionality for the WebView
type ZoomController struct {
	currentURL         string
	lastZoomDomain     string
	programmaticChange bool
	browserService     *services.BrowserService
	webView            *webkit.WebView
}

// WebViewInterface defines the interface for WebView zoom operations
type WebViewInterface interface {
	GetCurrentURL() string
	SetZoom(level float64) error
	InjectScript(script string) error
	RegisterZoomChangedHandler(handler func(float64))
	RegisterURIChangedHandler(handler func(string))
}

// NewZoomController creates a new zoom controller
func NewZoomController(browserService *services.BrowserService, webView *webkit.WebView) *ZoomController {
	return &ZoomController{
		currentURL:     "dumb://homepage",
		browserService: browserService,
		webView:        webView,
	}
}

// RegisterHandlers sets up the zoom-related event handlers
func (z *ZoomController) RegisterHandlers() {
	z.webView.RegisterZoomChangedHandler(z.handleZoomChange)
	z.webView.RegisterURIChangedHandler(z.handleURIChange)
}

// handleURIChange responds to URL changes and applies saved zoom levels
func (z *ZoomController) handleURIChange(url string) {
	z.currentURL = url
	if url == "" {
		return
	}
	ctx := context.Background()
	currentDomain := services.ZoomKeyForLog(url)

	if zoomLevel, err := z.browserService.GetZoomLevel(ctx, url); err == nil {
		z.programmaticChange = true
		if err := z.webView.SetZoom(zoomLevel); err == nil {
			log.Printf("[zoom] loaded %.2f for %s", zoomLevel, currentDomain)

			// Only show toast when entering a new domain (not on first load)
			if currentDomain != z.lastZoomDomain && z.lastZoomDomain != "" {
				z.showZoomToast(zoomLevel)
			}
			z.lastZoomDomain = currentDomain
		}
		z.programmaticChange = false
	}
}

// handleZoomChange responds to zoom level changes and persists them
func (z *ZoomController) handleZoomChange(level float64) {
	url := z.webView.GetCurrentURL()
	if url == "" {
		return
	}
	ctx := context.Background()
	if err := z.browserService.SetZoomLevel(ctx, url, level); err != nil {
		log.Printf("[zoom] failed to save level %.2f for %s: %v", level, url, err)
		return
	}
	key := services.ZoomKeyForLog(url)
	log.Printf("[zoom] saved %.2f for %s", level, key)

	// Only show toast for user-initiated zoom changes, not programmatic ones
	if !z.programmaticChange {
		z.showZoomToast(level)
	}
}

// showZoomToast displays a zoom level notification using TypeScript toast system
func (z *ZoomController) showZoomToast(level float64) {
	log.Printf("[zoom] Attempting to show zoom toast for level %.2f", level)

	percentage := int(level * 100)

	// Enhanced approach that ensures toast always appears
	js := fmt.Sprintf(`(function() {
		try {
			console.log('[dumber] Attempting to show zoom toast...');
			const zoomLevel = %f;
			const percentage = %d;
			let toastShown = false;

			// Prefer unified page-world API
			if (window.__dumber && window.__dumber.toast && typeof window.__dumber.toast.zoom === 'function') {
				console.log('[dumber] Using unified toast.zoom');
				window.__dumber.toast.zoom(zoomLevel);
				toastShown = true;
			} else if (typeof window.__dumber_showZoomToast === 'function') {
				console.log('[dumber] Using legacy zoom toast');
				window.__dumber_showZoomToast(zoomLevel);
				toastShown = true;
			} else if (window.__dumber && window.__dumber.toast && typeof window.__dumber.toast.show === 'function') {
				console.log('[dumber] Using unified toast.show');
				window.__dumber.toast.show('Zoom: ' + percentage + '%%', 1500, 'info');
				toastShown = true;
			} else if (typeof window.__dumber_showToast === 'function') {
				console.log('[dumber] Using legacy toast.show');
				window.__dumber_showToast('Zoom: ' + percentage + '%%', 1500, 'info');
				toastShown = true;
			}


			if (!toastShown) {
				console.warn('[dumber] Failed to show zoom toast - no DOM body available');
			}

		} catch(e) {
			console.error('[dumber] Zoom toast error:', e);
		}
	})();`, level, percentage)

	if err := z.webView.InjectScript(js); err != nil {
		log.Printf("[zoom] failed to show toast: %v", err)
	} else {
		log.Printf("[zoom] Toast script injected successfully")
	}
}

// ApplyInitialZoom sets the initial zoom level for the current URL
func (z *ZoomController) ApplyInitialZoom() {
	ctx := context.Background()
	url := z.webView.GetCurrentURL()
	if url == "" {
		url = "dumb://homepage"
	}
	if zoomLevel, err := z.browserService.GetZoomLevel(ctx, url); err == nil {
		z.programmaticChange = true
		if err := z.webView.SetZoom(zoomLevel); err != nil {
			log.Printf("Warning: failed to set initial zoom: %v", err)
		} else {
			key := services.ZoomKeyForLog(url)
			log.Printf("[zoom] loaded %.2f for %s", zoomLevel, key)
		}
		z.programmaticChange = false
	}
}

// ApplyZoomForURL applies zoom for a specific URL (used for navigation)
func (z *ZoomController) ApplyZoomForURL(url string) {
	ctx := context.Background()
	if zoomLevel, err := z.browserService.GetZoomLevel(ctx, url); err == nil {
		z.programmaticChange = true
		if err := z.webView.SetZoom(zoomLevel); err != nil {
			log.Printf("Warning: failed to set zoom: %v", err)
		} else {
			key := services.ZoomKeyForLog(url)
			log.Printf("[zoom] loaded %.2f for %s", zoomLevel, key)
		}
		z.programmaticChange = false
	}
}
