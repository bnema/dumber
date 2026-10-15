package content

import (
	"context"
	"strings"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/domain/entity"
	urlutil "github.com/bnema/dumber/internal/domain/url"
	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/dumber/internal/ui/component"
	"github.com/bnema/puregotk/v4/gtk"
)

// onLoadCommitted re-applies zoom when page content starts loading and records history.
// WebKit may reset zoom during document transitions, so we reapply after LoadCommitted.
// History is recorded here because the URI is guaranteed to be correct after commit.
// Also shows the WebView widget (it's hidden during creation to avoid white flash).
func (c *Coordinator) onLoadCommitted(ctx context.Context, paneID entity.PaneID, wv port.WebView, identity webViewIdentity) {
	log := logging.FromContext(ctx)

	uri := wv.URI()
	if uri == "" {
		return
	}

	// Set appropriate background color based on page type to prevent dark background bleeding.
	switch {
	case strings.HasPrefix(uri, "dumb://"):
		// Internal pages: apply themed background
		theme, ok := c.getCurrentTheme()
		if ok && theme.prefersDark {
			wv.SetBackgroundColor(darkBgR, darkBgG, darkBgB, darkBgA)
		} else {
			wv.ResetBackgroundToDefault()
		}
	case strings.HasPrefix(uri, "about:"):
		// Keep pool background (no action)
	default:
		// External pages: white background
		wv.ResetBackgroundToDefault()
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
		c.clearPendingReveal(paneID, identity)
		return
	}

	if !c.applyPendingThemeUpdate(ctx, paneID, wv) {
		c.applyCurrentTheme(ctx, paneID, wv)
	}

	c.markPendingReveal(paneID, identity)
	c.revealIfPending(ctx, paneID, wv, identity, uri, "progress-after-commit")

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

	zoomKey, err := usecase.ExtractZoomKey(uri)
	if err != nil {
		log.Debug().Err(err).Str("uri", uri).Msg("skipping zoom application: cannot extract zoom key")
		return
	}

	_ = c.zoomUC.ApplyToWebView(ctx, wv, zoomKey)
}

