package content

import (
	"context"
	"strings"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/dumber/internal/ui/adapter"
	"github.com/bnema/dumber/internal/ui/layout"
	"github.com/bnema/puregotk/v4/gdk"
	"github.com/bnema/puregotk/v4/glib"
)

// GetTitle returns the current title for a pane.
func (c *Coordinator) GetTitle(paneID entity.PaneID) string {
	c.titleMu.RLock()
	defer c.titleMu.RUnlock()
	return c.paneTitles[paneID]
}

// onTitleChanged updates title tracking when a WebView's title changes.
func (c *Coordinator) onTitleChanged(ctx context.Context, paneID entity.PaneID, title string) {
	log := logging.FromContext(ctx)

	// Update title map
	c.titleMu.Lock()
	c.paneTitles[paneID] = title
	c.titleMu.Unlock()

	// Update domain model and check if this is the active pane
	isActivePaneTitle := false
	if c.getActiveWS == nil {
		return
	}
	ws, wsView := c.getActiveWS()
	if ws != nil {
		paneNode := ws.FindPane(paneID)
		if paneNode != nil && paneNode.Pane != nil {
			paneNode.Pane.Title = title
		}
		// Check if this pane is the active one
		if ws.ActivePaneID == paneID {
			isActivePaneTitle = true
		}
	}

	// Update StackedView title bar if this pane is in a stack
	if wsView != nil {
		tr := wsView.TreeRenderer()
		if tr != nil {
			stackedView := tr.GetStackedViewForPane(string(paneID))
			if stackedView != nil {
				c.updateStackedPaneTitle(ctx, stackedView, paneID, title)
			}
		}
	}

	// Notify history persistence (get URL from WebView)
	if c.onTitleUpdated != nil {
		if wv := c.getWebViewLocked(paneID); wv != nil {
			currentURI := wv.URI()
			if currentURI != "" && title != "" {
				c.onTitleUpdated(ctx, paneID, currentURI, title)
			}
		}
	}

	// Notify window title update if this is the active pane
	if isActivePaneTitle && c.onWindowTitleChanged != nil {
		c.onWindowTitleChanged(title)
	}

	log.Debug().
		Str("pane_id", string(paneID)).
		Str("title", title).
		Msg("pane title updated")
}

// updateStackedPaneTitle updates the title of a pane in a StackedView.
func (c *Coordinator) updateStackedPaneTitle(
	ctx context.Context,
	sv *layout.StackedView,
	paneID entity.PaneID,
	title string,
) {
	log := logging.FromContext(ctx)

	// Find the pane's index directly in the StackedView
	index := sv.FindPaneIndex(string(paneID))
	if index < 0 {
		log.Debug().
			Str("pane_id", string(paneID)).
			Msg("pane not found in StackedView for title update")
		return
	}

	if err := sv.UpdateTitle(index, title); err != nil {
		log.Warn().Err(err).Int("index", index).Msg("failed to update stacked pane title")
	}
}

// syncStackedTitle updates the stacked title bar for a pane if it's in a stack.
// Called from onLoadCommitted to keep titles in sync during navigation.
func (c *Coordinator) syncStackedTitle(ctx context.Context, paneID entity.PaneID, title string) {
	if c.getActiveWS == nil {
		return
	}
	_, wsView := c.getActiveWS()
	if wsView == nil {
		return
	}
	tr := wsView.TreeRenderer()
	if tr == nil {
		return
	}
	if sv := tr.GetStackedViewForPane(string(paneID)); sv != nil {
		c.updateStackedPaneTitle(ctx, sv, paneID, title)
	}
}

// onFaviconChanged updates favicon tracking when a WebView's favicon changes.
func (c *Coordinator) onFaviconChanged(ctx context.Context, paneID entity.PaneID, emittingWV port.WebView, favicon *gdk.Texture) {
	log := logging.FromContext(ctx)

	// Verify this WebView is still bound to the expected pane
	currentWV := c.getWebViewLocked(paneID)
	if currentWV == nil || currentWV != emittingWV {
		log.Debug().
			Str("pane_id", string(paneID)).
			Msg("ignoring favicon change from unbound webview")
		return
	}

	uri := emittingWV.URI()

	// Update favicon cache with domain key (handles cross-domain redirects)
	if c.faviconAdapter != nil && favicon != nil && uri != "" {
		c.navOriginMu.RLock()
		originURL := c.navOrigins[paneID]
		c.navOriginMu.RUnlock()
		c.faviconAdapter.StoreFromWebKitWithOrigin(ctx, uri, originURL, favicon)
	}

	// Update StackedView favicon if this pane is in a stack
	c.updateStackedFaviconForPane(ctx, paneID, favicon)

	log.Debug().
		Str("pane_id", string(paneID)).
		Str("uri", uri).
		Bool("has_favicon", favicon != nil).
		Msg("pane favicon updated")
}

// updateStackedPaneFavicon updates the favicon of a pane in a StackedView.
func (c *Coordinator) updateStackedPaneFavicon(
	ctx context.Context,
	sv *layout.StackedView,
	paneID entity.PaneID,
	favicon *gdk.Texture,
) {
	log := logging.FromContext(ctx)

	// Find the pane's index directly in the StackedView
	index := sv.FindPaneIndex(string(paneID))
	if index < 0 {
		log.Debug().
			Str("pane_id", string(paneID)).
			Msg("pane not found in StackedView for favicon update")
		return
	}

	if err := sv.UpdateFaviconTexture(index, favicon); err != nil {
		log.Warn().Err(err).Int("index", index).Msg("failed to update stacked pane favicon")
	}
}

