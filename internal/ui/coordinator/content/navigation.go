package content

import (
	"context"
	"strings"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/domain/entity"
	urlutil "github.com/bnema/dumber/internal/domain/url"
	"github.com/bnema/dumber/internal/infrastructure/desktop"
	"github.com/bnema/dumber/internal/infrastructure/webkit"
	"github.com/bnema/dumber/internal/logging"
)

// onLoadCommitted re-applies zoom when page content starts loading and records history.
// WebKit may reset zoom during document transitions, so we reapply after LoadCommitted.
// History is recorded here because the URI is guaranteed to be correct after commit.
// Also shows the WebView widget (it's hidden during creation to avoid white flash).
func (c *Coordinator) onLoadCommitted(ctx context.Context, paneID entity.PaneID, wv port.WebView) {
	log := logging.FromContext(ctx)
	logging.Trace().Mark("load_committed")

	uri := wv.URI()
	if uri == "" {
		return
	}

	// Set appropriate background color based on page type to prevent dark background bleeding.
	// Type-assert to access webkit-specific background color methods.
	if wkWV, ok := wv.(*webkit.WebView); ok {
		switch {
		case strings.HasPrefix(uri, "dumb://"):
			// Internal pages: apply themed background
			theme, ok := c.getCurrentTheme()
			if ok && theme.prefersDark {
				wkWV.SetBackgroundColor(darkBgR, darkBgG, darkBgB, darkBgA)
			} else {
				wkWV.ResetBackgroundToDefault()
			}
		case strings.HasPrefix(uri, "about:"):
			// Keep pool background (no action)
		default:
			// External pages: white background
			wkWV.ResetBackgroundToDefault()
		}
	}

	// Show the WebView now that content is being painted
	// (WebViews are hidden on creation to avoid white flash)
	// Skip showing if this is about:blank but the pane is loading a different URL
	// This prevents the brief flash of about:blank during initial navigation
	shouldShow := true
	if uri == aboutBlankURI {
		// Get the pane's intended URI from the workspace
		ws, _ := c.getActiveWS()
		if ws != nil {
			if paneNode := ws.FindPane(paneID); paneNode != nil && paneNode.Pane != nil {
				// Don't show about:blank if the pane is supposed to load a different URL
				if paneNode.Pane.URI != "" && paneNode.Pane.URI != aboutBlankURI {
					shouldShow = false
					log.Debug().
						Str("pane_id", string(paneID)).
						Str("pane_uri", paneNode.Pane.URI).
						Msg("skipping webview show for about:blank (pane loading different URL)")
				}
			}
		}
	}

	if !shouldShow {
		// Avoid updating UI/domain state to about:blank when we know the pane is
		// navigating to a different URL. This prevents the omnibox/window title from
		// briefly showing about:blank on cold start.
		c.clearPendingReveal(paneID)
		return
	}

	if !c.applyPendingThemeUpdate(ctx, paneID, wv) {
		c.applyCurrentTheme(ctx, paneID, wv)
	}

	c.markPendingReveal(paneID)
	if wv.EstimatedProgress() > 0 {
		c.revealIfPending(ctx, paneID, uri, "progress-after-commit")
	}

	// Update domain model with current URI for session snapshots
	c.updatePaneURI(paneID, uri)

	// Sync StackedView title bar with the WebView's current title.
	// This keeps the stacked title bar up-to-date immediately on navigation,
	// before the asynchronous notify::title signal fires.
	if title := wv.Title(); title != "" {
		c.syncStackedTitle(ctx, paneID, title)
	}

	// Record history - URI is guaranteed to be correct at LoadCommitted
	if c.onHistoryRecord != nil {
		c.onHistoryRecord(ctx, paneID, uri)
	}

	// Resolve favicon for stacked title bar (fills gap when WebKit doesn't emit notify::favicon)
	c.resolveCommittedFavicon(ctx, paneID, wv)

	// Notify active pane navigation for permission indicator reset.
	c.notifyActiveNavigation(paneID, uri)

	// Apply zoom
	if c.zoomUC == nil {
		return
	}

	domain, err := usecase.ExtractDomain(uri)
	if err != nil {
		return
	}

	_ = c.zoomUC.ApplyToWebView(ctx, wv, domain)
}

func (c *Coordinator) notifyActiveNavigation(paneID entity.PaneID, uri string) {
	if c.onActiveNavigationCommitted == nil {
		return
	}
	ws, _ := c.getActiveWS()
	if ws != nil && ws.ActivePaneID == paneID {
		c.onActiveNavigationCommitted(uri)
	}
}

func (c *Coordinator) shouldSkipAboutBlankAppearance(paneID entity.PaneID, wv port.WebView) bool {
	if wv == nil || wv.IsDestroyed() {
		return false
	}
	if wv.URI() != aboutBlankURI {
		return false
	}
	ws, _ := c.getActiveWS()
	if ws == nil {
		return false
	}
	paneNode := ws.FindPane(paneID)
	if paneNode == nil || paneNode.Pane == nil {
		return false
	}
	if paneNode.Pane.URI != "" && paneNode.Pane.URI != aboutBlankURI {
		return true
	}
	return false
}