func (c *Coordinator) notifyActiveNavigation(paneID entity.PaneID, uri string) {
	if c.onActiveNavigationCommitted == nil {
		return
	}
	c.activePaneOverrideMu.RLock()
	override := c.activePaneOverride
	c.activePaneOverrideMu.RUnlock()
	if override != "" {
		if override == paneID {
			c.onActiveNavigationCommitted(paneID, uri)
		}
		return
	}
	ws, _ := c.getActiveWS()
	if ws != nil && ws.ActivePaneID == paneID {
		c.onActiveNavigationCommitted(paneID, uri)
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
	// Trigger deferred initialization on first load_started.
	// This ensures non-critical init runs after initial navigation starts.
	c.loadStartedOnce.Do(func() {
		if c.onFirstLoadStarted != nil {
			c.onFirstLoadStarted()
		}
	})

	_, wsView := c.getActiveWS()
	var paneView *component.PaneView
	if wsView != nil {
		paneView = wsView.GetPaneView(paneID)
	}

	if wsView == nil {
		return
	}
	if paneView != nil {
		paneView.SetLoading(true)
	}
}

// onLoadFinished hides the progress bar when page loading completes.
func (c *Coordinator) onLoadFinished(ctx context.Context, paneID entity.PaneID, wv port.WebView, identity webViewIdentity) {
	_, wsView := c.getActiveWS()
	var paneView *component.PaneView
	if wsView != nil {
		paneView = wsView.GetPaneView(paneID)
	}

	if wsView == nil {
		return
	}
	if paneView != nil {
		paneView.SetLoading(false)
	}

	c.revealIfPending(ctx, paneID, wv, identity, "", "load-finished")
	if c.shouldSkipAboutBlankAppearance(paneID, wv) {
		return
	}
	c.applyPendingThemeUpdate(ctx, paneID, wv)
	c.refreshPendingScripts(ctx, paneID, wv)
}

// onProgressChanged updates the progress bar with current load progress.
func (c *Coordinator) onProgressChanged(paneID entity.PaneID, wv port.WebView, identity webViewIdentity, progress float64) {
	if progress > 0 {
		c.revealIfPending(context.Background(), paneID, wv, identity, "", "progress")
	}

	_, wsView := c.getActiveWS()
	var paneView *component.PaneView
	if wsView != nil {
		paneView = wsView.GetPaneView(paneID)
	}

	if wsView == nil {
		return
	}
	if paneView != nil {
		paneView.SetLoadProgress(progress)
	}
}

func (c *Coordinator) markPendingReveal(paneID entity.PaneID, identity webViewIdentity) {
	c.webViewsMu.RLock()
	c.revealMu.Lock()
	defer c.revealMu.Unlock()
	defer c.webViewsMu.RUnlock()
	currentWebView := c.webViews[paneID]
	if currentWebView == nil {
		return
	}
	currentIdentity, ok := identityForWebView(currentWebView)
	if !ok || currentIdentity != identity {
		return
	}
	c.ensureRevealMapsLocked()
	c.pendingReveal[paneID] = identity
}

func (c *Coordinator) clearPendingReveal(paneID entity.PaneID, identity webViewIdentity) {
	c.webViewsMu.RLock()
	c.revealMu.Lock()
	defer c.revealMu.Unlock()
	defer c.webViewsMu.RUnlock()
	pendingIdentity, pending := c.pendingReveal[paneID]
	if !pending || pendingIdentity != identity {
		return
	}
	delete(c.pendingReveal, paneID)
}

// WebViewRevealed reports whether this pane's current WebView has completed a
// reveal. State is identity-bound so a replacement cannot inherit it.
func (c *Coordinator) WebViewRevealed(paneID entity.PaneID) bool {
	return c.webViewRevealed(paneID, nil)
}

// webViewRevealed verifies the identity being attached when one is supplied.
// This prevents a rebuild from applying a replacement's reveal state to a
// previously read pooled WebView.
func (c *Coordinator) webViewRevealed(paneID entity.PaneID, expected port.WebView) bool {
	c.webViewsMu.RLock()
	c.revealMu.Lock()
	defer c.revealMu.Unlock()
	defer c.webViewsMu.RUnlock()
	current := c.webViews[paneID]
	identity, ok := identityForWebView(current)
	if !ok || expected != nil {
		expectedIdentity, expectedOK := identityForWebView(expected)
		if !expectedOK || expectedIdentity != identity {
			return false
		}
	}
	revealedIdentity, revealed := c.revealedWebViews[paneID]
	return revealed && revealedIdentity == identity
}

func (c *Coordinator) markWebViewRevealed(paneID entity.PaneID, identity webViewIdentity) bool {
	c.webViewsMu.RLock()
	c.revealMu.Lock()
	defer c.revealMu.Unlock()
	defer c.webViewsMu.RUnlock()
	currentIdentity, current := identityForWebView(c.webViews[paneID])
	if !current || currentIdentity != identity {
		return false
	}
	c.ensureRevealMapsLocked()
	c.revealedWebViews[paneID] = identity
	return true
}

func (c *Coordinator) revealIfPending(
	ctx context.Context, paneID entity.PaneID, wv port.WebView, identity webViewIdentity, uri, reason string,
) {
	// Mapping replacement and release acquire this gate too. No ordinary map
	// lock is held across the native call, but the gate keeps the validated
	// identity current until its widget visibility has been changed.
	c.revealMutationMu.Lock()

	c.webViewsMu.RLock()
	c.revealMu.Lock()
	pendingIdentity, pending := c.pendingReveal[paneID]
	if !pending || pendingIdentity != identity {
		c.revealMu.Unlock()
		c.webViewsMu.RUnlock()
		c.revealMutationMu.Unlock()
		return
	}
	currentIdentity, current := identityForWebView(c.webViews[paneID])
	if !current || currentIdentity != identity {
		c.revealMu.Unlock()
		c.webViewsMu.RUnlock()
		c.revealMutationMu.Unlock()
		return
	}
	delete(c.pendingReveal, paneID)
	c.revealMu.Unlock()
	c.webViewsMu.RUnlock()

	if wv.IsDestroyed() {
		c.revealMutationMu.Unlock()
		return
	}

	if c.setWebViewVisible != nil {
		c.setWebViewVisible(wv)
	} else if nwp, ok := wv.(port.NativeWidgetProvider); ok {
		if ptr := nwp.NativeWidget(); ptr != 0 {
			gtkWidget := &gtk.Widget{}
			gtkWidget.Ptr = ptr
			gtkWidget.SetVisible(true)
			logging.FromContext(ctx).
				Debug().
				Str("pane_id", string(paneID)).
				Str("uri", uri).
				Str("reason", reason).
				Msg("webview revealed")
		}
	}

	// Keep the presentation record in the same serialized operation as the
	// native mutation. User callbacks run only after the gate is released.
	revealed := c.markWebViewRevealed(paneID, identity)
	c.revealMutationMu.Unlock()
	if revealed && c.onWebViewShown != nil {
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

// externalPrompt is a pane's outstanding external-launch confirmation.
type externalPrompt struct {
	token    uint64
	identity webViewIdentity
}

// promptExternalLaunch asks the user whether the external URI may be handed to
// the system handler. A pane has at most one outstanding prompt: further
// external URIs from that pane are dropped until it is answered, so a page
// cannot flood the window's prompt queue.
func (c *Coordinator) promptExternalLaunch(ctx context.Context, paneID entity.PaneID, wv port.WebView, uri string) {
	log := logging.FromContext(ctx)
	identity, ok := identityForWebView(wv)
	if !ok || c.onConfirmExternalLaunch == nil || c.onLaunchExternalURL == nil {
		log.Info().Str("pane_id", string(paneID)).Msg("external scheme launch unavailable; blocked")
		return
	}

	c.externalPromptMu.Lock()
	if _, pending := c.externalPrompts[paneID]; pending {
		c.externalPromptMu.Unlock()
		return
	}
	if c.externalPrompts == nil {
		c.externalPrompts = make(map[entity.PaneID]externalPrompt)
	}
	c.externalPromptSeq++
	token := c.externalPromptSeq
	c.externalPrompts[paneID] = externalPrompt{token: token, identity: identity}
	origin := c.externalPromptOrigins[paneID]
	c.externalPromptMu.Unlock()

	c.onConfirmExternalLaunch(paneID, origin, uri, func(allowed bool) {
		c.externalPromptMu.Lock()
		prompt, pending := c.externalPrompts[paneID]
		// The prompt is stale when its pane was released, rebound, or
		// re-prompted while it was queued: never launch for it.
		current := pending && prompt.token == token
		if current {
			delete(c.externalPrompts, paneID)
		}
		c.externalPromptMu.Unlock()
		bound, ok := identityForWebView(c.GetWebView(paneID))
		current = current && ok && bound == identity

		log.Info().Str("pane_id", string(paneID)).Bool("allowed", allowed).Bool("stale", !current).
			Str("uri", logging.TruncateURL(uri, logging.PermissionLogURLMaxLen)).
			Msg("external scheme launch decision")
		if allowed && current {
			c.onLaunchExternalURL(uri)
		}
	})
}

// rememberExternalPromptOrigin records the host of the last web page shown in
// the pane, so a later external-launch prompt can name the requesting site.
func (c *Coordinator) rememberExternalPromptOrigin(paneID entity.PaneID, uri string) {
	host := logging.SafeURLHost(uri)
	if host == "" {
		return
	}
	c.externalPromptMu.Lock()
	if c.externalPromptOrigins == nil {
		c.externalPromptOrigins = make(map[entity.PaneID]string)
	}
	c.externalPromptOrigins[paneID] = host
	c.externalPromptMu.Unlock()
}

// clearExternalPrompt invalidates the pane's outstanding external-launch
// prompt so a late answer cannot launch anything, and forgets its origin.
func (c *Coordinator) clearExternalPrompt(paneID entity.PaneID) {
	c.externalPromptMu.Lock()
	delete(c.externalPrompts, paneID)
	delete(c.externalPromptOrigins, paneID)
	c.externalPromptMu.Unlock()
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
		log.Info().Str("pane_id", string(paneID)).Str("uri", logging.TruncateURL(uri, logging.PermissionLogURLMaxLen)).
			Msg("external scheme URI change intercepted")

		if err := wv.Stop(ctx); err != nil {
			log.Warn().Str("pane_id", string(paneID)).Err(err).Msg("stop webview for external URL")
		}

		// URI changes can be driven by page script or redirects (OAuth flows end
		// with a redirect to claude://, vscode://, ...), so the launch is never
		// automatic: the user must confirm it. Do not navigate back: going back
		// to an OAuth endpoint re-issues the redirect and loops.
		c.promptExternalLaunch(ctx, paneID, wv, uri)
		return
	}

	c.rememberExternalPromptOrigin(paneID, uri)
	if !wv.IsLoading() {
		c.onSPANavigation(ctx, paneID, uri)
	}
}
