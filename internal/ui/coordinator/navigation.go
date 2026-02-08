package coordinator

import (
	"context"
	"fmt"

	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/infrastructure/webkit"
	"github.com/bnema/dumber/internal/logging"
)

// OmniboxProvider provides access to omnibox operations.
type OmniboxProvider interface {
	ToggleOmnibox(ctx context.Context)
	UpdateOmniboxZoom(factor float64)
	SetOmniboxOnNavigate(fn func(url string))
}

// NavigationCoordinator handles URL navigation, history, and browser controls.
type NavigationCoordinator struct {
	navigateUC      *usecase.NavigateUseCase
	contentCoord    *ContentCoordinator
	omniboxProvider OmniboxProvider
}

// NewNavigationCoordinator creates a new NavigationCoordinator.
func NewNavigationCoordinator(
	ctx context.Context,
	navigateUC *usecase.NavigateUseCase,
	contentCoord *ContentCoordinator,
) *NavigationCoordinator {
	log := logging.FromContext(ctx)
	log.Debug().Msg("creating navigation coordinator")

	return &NavigationCoordinator{
		navigateUC:   navigateUC,
		contentCoord: contentCoord,
	}
}

// SetOmniboxProvider sets the omnibox provider for toggle/zoom operations.
func (c *NavigationCoordinator) SetOmniboxProvider(provider OmniboxProvider) {
	c.omniboxProvider = provider
	if c.omniboxProvider != nil {
		c.omniboxProvider.SetOmniboxOnNavigate(func(url string) {
			ctx := context.Background()
			if err := c.Navigate(ctx, url); err != nil {
				logging.FromContext(ctx).Warn().Err(err).Str("url", url).Msg("omnibox-initiated navigation failed")
			}
		})
	}
}

// Navigate loads a URL in the active pane using NavigateUseCase.
// This properly handles history recording and zoom application.
func (c *NavigationCoordinator) Navigate(ctx context.Context, url string) error {
	log := logging.FromContext(ctx)
	if c.contentCoord == nil {
		log.Warn().Str("url", url).Msg("content coordinator not initialized")
		return fmt.Errorf("content coordinator not initialized")
	}

	wv := c.contentCoord.ActiveWebView(ctx)
	if wv == nil {
		log.Warn().Str("url", url).Msg("no active webview for navigation")
		return fmt.Errorf("no active webview")
	}

	// Get active pane ID for tracking
	activePaneID := c.contentCoord.ActivePaneID(ctx)
	paneID := string(activePaneID)
	if activePaneID != "" {
		// Track original URL for cross-domain redirect favicon caching
		c.contentCoord.SetNavigationOrigin(activePaneID, url)
		// Pre-load cached favicon asynchronously (don't block navigation start)
		go c.contentCoord.PreloadCachedFavicon(ctx, activePaneID, url)
	}

	// Use NavigateUseCase which handles history + zoom
	if c.navigateUC != nil {
		input := usecase.NavigateInput{
			URL:     url,
			PaneID:  paneID,
			WebView: wv, // webkit.WebView implements port.WebView
		}
		output, err := c.navigateUC.Execute(ctx, input)
		if err != nil {
			log.Error().Err(err).Str("url", url).Msg("navigation failed")
			return err
		}
		log.Debug().
			Str("url", url).
			Float64("zoom", output.AppliedZoom).
			Msg("navigation initiated via usecase")
		return nil
	}

	// Fallback: direct navigation without usecase
	log.Warn().Msg("navigateUC not available, using direct LoadURI")
	if err := wv.LoadURI(ctx, url); err != nil {
		log.Error().Err(err).Str("url", url).Msg("failed to navigate")
		return err
	}

	log.Debug().Str("url", url).Msg("navigated active pane (direct)")
	return nil
}

// Reload reloads the current page.
func (c *NavigationCoordinator) Reload(ctx context.Context) error {
	log := logging.FromContext(ctx)

	wv := c.contentCoord.ActiveWebView(ctx)
	if wv == nil {
		log.Debug().Msg("no active webview for reload")
		return nil
	}

	if c.navigateUC != nil {
		return c.navigateUC.Reload(ctx, wv, false)
	}

	return wv.Reload(ctx)
}

// HardReload reloads the current page bypassing cache.
func (c *NavigationCoordinator) HardReload(ctx context.Context) error {
	log := logging.FromContext(ctx)

	wv := c.contentCoord.ActiveWebView(ctx)
	if wv == nil {
		log.Debug().Msg("no active webview for hard reload")
		return nil
	}

	if c.navigateUC != nil {
		return c.navigateUC.Reload(ctx, wv, true)
	}

	return wv.ReloadBypassCache(ctx)
}

