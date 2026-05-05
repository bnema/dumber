package coordinator

import (
	"context"
	"fmt"
	"time"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/dumber/internal/ui/coordinator/content"
)

// OmniboxProvider provides access to omnibox operations.
type OmniboxProvider interface {
	ToggleOmnibox(ctx context.Context)
	UpdateOmniboxZoom(factor float64)
}

// NavigationCoordinator handles URL navigation, history, and browser controls.
type NavigationCoordinator struct {
	contextProvider func() context.Context
	navigateUC      *usecase.NavigateUseCase
	contentCoord    *content.Coordinator
	omniboxProvider OmniboxProvider
}

const faviconPreloadTimeout = 300 * time.Millisecond

// NewNavigationCoordinator creates a new NavigationCoordinator.
func NewNavigationCoordinator(
	ctx context.Context,
	navigateUC *usecase.NavigateUseCase,
	contentCoord *content.Coordinator,
) *NavigationCoordinator {
	log := logging.FromContext(ctx)
	log.Debug().Msg("creating navigation coordinator")
	callbackLogger := *log

	return &NavigationCoordinator{
		contextProvider: func() context.Context {
			return logging.WithContext(context.Background(), callbackLogger)
		},
		navigateUC:   navigateUC,
		contentCoord: contentCoord,
	}
}

// SetOmniboxProvider sets the omnibox provider for toggle/zoom operations.
func (c *NavigationCoordinator) SetOmniboxProvider(provider OmniboxProvider) {
	c.omniboxProvider = provider
}

// requireWebView returns an error if wv is nil, preserving stable error text.
func requireWebView(wv port.WebView) error {
	if wv == nil {
		return fmt.Errorf("no webview provided")
	}
	return nil
}

func (c *NavigationCoordinator) trackNavigationOrigin(ctx context.Context, paneID entity.PaneID, url string) {
	if c.contentCoord == nil || paneID == "" {
		return
	}
	c.contentCoord.SetNavigationOrigin(paneID, url)
	go func() {
		preloadCtx, cancelPreload := context.WithTimeout(ctx, faviconPreloadTimeout)
		defer cancelPreload()
		c.contentCoord.PreloadCachedFavicon(preloadCtx, paneID, url)
	}()
}

// NavigateWebView loads a URL in the specified pane using the provided WebView.
// This is the explicit-target equivalent of Navigate: it uses NavigateUseCase if
// available, falling back to direct LoadURI. Origin tracking via contentCoord is
// performed when contentCoord and paneID are available.
func (c *NavigationCoordinator) NavigateWebView(ctx context.Context, url string, paneID entity.PaneID, wv port.WebView) error {
	log := logging.FromContext(ctx)

	if err := requireWebView(wv); err != nil {
		log.Warn().Str("url", url).Msg("NavigateWebView called with nil webview")
		return err
	}

	c.trackNavigationOrigin(ctx, paneID, url)

	paneIDStr := string(paneID)

	if c.navigateUC != nil {
		input := usecase.NavigateInput{
			URL:     url,
			PaneID:  paneIDStr,
			WebView: wv,
		}
		output, err := c.navigateUC.Execute(ctx, input)
		if err != nil {
			log.Error().Err(err).Str("url", url).Str("pane_id", paneIDStr).Msg("navigation failed")
			return err
		}
		log.Debug().
			Str("url", url).
			Str("pane_id", paneIDStr).
			Float64("zoom", output.AppliedZoom).
			Uint64("webview_id", uint64(wv.ID())).
			Msg("navigation initiated via usecase")
		return nil
	}

	log.Warn().Msg("navigateUC not available, using direct LoadURI")
	if err := wv.LoadURI(ctx, url); err != nil {
		log.Error().Err(err).Str("url", url).Msg("failed to navigate")
		return err
	}

	log.Debug().Str("url", url).Uint64("webview_id", uint64(wv.ID())).Msg("navigated explicit webview (direct)")
	return nil
}