// onSPANavigation records history when URL changes via JavaScript (History API).
// This handles SPA navigation like YouTube search, where the URL changes without a page load.
func (c *Coordinator) onSPANavigation(ctx context.Context, paneID entity.PaneID, uri string) {
	// Update domain model with current URI for session snapshots
	c.updatePaneURI(paneID, uri)

	// Record history for SPA navigation
	if c.onHistoryRecord != nil {
		c.onHistoryRecord(ctx, paneID, uri)
	}
}

// updatePaneURI updates the pane's URI in the domain model.
// This is called on navigation so that session snapshots capture the current URL.
func (c *Coordinator) updatePaneURI(paneID entity.PaneID, uri string) {
	if c.onPaneURIUpdated != nil {
		c.onPaneURIUpdated(paneID, uri)
	}
}

// onLoadStarted shows the progress bar when page loading begins.
func (c *Coordinator) onLoadStarted(paneID entity.PaneID) {
	logging.Trace().Mark("load_started")

	// Trigger deferred initialization on first load_started.
	// This ensures non-critical init runs after initial navigation starts.
	c.loadStartedOnce.Do(func() {
		if c.onFirstLoadStarted != nil {
			c.onFirstLoadStarted()
		}
	})

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
func (c *Coordinator) onLoadFinished(ctx context.Context, paneID entity.PaneID, wv port.WebView) {
	_, wsView := c.getActiveWS()
	if wsView == nil {
		return
	}

	paneView := wsView.GetPaneView(paneID)
	if paneView != nil {
		paneView.SetLoading(false)
	}

	c.revealIfPending(ctx, paneID, "", "load-finished")
	if c.shouldSkipAboutBlankAppearance(paneID, wv) {
		return
	}
	c.applyPendingThemeUpdate(ctx, paneID, wv)
	c.refreshPendingScripts(ctx, paneID, wv)
}

// onProgressChanged updates the progress bar with current load progress.
func (c *Coordinator) onProgressChanged(paneID entity.PaneID, progress float64) {
	if progress > 0 {
		c.revealIfPending(context.Background(), paneID, "", "progress")
	}

	_, wsView := c.getActiveWS()
	if wsView == nil {
		return
	}

	paneView := wsView.GetPaneView(paneID)
	if paneView != nil {
		paneView.SetLoadProgress(progress)
	}
}

func (c *Coordinator) markPendingReveal(paneID entity.PaneID) {
	c.revealMu.Lock()
	c.pendingReveal[paneID] = true
	c.revealMu.Unlock()
}

func (c *Coordinator) clearPendingReveal(paneID entity.PaneID) {
	c.revealMu.Lock()
	delete(c.pendingReveal, paneID)
	c.revealMu.Unlock()
}

func (c *Coordinator) revealIfPending(ctx context.Context, paneID entity.PaneID, uri, reason string) {
	c.revealMu.Lock()
	pending := c.pendingReveal[paneID]
	if pending {
		delete(c.pendingReveal, paneID)
	}
	c.revealMu.Unlock()

	if !pending {
		return
	}

	wv := c.getWebViewLocked(paneID)
	if wv == nil || wv.IsDestroyed() {
		return
	}

	// Type-assert to access webkit-specific Widget() for visibility control
	if wkWV, ok := wv.(*webkit.WebView); ok {
		if inner := wkWV.Widget(); inner != nil {
			inner.SetVisible(true)
			logging.FromContext(ctx).
				Debug().
				Str("pane_id", string(paneID)).
				Str("uri", uri).
				Str("reason", reason).
				Msg("webview revealed")
		}
	}

	// Mark first_paint and finish startup trace
	logging.Trace().Mark("first_paint")
	logging.Trace().Finish()

	if c.onWebViewShown != nil {
		c.onWebViewShown(paneID)
	}
}

// onLinkHover updates the link status overlay when hovering over links.
func (c *Coordinator) onLinkHover(paneID entity.PaneID, uri string) {
	_, wsView := c.getActiveWS()
	if wsView == nil {
		return
	}

	paneView := wsView.GetPaneView(paneID)
	if paneView == nil {
		return
	}

	if uri != "" {
		paneView.ShowLinkStatus(uri)
	} else {
		paneView.HideLinkStatus()
	}
}

// handleURIChanged handles URI changes from WebKit, including external scheme detection
// and SPA navigation tracking.
func (c *Coordinator) handleURIChanged(ctx context.Context, paneID entity.PaneID, wv port.WebView, uri string) {
	if uri == "" {
		return
	}

	log := logging.FromContext(ctx)

	// Check for external URL schemes (vscode://, vscode-insiders://, spotify://, etc.)
	// These are typically triggered by JavaScript redirects (window.location)
	isExternal := urlutil.IsExternalScheme(uri)

	if isExternal {
		log.Info().Str("pane_id", string(paneID)).Str("uri", uri).Msg("external scheme detected, launching externally")

		// Launch externally
		desktop.LaunchExternalURL(uri)

		// Stop loading to prevent WebKit from showing an error page
		// The page stays on the previous URL before the JS redirect
		if err := wv.Stop(ctx); err != nil {
			log.Warn().Str("pane_id", string(paneID)).Str("uri", uri).Err(err).Msg("stop webview for external URL")
		}

		// Navigate back to avoid stale URI in omnibox/history
		if wv.CanGoBack() {
			if err := wv.GoBack(ctx); err != nil {
				log.Warn().Str("pane_id", string(paneID)).Err(err).Msg("GoBack after external URL")
			}
		}
		return
	}

	if !wv.IsLoading() {
		c.onSPANavigation(ctx, paneID, uri)
	}
}
