package coordinator

import (
	"context"
	"fmt"
	"sync"

	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/infrastructure/webkit"
	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/dumber/internal/ui/cache"
	"github.com/bnema/dumber/internal/ui/component"
	"github.com/bnema/dumber/internal/ui/input"
	"github.com/bnema/dumber/internal/ui/layout"
	"github.com/jwijenbergh/puregotk/v4/gdk"
)

// ContentCoordinator manages WebView lifecycle, title tracking, and content attachment.
type ContentCoordinator struct {
	pool          *webkit.WebViewPool
	widgetFactory layout.WidgetFactory
	faviconCache  *cache.FaviconCache
	zoomUC        *usecase.ManageZoomUseCase

	webViews   map[entity.PaneID]*webkit.WebView
	paneTitles map[entity.PaneID]string
	titleMu    sync.RWMutex

	// Track original navigation URLs to handle cross-domain redirects
	// e.g., google.fr → google.com: cache favicon under both domains
	navOrigins  map[entity.PaneID]string
	navOriginMu sync.RWMutex

	// Callback to get active workspace state (avoids circular dependency)
	getActiveWS func() (*entity.Workspace, *component.WorkspaceView)

	// Callback when title changes (for history persistence)
	onTitleUpdated func(ctx context.Context, paneID entity.PaneID, url, title string)

	// Callback when page is committed (for history recording)
	onHistoryRecord func(ctx context.Context, paneID entity.PaneID, url string)

	// Gesture action handler for mouse button navigation
	gestureActionHandler input.ActionHandler
}

// NewContentCoordinator creates a new ContentCoordinator.
func NewContentCoordinator(
	ctx context.Context,
	pool *webkit.WebViewPool,
	widgetFactory layout.WidgetFactory,
	faviconCache *cache.FaviconCache,
	getActiveWS func() (*entity.Workspace, *component.WorkspaceView),
	zoomUC *usecase.ManageZoomUseCase,
) *ContentCoordinator {
	log := logging.FromContext(ctx)
	log.Debug().Msg("creating content coordinator")

	return &ContentCoordinator{
		pool:          pool,
		widgetFactory: widgetFactory,
		faviconCache:  faviconCache,
		zoomUC:        zoomUC,
		webViews:      make(map[entity.PaneID]*webkit.WebView),
		paneTitles:    make(map[entity.PaneID]string),
		navOrigins:    make(map[entity.PaneID]string),
		getActiveWS:   getActiveWS,
	}
}

// SetOnTitleUpdated sets the callback for title changes (for history persistence).
func (c *ContentCoordinator) SetOnTitleUpdated(fn func(ctx context.Context, paneID entity.PaneID, url, title string)) {
	c.onTitleUpdated = fn
}

// SetOnHistoryRecord sets the callback for recording history on page commit.
func (c *ContentCoordinator) SetOnHistoryRecord(fn func(ctx context.Context, paneID entity.PaneID, url string)) {
	c.onHistoryRecord = fn
}

// SetGestureActionHandler sets the callback for mouse button navigation gestures.
func (c *ContentCoordinator) SetGestureActionHandler(handler input.ActionHandler) {
	c.gestureActionHandler = handler
}

// EnsureWebView acquires or reuses a WebView for the given pane.
func (c *ContentCoordinator) EnsureWebView(ctx context.Context, paneID entity.PaneID) (*webkit.WebView, error) {
	log := logging.FromContext(ctx)

	if wv, ok := c.webViews[paneID]; ok && wv != nil && !wv.IsDestroyed() {
		return wv, nil
	}

	if c.pool == nil {
		return nil, fmt.Errorf("webview pool not configured")
	}

	wv, err := c.pool.Acquire(ctx)
	if err != nil {
		return nil, err
	}

	c.webViews[paneID] = wv

	// Set up title change callback
	wv.OnTitleChanged = func(title string) {
		c.onTitleChanged(ctx, paneID, title)
	}

	// Set up favicon change callback
	wv.OnFaviconChanged = func(favicon *gdk.Texture) {
		c.onFaviconChanged(ctx, paneID, favicon)
	}

	// Set up load change callback to re-apply zoom on navigation and handle progress bar
	wv.OnLoadChanged = func(event webkit.LoadEvent) {
		switch event {
		case webkit.LoadStarted:
			c.onLoadStarted(ctx, paneID)
		case webkit.LoadCommitted:
			c.onLoadCommitted(ctx, paneID, wv)
		case webkit.LoadFinished:
			c.onLoadFinished(ctx, paneID)
		}
	}

	// Set up progress callback for loading indicator
	wv.OnProgressChanged = func(progress float64) {
		c.onProgressChanged(ctx, paneID, progress)
	}

	// Set up URI change callback for SPA navigation (History API)
	// This fires when URL changes via JavaScript without a full page load
	wv.OnURIChanged = func(uri string) {
		// Only record if not loading (SPA navigation via History API)
		// Full page loads are handled by OnLoadCommitted
		if !wv.IsLoading() && uri != "" {
			c.onSPANavigation(ctx, paneID, uri)
		}
	}

	log.Debug().Str("pane_id", string(paneID)).Msg("webview acquired for pane")
	return wv, nil
}

