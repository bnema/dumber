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
}

// Navigate loads a URL in the active pane using NavigateUseCase.
// This properly handles history recording and zoom application.
func (c *NavigationCoordinator) Navigate(ctx context.Context, url string) error {
	log := logging.FromContext(ctx)

	wv := c.contentCoord.ActiveWebView(ctx)
	if wv == nil {
		log.Warn().Str("url", url).Msg("no active webview for navigation")
		return fmt.Errorf("no active webview")
	}

	// Get active pane ID for tracking
	ws, _ := c.contentCoord.getActiveWS()
	var paneID string
	if ws != nil {
		if pane := ws.ActivePane(); pane != nil && pane.Pane != nil {
			paneID = string(pane.Pane.ID)
			// Track original URL for cross-domain redirect favicon caching
			c.contentCoord.SetNavigationOrigin(pane.Pane.ID, url)
			// Pre-load cached favicon for instant display in stacked pane title bar
			c.contentCoord.PreloadCachedFavicon(ctx, pane.Pane.ID, url)
		}
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
func (c *NavigationCoordinator) GoBack(ctx context.Context) error {
	log := logging.FromContext(ctx)

	wv := c.contentCoord.ActiveWebView(ctx)
	if wv == nil {
		log.Debug().Msg("no active webview for go back")
		return nil
	}

	if c.navigateUC != nil {
		return c.navigateUC.GoBack(ctx, wv)
	}

	return wv.GoBack(ctx)
}

// GoForward navigates forward in history.
func (c *NavigationCoordinator) GoForward(ctx context.Context) error {
	log := logging.FromContext(ctx)

	wv := c.contentCoord.ActiveWebView(ctx)
	if wv == nil {
		log.Debug().Msg("no active webview for go forward")
		return nil
	}

	if c.navigateUC != nil {
		return c.navigateUC.GoForward(ctx, wv)
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
