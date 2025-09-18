package control

import (
	"context"
	"log"
	"os"

	"github.com/bnema/dumber/pkg/webkit"
	"github.com/bnema/dumber/internal/services"
)

// NavigationController manages navigation functionality
type NavigationController struct {
	parserService  *services.ParserService
	browserService *services.BrowserService
	webView        *webkit.WebView
	zoomController *ZoomController
}

// NewNavigationController creates a new navigation controller
func NewNavigationController(
	parserService *services.ParserService,
	browserService *services.BrowserService,
	webView *webkit.WebView,
	zoomController *ZoomController,
) *NavigationController {
	return &NavigationController{
		parserService:  parserService,
		browserService: browserService,
		webView:        webView,
		zoomController: zoomController,
	}
}

// NavigateToURL parses input and navigates to the resulting URL
func (n *NavigationController) NavigateToURL(input string) error {
	ctx := context.Background()
	result, err := n.parserService.ParseInput(ctx, input)
	if err != nil {
		return err
	}

	// Record navigation in browser service
	if _, navErr := n.browserService.Navigate(ctx, result.URL); navErr != nil {
		log.Printf("Warning: failed to navigate to %s: %v", result.URL, navErr)
	}

	var (
		zoomLevel float64
		haveZoom  bool
	)
	if n.browserService != nil {
		if level, err := n.browserService.GetZoomLevel(ctx, result.URL); err == nil {
			zoomLevel = level
			haveZoom = true
			if n.webView != nil {
				if n.webView.UsesDomZoom() {
					n.webView.SeedDomZoom(zoomLevel)
				} else {
					n.webView.RunOnMainThread(func() {
						if err := n.webView.SetZoom(zoomLevel); err != nil {
							log.Printf("Warning: failed to prime native zoom for %s: %v", result.URL, err)
						}
					})
				}
			}
		} else {
			log.Printf("Warning: failed to lookup zoom for %s: %v", result.URL, err)
		}
	}

	// Load URL in WebView
	if err := n.webView.LoadURL(result.URL); err != nil {
		return err
	}

	// Apply zoom for the new URL using the cached level when available.
	if n.zoomController != nil {
		if haveZoom {
			n.zoomController.ApplyZoomForURLWithLevel(result.URL, zoomLevel, false)
		} else {
			n.zoomController.ApplyZoomForURL(result.URL)
		}
	}

	return nil
}

// HandleBrowseCommand processes the browse command line argument
func (n *NavigationController) HandleBrowseCommand() {
	if len(os.Args) >= 3 && os.Args[1] == "browse" {
		log.Printf("Browse command detected: %s", os.Args[2])
		ctx := context.Background()
		result, err := n.parserService.ParseInput(ctx, os.Args[2])
		if err == nil {
			log.Printf("Parsed input → URL: %s", result.URL)
			if _, navErr := n.browserService.Navigate(ctx, result.URL); navErr != nil {
				log.Printf("Warning: failed to navigate to %s: %v", result.URL, navErr)
			}
			if n.webView != nil {
				log.Printf("Loading URL in WebView: %s", result.URL)
				var (
					zoomLevel float64
					haveZoom  bool
				)
				if n.browserService != nil {
					if level, err := n.browserService.GetZoomLevel(ctx, result.URL); err == nil {
						zoomLevel = level
						haveZoom = true
						if n.webView.UsesDomZoom() {
							n.webView.SeedDomZoom(zoomLevel)
						} else {
							n.webView.RunOnMainThread(func() {
								if err := n.webView.SetZoom(zoomLevel); err != nil {
									log.Printf("Warning: failed to prime native zoom for %s: %v", result.URL, err)
								}
							})
						}
					} else {
						log.Printf("Warning: failed to lookup zoom for %s: %v", result.URL, err)
					}
				}
				if err := n.webView.LoadURL(result.URL); err != nil {
					log.Printf("Warning: failed to load URL: %v", err)
				}
				if n.zoomController != nil {
					if haveZoom {
						n.zoomController.ApplyZoomForURLWithLevel(result.URL, zoomLevel, false)
					} else {
						n.zoomController.ApplyZoomForURL(result.URL)
					}
				}
			}
		}
	}
}