// ReleaseWebView returns the WebView for a pane to the pool.
func (c *ContentCoordinator) ReleaseWebView(ctx context.Context, paneID entity.PaneID) {
	log := logging.FromContext(ctx)

	wv, ok := c.webViews[paneID]
	if !ok || wv == nil {
		return
	}
	delete(c.webViews, paneID)

	// Clean up title tracking
	c.titleMu.Lock()
	delete(c.paneTitles, paneID)
	c.titleMu.Unlock()

	// Clean up navigation origin tracking
	c.navOriginMu.Lock()
	delete(c.navOrigins, paneID)
	c.navOriginMu.Unlock()

	if c.pool != nil {
		c.pool.Release(ctx, wv)
	} else {
		wv.Destroy()
	}

	log.Debug().Str("pane_id", string(paneID)).Msg("webview released")
}

// AttachToWorkspace ensures each pane in the workspace has a WebView widget attached.
func (c *ContentCoordinator) AttachToWorkspace(ctx context.Context, ws *entity.Workspace, wsView *component.WorkspaceView) {
	log := logging.FromContext(ctx)

	if ws == nil || wsView == nil || c.widgetFactory == nil {
		return
	}

	for _, pane := range ws.AllPanes() {
		if pane == nil {
			continue
		}

		wv, err := c.EnsureWebView(ctx, pane.ID)
		if err != nil {
			log.Warn().Err(err).Str("pane_id", string(pane.ID)).Msg("failed to ensure webview for pane")
			continue
		}

		// Load the pane's URI if set and different from current
		if pane.URI != "" && pane.URI != wv.URI() {
			if err := wv.LoadURI(ctx, pane.URI); err != nil {
				log.Warn().Err(err).Str("pane_id", string(pane.ID)).Str("uri", pane.URI).Msg("failed to load pane URI")
			}
		}

		widget := c.WrapWidget(ctx, wv)
		if widget == nil {
			continue
		}

		if err := wsView.SetWebViewWidget(pane.ID, widget); err != nil {
			log.Warn().Err(err).Str("pane_id", string(pane.ID)).Msg("failed to attach webview widget")
		}
	}
}

// WrapWidget converts a WebView to a layout.Widget for embedding.
// It also attaches gesture handlers for mouse button navigation.
func (c *ContentCoordinator) WrapWidget(ctx context.Context, wv *webkit.WebView) layout.Widget {
	log := logging.FromContext(ctx)

	if wv == nil || c.widgetFactory == nil {
		log.Debug().Msg("cannot wrap nil webview or factory")
		return nil
	}

	gtkView := wv.Widget()
	if gtkView == nil {
		return nil
	}

	widget := c.widgetFactory.WrapWidget(&gtkView.Widget)

	// Attach gesture handler for mouse button 8/9 navigation
	if widget != nil && c.gestureActionHandler != nil {
		gestureHandler := input.NewGestureHandler(ctx)
		gestureHandler.SetOnAction(c.gestureActionHandler)
		gestureHandler.AttachTo(widget.GtkWidget())
		log.Debug().Msg("gesture handler attached to webview")
	}

	return widget
}

// ActiveWebView returns the WebView for the active pane.
func (c *ContentCoordinator) ActiveWebView(ctx context.Context) *webkit.WebView {
	log := logging.FromContext(ctx)

	ws, _ := c.getActiveWS()
	if ws == nil {
		log.Debug().Msg("no active workspace")
		return nil
	}

	pane := ws.ActivePane()
	if pane == nil || pane.Pane == nil {
		log.Debug().Msg("no active pane")
		return nil
	}

	return c.webViews[pane.Pane.ID]
}

// GetWebView returns the WebView for a specific pane.
func (c *ContentCoordinator) GetWebView(paneID entity.PaneID) *webkit.WebView {
	return c.webViews[paneID]
}