// GoBack navigates back in history.
// Calls webview directly - the webview layer handles SPA navigation via JS fallback.
func (c *NavigationCoordinator) GoBack(ctx context.Context) error {
	log := logging.FromContext(ctx)

	wv := c.contentCoord.ActiveWebView(ctx)
	if wv == nil {
		log.Debug().Msg("no active webview for go back")
		return nil
	}

	return wv.GoBack(ctx)
}

// GoForward navigates forward in history.
// Calls webview directly - the webview layer handles SPA navigation via JS fallback.
func (c *NavigationCoordinator) GoForward(ctx context.Context) error {
	log := logging.FromContext(ctx)

	wv := c.contentCoord.ActiveWebView(ctx)
	if wv == nil {
		log.Debug().Msg("no active webview for go forward")
		return nil
	}

	return wv.GoForward(ctx)
}

// OpenOmnibox toggles the omnibox visibility.
func (c *NavigationCoordinator) OpenOmnibox(ctx context.Context) error {
	log := logging.FromContext(ctx)

	if c.omniboxProvider == nil {
		log.Error().Msg("omnibox provider not initialized")
		return fmt.Errorf("omnibox provider not initialized")
	}

	log.Debug().Msg("toggling omnibox")
	c.omniboxProvider.ToggleOmnibox(ctx)
	return nil
}

// OpenDevTools opens the WebKit inspector for the active WebView.
func (c *NavigationCoordinator) OpenDevTools(ctx context.Context) error {
	log := logging.FromContext(ctx)

	wv := c.contentCoord.ActiveWebView(ctx)
	if wv == nil {
		log.Warn().Msg("no active webview for devtools")
		return fmt.Errorf("no active webview")
	}

	log.Debug().Uint64("webview_id", uint64(wv.ID())).Msg("opening devtools")

	// Type assert to access ShowDevTools (not in port.WebView interface)
	if webkitWV, ok := interface{}(wv).(*webkit.WebView); ok {
		return webkitWV.ShowDevTools()
	}

	return fmt.Errorf("webview does not support devtools")
}

// PrintPage opens the print dialog for the active WebView.
func (c *NavigationCoordinator) PrintPage(ctx context.Context) error {
	log := logging.FromContext(ctx)

	wv := c.contentCoord.ActiveWebView(ctx)
	if wv == nil {
		log.Warn().Msg("no active webview for print")
		return fmt.Errorf("no active webview")
	}

	log.Debug().Uint64("webview_id", uint64(wv.ID())).Msg("opening print dialog")

	// Type assert to access Print (not in port.WebView interface)
	if webkitWV, ok := interface{}(wv).(*webkit.WebView); ok {
		return webkitWV.Print()
	}

	return fmt.Errorf("webview does not support printing")
}

// UpdateHistoryTitle updates the title of a history entry after page load.
func (c *NavigationCoordinator) UpdateHistoryTitle(ctx context.Context, paneID entity.PaneID, url, title string) {
	log := logging.FromContext(ctx)

	if c.navigateUC == nil {
		return
	}

	if err := c.navigateUC.UpdateHistoryTitle(ctx, url, title); err != nil {
		log.Warn().Err(err).Str("url", url).Msg("failed to update history title")
	}
}

// RecordHistory records a URL in history on page commit.
func (c *NavigationCoordinator) RecordHistory(ctx context.Context, paneID entity.PaneID, url string) {
	if c.navigateUC == nil {
		return
	}

	c.navigateUC.RecordHistory(ctx, string(paneID), url)
}

// ClearPaneHistory clears per-pane navigation history deduplication state.
func (c *NavigationCoordinator) ClearPaneHistory(paneID entity.PaneID) {
	if c.navigateUC == nil {
		return
	}
	c.navigateUC.ClearPaneHistory(string(paneID))
}

// ActiveWebView returns the WebView for the active pane (for zoom operations).
func (c *NavigationCoordinator) ActiveWebView(ctx context.Context) *webkit.WebView {
	return c.contentCoord.ActiveWebView(ctx)
}

// NotifyZoomChanged updates the omnibox zoom indicator.
func (c *NavigationCoordinator) NotifyZoomChanged(ctx context.Context, factor float64) {
	if c.omniboxProvider != nil {
		c.omniboxProvider.UpdateOmniboxZoom(factor)
	}
}