// updateStackedFaviconForPane updates the stacked title bar favicon for a pane.
func (c *Coordinator) updateStackedFaviconForPane(ctx context.Context, paneID entity.PaneID, texture *gdk.Texture) {
	if c.getActiveWS == nil {
		return
	}
	_, wsView := c.getActiveWS()
	if wsView == nil {
		return
	}
	tr := wsView.TreeRenderer()
	if tr == nil {
		return
	}
	if sv := tr.GetStackedViewForPane(string(paneID)); sv != nil {
		c.updateStackedPaneFavicon(ctx, sv, paneID, texture)
	}
}

// resolveCommittedFavicon ensures the stacked title bar has a favicon after navigation commits.
// First checks if WebKit already has a favicon for the page (common for subpath URLs).
// Falls back to the full GetOrFetch pipeline (cache + WebKit DB + DuckDuckGo API).
// This closes the gap where title bars relied solely on the notify::favicon signal.
func (c *Coordinator) resolveCommittedFavicon(ctx context.Context, paneID entity.PaneID, wv port.WebView) {
	if c.faviconAdapter == nil || wv == nil {
		return
	}

	uri := wv.URI()
	if uri == "" || strings.HasPrefix(uri, "about:") || strings.HasPrefix(uri, "dumb://") {
		return
	}

	log := logging.FromContext(ctx)

	// Check if we already have a texture cached for this domain
	if texture := c.faviconAdapter.GetTextureByURL(uri); texture != nil {
		// Already cached - just ensure the stacked view is updated
		c.updateStackedFaviconForPane(ctx, paneID, texture)
		return
	}

	// Check if the WebView already has a favicon for this page
	if icon := wv.Favicon(); icon != nil {
		if gdkIcon, ok := icon.(*gdk.Texture); ok {
			log.Debug().Str("pane_id", string(paneID)).Str("uri", uri).Msg("using existing favicon for committed page")
			// Store and update through the normal path
			c.navOriginMu.RLock()
			originURL := c.navOrigins[paneID]
			c.navOriginMu.RUnlock()
			c.faviconAdapter.StoreFromWebKitWithOrigin(ctx, uri, originURL, gdkIcon)
			c.updateStackedFaviconForPane(ctx, paneID, gdkIcon)
			return
		}
	}

	// Fall back to GetOrFetch (checks service cache, engine DB, then DuckDuckGo API)
	// Capture current generation to guard against stale callbacks
	gen := wv.Generation()
	capturedURI := uri
	c.faviconAdapter.GetOrFetch(ctx, uri, func(texture *gdk.Texture) {
		// Skip nil results — a nil means "couldn't resolve", not "no favicon".
		// Without this guard, a late nil callback can overwrite a good favicon
		// that was already set by an earlier onFaviconChanged signal.
		if texture == nil {
			return
		}
		// Verify WebView is still bound to pane and hasn't been reused
		if wv.Generation() != gen {
			return
		}
		currentWV := c.getWebViewLocked(paneID)
		if currentWV != wv {
			return
		}
		// Verify the WebView hasn't navigated away since we started the fetch
		if currentWV.URI() != capturedURI {
			return
		}
		c.updateStackedFaviconForPane(ctx, paneID, texture)
	})
}

// FaviconAdapter returns the favicon adapter for external use (e.g., omnibox).
func (c *Coordinator) FaviconAdapter() *adapter.FaviconAdapter {
	return c.faviconAdapter
}

// SetNavigationOrigin records the original URL before navigation starts.
// This allows caching favicons under both original and final domains
// when cross-domain redirects occur (e.g., google.fr → google.com).
func (c *Coordinator) SetNavigationOrigin(paneID entity.PaneID, uri string) {
	c.navOriginMu.Lock()
	c.navOrigins[paneID] = uri
	c.navOriginMu.Unlock()
}

// PreloadCachedFavicon checks the favicon cache and updates the stacked pane
// title bar immediately if a cached favicon exists for the URL.
// This provides instant favicon display without waiting for WebKit.
func (c *Coordinator) PreloadCachedFavicon(ctx context.Context, paneID entity.PaneID, uri string) {
	if c.faviconAdapter == nil || uri == "" {
		return
	}
	select {
	case <-ctx.Done():
		return
	default:
	}

	// Check memory and disk cache (no external fetch)
	texture := c.faviconAdapter.PreloadFromCache(uri)
	select {
	case <-ctx.Done():
		return
	default:
	}

	// Update stacked pane favicon on GTK main loop.
	cb := glib.SourceFunc(func(_ uintptr) bool {
		select {
		case <-ctx.Done():
			return false
		default:
		}
		if c.getActiveWS == nil {
			return false
		}
		// Only update if we found a cached texture.
		// If nil, let resolveCommittedFavicon handle it after load commits.
		if texture == nil {
			return false
		}
		_, wsView := c.getActiveWS()
		if wsView == nil {
			return false
		}
		tr := wsView.TreeRenderer()
		if tr == nil {
			return false
		}
		stackedView := tr.GetStackedViewForPane(string(paneID))
		if stackedView == nil {
			return false
		}
		c.updateStackedPaneFavicon(ctx, stackedView, paneID, texture)
		return false
	})
	glib.IdleAdd(&cb, 0)
}