// GetTitle returns the current title for a pane.
func (c *ContentCoordinator) GetTitle(paneID entity.PaneID) string {
	c.titleMu.RLock()
	defer c.titleMu.RUnlock()
	return c.paneTitles[paneID]
}

// onTitleChanged updates title tracking when a WebView's title changes.
func (c *ContentCoordinator) onTitleChanged(ctx context.Context, paneID entity.PaneID, title string) {
	log := logging.FromContext(ctx)

	// Update title map
	c.titleMu.Lock()
	c.paneTitles[paneID] = title
	c.titleMu.Unlock()

	// Update domain model
	ws, wsView := c.getActiveWS()
	if ws != nil {
		paneNode := ws.FindPane(paneID)
		if paneNode != nil && paneNode.Pane != nil {
			paneNode.Pane.Title = title
		}
	}

	// Update StackedView title bar if this pane is in a stack
	if wsView != nil {
		tr := wsView.TreeRenderer()
		if tr != nil {
			stackedView := tr.GetStackedViewForPane(string(paneID))
			if stackedView != nil {
				c.updateStackedPaneTitle(ctx, ws, stackedView, paneID, title)
			}
		}
	}

	// Notify history persistence (get URL from WebView)
	if c.onTitleUpdated != nil {
		if wv := c.webViews[paneID]; wv != nil {
			url := wv.URI()
			if url != "" && title != "" {
				c.onTitleUpdated(ctx, paneID, url, title)
			}
		}
	}

	log.Debug().
		Str("pane_id", string(paneID)).
		Str("title", title).
		Msg("pane title updated")
}

// updateStackedPaneTitle updates the title of a pane in a StackedView.
func (c *ContentCoordinator) updateStackedPaneTitle(ctx context.Context, ws *entity.Workspace, sv *layout.StackedView, paneID entity.PaneID, title string) {
	log := logging.FromContext(ctx)

	if ws == nil {
		return
	}

	paneNode := ws.FindPane(paneID)
	if paneNode == nil {
		return
	}

	// If the pane is in a stacked parent, find its index
	if paneNode.Parent != nil && paneNode.Parent.IsStacked {
		for i, child := range paneNode.Parent.Children {
			if child.Pane != nil && child.Pane.ID == paneID {
				if err := sv.UpdateTitle(i, title); err != nil {
					log.Warn().Err(err).Int("index", i).Msg("failed to update stacked pane title")
				}
				return
			}
		}
	}
}

// onFaviconChanged updates favicon tracking when a WebView's favicon changes.
func (c *ContentCoordinator) onFaviconChanged(ctx context.Context, paneID entity.PaneID, favicon *gdk.Texture) {
	log := logging.FromContext(ctx)

	// Get current URI to extract domain for caching
	wv := c.webViews[paneID]
	if wv == nil {
		return
	}
	uri := wv.URI()

	// Update favicon cache with domain key (final URL after redirects)
	if c.faviconCache != nil && favicon != nil && uri != "" {
		c.faviconCache.SetByURL(uri, favicon)

		// Also cache under original navigation URL to handle cross-domain redirects
		// e.g., google.fr → google.com: cache favicon under both domains
		c.navOriginMu.RLock()
		originURL := c.navOrigins[paneID]
		c.navOriginMu.RUnlock()
		if originURL != "" && originURL != uri {
			c.faviconCache.SetByURL(originURL, favicon)
		}
	}

	// Update StackedView favicon if this pane is in a stack
	ws, wsView := c.getActiveWS()
	if wsView != nil {
		tr := wsView.TreeRenderer()
		if tr != nil {
			stackedView := tr.GetStackedViewForPane(string(paneID))
			if stackedView != nil {
				c.updateStackedPaneFavicon(ctx, ws, stackedView, paneID, favicon)
			}
		}
	}

	log.Debug().
		Str("pane_id", string(paneID)).
		Str("uri", uri).
		Bool("has_favicon", favicon != nil).
		Msg("pane favicon updated")
}