// ReloadWebView reloads the page in the provided WebView, optionally bypassing cache.
func (c *NavigationCoordinator) ReloadWebView(ctx context.Context, wv port.WebView, bypassCache bool) error {
	log := logging.FromContext(ctx)

	if err := requireWebView(wv); err != nil {
		log.Debug().Msg("ReloadWebView called with nil webview")
		return err
	}

	if c.navigateUC != nil {
		return c.navigateUC.Reload(ctx, wv, bypassCache)
	}

	if bypassCache {
		log.Debug().Uint64("webview_id", uint64(wv.ID())).Msg("reloading explicit webview bypassing cache")
		return wv.ReloadBypassCache(ctx)
	}

	log.Debug().Uint64("webview_id", uint64(wv.ID())).Msg("reloading explicit webview")
	return wv.Reload(ctx)
}

// StopWebView stops loading in the provided WebView.
func (c *NavigationCoordinator) StopWebView(ctx context.Context, wv port.WebView) error {
	log := logging.FromContext(ctx)

	if err := requireWebView(wv); err != nil {
		log.Debug().Msg("StopWebView called with nil webview")
		return err
	}

	if c.navigateUC != nil {
		return c.navigateUC.Stop(ctx, wv)
	}

	log.Debug().Uint64("webview_id", uint64(wv.ID())).Msg("stopping explicit webview")
	return wv.Stop(ctx)
}

// GoBackWebView navigates back in history for the provided WebView.
// Calls webview directly - the webview layer handles SPA navigation via JS fallback.
func (c *NavigationCoordinator) GoBackWebView(ctx context.Context, wv port.WebView) error {
	log := logging.FromContext(ctx)

	if err := requireWebView(wv); err != nil {
		log.Debug().Msg("GoBackWebView called with nil webview")
		return err
	}

	log.Debug().Uint64("webview_id", uint64(wv.ID())).Msg("going back in explicit webview")
	return wv.GoBack(ctx)
}

// GoForwardWebView navigates forward in history for the provided WebView.
// Calls webview directly - the webview layer handles SPA navigation via JS fallback.
func (c *NavigationCoordinator) GoForwardWebView(ctx context.Context, wv port.WebView) error {
	log := logging.FromContext(ctx)

	if err := requireWebView(wv); err != nil {
		log.Debug().Msg("GoForwardWebView called with nil webview")
		return err
	}

	log.Debug().Uint64("webview_id", uint64(wv.ID())).Msg("going forward in explicit webview")
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

// OpenDevToolsWebView opens the WebKit inspector for the provided WebView.
func (c *NavigationCoordinator) OpenDevToolsWebView(ctx context.Context, wv port.WebView) error {
	log := logging.FromContext(ctx)

	if err := requireWebView(wv); err != nil {
		log.Warn().Msg("OpenDevToolsWebView called with nil webview")
		return err
	}

	log.Debug().Uint64("webview_id", uint64(wv.ID())).Msg("opening devtools")

	if opener, ok := wv.(port.DevToolsOpener); ok {
		opener.OpenDevTools()
		return nil
	}

	return fmt.Errorf("webview does not support devtools")
}

// PrintWebView opens the print dialog for the provided WebView.
func (c *NavigationCoordinator) PrintWebView(ctx context.Context, wv port.WebView) error {
	log := logging.FromContext(ctx)

	if err := requireWebView(wv); err != nil {
		log.Warn().Msg("PrintWebView called with nil webview")
		return err
	}

	log.Debug().Uint64("webview_id", uint64(wv.ID())).Msg("opening print dialog")

	if printer, ok := wv.(port.Printer); ok {
		printer.PrintPage()
		return nil
	}

	return fmt.Errorf("webview does not support printing")
}

// UpdateHistoryTitle updates the title of a history entry after page load.
func (c *NavigationCoordinator) UpdateHistoryTitle(ctx context.Context, paneID entity.PaneID, url, title string) {
	if c.navigateUC == nil {
		return
	}

	c.navigateUC.UpdateHistoryTitle(ctx, url, title)
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

// NotifyZoomChanged updates the omnibox zoom indicator.
func (c *NavigationCoordinator) NotifyZoomChanged(ctx context.Context, factor float64) {
	if c.omniboxProvider != nil {
		c.omniboxProvider.UpdateOmniboxZoom(factor)
	}
}