// updateStackedPaneFavicon updates the favicon of a pane in a StackedView.
func (c *ContentCoordinator) updateStackedPaneFavicon(ctx context.Context, ws *entity.Workspace, sv *layout.StackedView, paneID entity.PaneID, favicon *gdk.Texture) {
	log := logging.FromContext(ctx)

	if ws == nil {
		return
	}

	paneNode := ws.FindPane(paneID)
	if paneNode == nil {
		return
	}

	// If the pane is in a stacked parent, find its index
	if paneNode.Parent != nil && paneNode.Parent.IsStacked {
		for i, child := range paneNode.Parent.Children {
			if child.Pane != nil && child.Pane.ID == paneID {
				if err := sv.UpdateFaviconTexture(i, favicon); err != nil {
					log.Warn().Err(err).Int("index", i).Msg("failed to update stacked pane favicon")
				}
				return
			}
		}
	}
}

// FaviconCache returns the favicon cache for external use (e.g., omnibox).
func (c *ContentCoordinator) FaviconCache() *cache.FaviconCache {
	return c.faviconCache
}

// SetNavigationOrigin records the original URL before navigation starts.
// This allows caching favicons under both original and final domains
// when cross-domain redirects occur (e.g., google.fr → google.com).
func (c *ContentCoordinator) SetNavigationOrigin(paneID entity.PaneID, url string) {
	c.navOriginMu.Lock()
	c.navOrigins[paneID] = url
	c.navOriginMu.Unlock()
}

// PreloadCachedFavicon checks the favicon cache and updates the stacked pane
// title bar immediately if a cached favicon exists for the URL.
// This provides instant favicon display without waiting for WebKit.
func (c *ContentCoordinator) PreloadCachedFavicon(ctx context.Context, paneID entity.PaneID, url string) {
	if c.faviconCache == nil || url == "" {
		return
	}

	// Check memory and disk cache (no external fetch)
	texture := c.faviconCache.GetFromCacheByURL(url)
	if texture == nil {
		return
	}

	// Update stacked pane favicon if applicable
	ws, wsView := c.getActiveWS()
	if wsView != nil {
		tr := wsView.TreeRenderer()
		if tr != nil {
			stackedView := tr.GetStackedViewForPane(string(paneID))
			if stackedView != nil {
				c.updateStackedPaneFavicon(ctx, ws, stackedView, paneID, texture)
			}
		}
	}
}

// onLoadCommitted re-applies zoom when page content starts loading and records history.
// WebKit may reset zoom during document transitions, so we reapply after LoadCommitted.
// History is recorded here because the URI is guaranteed to be correct after commit.
func (c *ContentCoordinator) onLoadCommitted(ctx context.Context, paneID entity.PaneID, wv *webkit.WebView) {
	url := wv.URI()
	if url == "" {
		return
	}

	// Record history - URI is guaranteed to be correct at LoadCommitted
	if c.onHistoryRecord != nil {
		c.onHistoryRecord(ctx, paneID, url)
	}

	// Apply zoom
	if c.zoomUC == nil {
		return
	}

	domain, err := usecase.ExtractDomain(url)
	if err != nil {
		return
	}

	_ = c.zoomUC.ApplyToWebView(ctx, wv, domain)
}

// onSPANavigation records history when URL changes via JavaScript (History API).
// This handles SPA navigation like YouTube search, where the URL changes without a page load.
func (c *ContentCoordinator) onSPANavigation(ctx context.Context, paneID entity.PaneID, url string) {
	log := logging.FromContext(ctx)
	log.Debug().Str("pane_id", string(paneID)).Str("url", url).Msg("SPA navigation detected")

	// Record history for SPA navigation
	if c.onHistoryRecord != nil {
		c.onHistoryRecord(ctx, paneID, url)
	}
}

// onLoadStarted shows the progress bar when page loading begins.
func (c *ContentCoordinator) onLoadStarted(ctx context.Context, paneID entity.PaneID) {
	_, wsView := c.getActiveWS()
	if wsView == nil {
		return
	}

	paneView := wsView.GetPaneView(paneID)
	if paneView != nil {
		paneView.SetLoading(true)
	}
}

// onLoadFinished hides the progress bar when page loading completes.
func (c *ContentCoordinator) onLoadFinished(ctx context.Context, paneID entity.PaneID) {
	_, wsView := c.getActiveWS()
	if wsView == nil {
		return
	}

	paneView := wsView.GetPaneView(paneID)
	if paneView != nil {
		paneView.SetLoading(false)
	}
}

// onProgressChanged updates the progress bar with current load progress.
func (c *ContentCoordinator) onProgressChanged(ctx context.Context, paneID entity.PaneID, progress float64) {
	_, wsView := c.getActiveWS()
	if wsView == nil {
		return
	}

	paneView := wsView.GetPaneView(paneID)
	if paneView != nil {
		paneView.SetLoadProgress(progress)
	}
}
