package content

import (
	"context"
	"errors"
	"fmt"
	neturl "net/url"
	"strings"
	"time"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/logging"
)

// PopupType indicates whether the popup was triggered by a link or JavaScript.
type PopupType int

const (
	// PopupTypeTab represents a popup from target="_blank" links.
	// These are typically user-initiated and open like new tabs.
	PopupTypeTab PopupType = iota
	// PopupTypePopup represents a popup from window.open() JavaScript calls.
	// These may have different placement behavior than tab-like popups.
	PopupTypePopup
)

// String returns a human-readable name for the popup type.
func (t PopupType) String() string {
	switch t {
	case PopupTypeTab:
		return "tab"
	case PopupTypePopup:
		return "popup"
	default:
		return "unknown"
	}
}

// DetectPopupType determines if this is a tab-like or JS popup based on frame name.
func DetectPopupType(frameName string) PopupType {
	// _blank is the standard indicator for "open in new tab/window"
	if frameName == "_blank" {
		return PopupTypeTab
	}
	// Empty or custom frame names indicate JS window.open()
	return PopupTypePopup
}

// PendingPopup tracks a popup WebView that has been created but not yet
// inserted into the workspace. This is used during the three-phase popup
// lifecycle: create → ready-to-show → (insert into workspace).
type PendingPopup struct {
	// PaneID is the popup pane created during the create phase.
	PaneID entity.PaneID

	// WebView is the related WebView created in the "create" phase.
	// It shares cookies/session with the parent for OAuth support.
	WebView port.WebView

	// ParentPaneID is the pane that spawned this popup.
	ParentPaneID entity.PaneID

	// ParentWebViewID is the WebView ID of the parent (for lookup).
	ParentWebViewID port.WebViewID

	// TargetURI is the initial URL the popup will load.
	TargetURI string

	// FrameName is the target frame name from the navigation action.
	// "_blank" indicates a tab-like popup, other values indicate JS popups.
	FrameName string

	// IsUserGesture indicates if the popup was triggered by user action
	// (click) vs script-initiated (e.g., popup after timeout).
	IsUserGesture bool

	// PopupType categorizes this as a tab-like or JS popup.
	PopupType PopupType

	// CreatedAt is when the popup was created (for timeout handling).
	CreatedAt time.Time
}

type namedPopupKey struct {
	ParentPaneID entity.PaneID
	FrameName    string
}

type namedPopupState struct {
	WebView port.WebView
}

func isReusableNamedPopupFrame(frameName string) bool {
	return frameName != "" && frameName != "_blank"
}

func (c *Coordinator) lookupReusableNamedPopup(parentPaneID entity.PaneID, frameName string) (*namedPopupState, bool) {
	if !isReusableNamedPopupFrame(frameName) {
		return nil, false
	}

	key := namedPopupKey{ParentPaneID: parentPaneID, FrameName: frameName}

	c.popupMu.RLock()
	state, ok := c.namedPopups[key]
	c.popupMu.RUnlock()
	if !ok || state == nil || state.WebView == nil {
		return nil, false
	}

	if state.WebView.IsDestroyed() {
		c.popupMu.Lock()
		if current, ok := c.namedPopups[key]; ok && current == state {
			delete(c.namedPopups, key)
		}
		c.popupMu.Unlock()
		return nil, false
	}

	return state, true
}

func (c *Coordinator) storeReusableNamedPopup(
	parentPaneID entity.PaneID,
	frameName string,
	wv port.WebView,
) {
	if !isReusableNamedPopupFrame(frameName) || wv == nil {
		return
	}

	key := namedPopupKey{ParentPaneID: parentPaneID, FrameName: frameName}

	c.popupMu.Lock()
	if c.namedPopups == nil {
		c.namedPopups = make(map[namedPopupKey]*namedPopupState)
	}
	c.namedPopups[key] = &namedPopupState{WebView: wv}
	c.popupMu.Unlock()
}

func (c *Coordinator) updatePendingPopupTarget(popupID port.WebViewID, targetURI string) {
	c.popupMu.Lock()
	if pending, ok := c.pendingPopups[popupID]; ok && pending != nil {
		pending.TargetURI = targetURI
	}
	c.popupMu.Unlock()
}

func (c *Coordinator) clearReusableNamedPopupByWebViewID(popupID port.WebViewID) {
	c.popupMu.Lock()
	for key, state := range c.namedPopups {
		if state != nil && state.WebView != nil && state.WebView.ID() == popupID {
			delete(c.namedPopups, key)
		}
	}
	c.popupMu.Unlock()
}

// InsertPopupInput contains the data needed to insert a popup into the workspace.
type InsertPopupInput struct {
	// ParentPaneID is the pane that spawned this popup.
	ParentPaneID entity.PaneID

	// PopupPane is the pre-created pane entity for the popup.
	// It should have IsRelated=true and ParentPaneID set.
	PopupPane *entity.Pane

	// WebView is the related WebView for the popup.
	WebView port.WebView

	// Behavior determines how the popup is inserted (split/stacked/tabbed).
	Behavior entity.PopupBehavior

	// Placement specifies direction for split behavior (right/left/top/bottom).
	Placement string

	// PopupType indicates if this is a tab-like or JS popup.
	PopupType PopupType

	// TargetURI is the URL to load in the popup.
	TargetURI string
}

// GetBehavior returns the appropriate behavior based on popup type and config.
func GetBehavior(popupType PopupType, cfg *entity.PopupBehaviorConfig) entity.PopupBehavior {
	if cfg == nil {
		return entity.PopupBehaviorSplit // Default
	}

	switch popupType {
	case PopupTypeTab:
		// Tab-like popups (_blank) use blank_target_behavior
		switch cfg.BlankTargetBehavior {
		case "split":
			return entity.PopupBehaviorSplit
		case "stacked":
			return entity.PopupBehaviorStacked
		case "tabbed":
			return entity.PopupBehaviorTabbed
		default:
			return entity.PopupBehaviorStacked // Default for _blank
		}
	case PopupTypePopup:
		// JS popups use the main behavior setting
		return cfg.Behavior
	default:
		return cfg.Behavior
	}
}

// SetPopupConfig configures popup handling.
func (c *Coordinator) SetPopupConfig(
	factory port.WebViewFactory,
	popupConfig *entity.PopupBehaviorConfig,
	generateID func() string,
) {
	c.factory = factory
	c.popupConfig = popupConfig
	c.generateID = generateID

	c.popupMu.Lock()
	c.relatedPopupUnsupported = false
	c.relatedPopupSupportDetected = false
	c.popupMu.Unlock()
}

// SetOnInsertPopup sets the callback to insert popups into the workspace.
func (c *Coordinator) SetOnInsertPopup(fn func(ctx context.Context, input InsertPopupInput) error) {
	c.onInsertPopup = fn
}

// SetOnClosePane sets the callback to close a pane when its popup closes.
func (c *Coordinator) SetOnClosePane(fn func(ctx context.Context, paneID entity.PaneID) error) {
	c.onClosePane = fn
}

// buildPopupCreateHandler returns the OnCreate callback for a WebView.
// Returns nil if popup handling is not configured.
func (c *Coordinator) buildPopupCreateHandler(
	ctx context.Context, paneID entity.PaneID, wv port.WebView,
) func(port.PopupRequest) port.WebView {
	if wv == nil {
		return nil
	}

	log := logging.FromContext(ctx)
	log.Debug().Str("pane_id", string(paneID)).Msg("popup handling configured for webview")

	return func(req port.PopupRequest) port.WebView {
		return c.handlePopupCreate(ctx, paneID, wv, req)
	}
}

// createPopupPane creates a new pane entity for a popup window.
func (c *Coordinator) createPopupPane(
	popupID port.WebViewID,
	parentPaneID entity.PaneID,
	targetURI string,
) (entity.PaneID, *entity.Pane) {
	var paneID entity.PaneID
	if c.generateID != nil {
		paneID = entity.PaneID(c.generateID())
	} else {
		paneID = entity.PaneID(fmt.Sprintf("popup_%d", popupID))
	}

	popupPane := entity.NewPane(paneID)
	popupPane.WindowType = entity.WindowPopup
	popupPane.IsRelated = true
	popupPane.ParentPaneID = &parentPaneID
	popupPane.URI = targetURI

	return paneID, popupPane
}

func normalizePopupTargetURIForFallback(rawTargetURI string) string {
	trimmed := strings.TrimSpace(rawTargetURI)
	if trimmed == "" {
		return rawTargetURI
	}

	parsed, err := neturl.Parse(trimmed)
	if err != nil {
		return rawTargetURI
	}

	host := strings.ToLower(parsed.Hostname())
	isNotionHost := host == "notion.so" || host == "notion.com" ||
		strings.HasSuffix(host, ".notion.so") || strings.HasSuffix(host, ".notion.com")
	if !isNotionHost || parsed.Path != "/verifyNoPopupBlockerHtmlAndRedirect" {
		return rawTargetURI
	}

	redirectURI := strings.TrimSpace(parsed.Query().Get("redirectUri"))
	if redirectURI == "" {
		return rawTargetURI
	}

	redirectParsed, err := neturl.Parse(redirectURI)
	if err != nil {
		return rawTargetURI
	}
	if redirectParsed.Scheme != "https" && redirectParsed.Scheme != "http" {
		return rawTargetURI
	}

	return redirectParsed.String()
}

func (c *Coordinator) relatedPopupSupportDisabled() bool {
	c.popupMu.RLock()
	defer c.popupMu.RUnlock()
	return c.relatedPopupSupportDetected && c.relatedPopupUnsupported
}

func (c *Coordinator) markRelatedPopupUnsupported() {
	c.popupMu.Lock()
	defer c.popupMu.Unlock()
	c.relatedPopupSupportDetected = true
	c.relatedPopupUnsupported = true
}

func (c *Coordinator) markRelatedPopupSupported() {
	c.popupMu.Lock()
	defer c.popupMu.Unlock()
	c.relatedPopupSupportDetected = true
	c.relatedPopupUnsupported = false
}

// createPopupWebView prefers a related WebView so popups can share the
// parent session/context. If the engine does not support related popup views,
// it gracefully falls back to a regular WebView so target="_blank" and
// window.open() still open in a workspace pane.
func (c *Coordinator) createPopupWebView(
	ctx context.Context,
	parentID port.WebViewID,
	targetURI string,
	noJavaScriptAccess bool,
) (port.WebView, string, error) {
	if c.factory == nil {
		return nil, targetURI, fmt.Errorf("no webview factory configured")
	}

	log := logging.FromContext(ctx)
	relatedErr := error(nil)

	if !noJavaScriptAccess && !c.relatedPopupSupportDisabled() {
		popupWV, err := c.factory.CreateRelated(ctx, parentID)
		if err == nil && popupWV != nil {
			c.markRelatedPopupSupported()
			return popupWV, targetURI, nil
		}
		if err == nil {
			err = fmt.Errorf("related popup webview factory returned nil without error")
		}
		relatedErr = err
		if errors.Is(err, port.ErrRelatedWebViewUnsupported) {
			c.markRelatedPopupUnsupported()
		}

		log.Debug().
			Err(err).
			Uint64("parent_webview_id", uint64(parentID)).
			Str("target_uri", logging.TruncateURL(targetURI, logURLMaxLen)).
			Msg("related popup webview unavailable, falling back to regular webview")
	} else if noJavaScriptAccess {
		log.Debug().
			Uint64("parent_webview_id", uint64(parentID)).
			Str("target_uri", logging.TruncateURL(targetURI, logURLMaxLen)).
			Msg("popup requested no JavaScript opener access, creating regular webview")
	} else {
		log.Debug().
			Uint64("parent_webview_id", uint64(parentID)).
			Str("target_uri", logging.TruncateURL(targetURI, logURLMaxLen)).
			Msg("related popup webviews known unsupported, creating regular webview")
	}

	popupWV, fallbackErr := c.factory.Create(ctx)
	if fallbackErr != nil {
		if relatedErr != nil {
			return nil, targetURI, fmt.Errorf("create popup webview: related failed: %w; fallback failed: %w", relatedErr, fallbackErr)
		}
		return nil, targetURI, fmt.Errorf("create popup webview: fallback failed: %w", fallbackErr)
	}
	if popupWV == nil {
		return nil, targetURI, fmt.Errorf("popup webview factory returned nil without error")
	}

	effectiveTargetURI := normalizePopupTargetURIForFallback(targetURI)
	if effectiveTargetURI != targetURI {
		log.Debug().
			Str("original_target_uri", logging.TruncateURL(targetURI, logURLMaxLen)).
			Str("effective_target_uri", logging.TruncateURL(effectiveTargetURI, logURLMaxLen)).
			Msg("rewrote popup target URI for regular-webview fallback")
	}

	return popupWV, effectiveTargetURI, nil
}

// handlePopupCreate handles a popup request from the current WebView.
// Returns a WebView if popup handling is allowed, nil to block.
//
// IMPORTANT: The WebView MUST be added to a GtkWindow hierarchy BEFORE this
// signal handler returns for engines that require native popup lifecycles.
// The WebView stays hidden until ready-to-show signal is received.
func (c *Coordinator) handlePopupCreate(
	ctx context.Context,
	parentPaneID entity.PaneID,
	parentWV port.WebView,
	req port.PopupRequest,
) port.WebView {
	log := logging.FromContext(ctx)

	log.Debug().
		Str("parent_pane", string(parentPaneID)).
		Str("target_uri", logging.TruncateURL(req.TargetURI, logURLMaxLen)).
		Str("frame_name", req.FrameName).
		Bool("user_gesture", req.IsUserGesture).
		Bool("no_javascript_access", req.NoJavaScriptAccess).
		Msg("popup create request")

	// Check if popups are enabled
	if c.popupConfig != nil && !c.popupConfig.OpenInNewPane {
		log.Debug().Msg("popups disabled by config, blocking")
		return nil
	}

	// Check if factory is available
	if c.factory == nil {
		log.Warn().Msg("no webview factory, cannot create popup")
		return nil
	}

	parentID := parentWV.ID()
	parentURIAtOpen := ""
	if parent := c.getWebViewLocked(parentPaneID); parent != nil {
		parentURIAtOpen = parent.URI()
	}
	if !req.NoJavaScriptAccess {
		if reused, ok := c.reuseNamedPopup(ctx, parentPaneID, req.FrameName, req.TargetURI); ok {
			return reused
		}
	}

	// Prefer a related WebView for session sharing, but fall back to a regular
	// WebView on engines like CEF OSR that do not implement native related
	// popup views yet.
	popupWV, effectiveTargetURI, err := c.createPopupWebView(ctx, parentID, req.TargetURI, req.NoJavaScriptAccess)
	if err != nil {
		log.Error().Err(err).Msg("failed to create webview for popup")
		return nil
	}

	if effectiveTargetURI != req.TargetURI {
		req.TargetURI = effectiveTargetURI
	}

	// Detect popup type
	popupType := DetectPopupType(req.FrameName)
	popupID := popupWV.ID()

	// Determine behavior from config
	behavior := GetBehavior(popupType, c.popupConfig)
	placement := "right"
	if c.popupConfig != nil {
		placement = c.popupConfig.Placement
	}

	// Create popup pane entity
	paneID, popupPane := c.createPopupPane(popupID, parentPaneID, req.TargetURI)

	return c.finishPopupCreate(ctx, parentPaneID, parentID, parentURIAtOpen, popupID, popupWV, popupPane, paneID, popupType, behavior, placement, req)
}

func (c *Coordinator) reuseNamedPopup(
	ctx context.Context,
	parentPaneID entity.PaneID,
	frameName string,
	targetURI string,
) (port.WebView, bool) {
	log := logging.FromContext(ctx)

	if existing, ok := c.lookupReusableNamedPopup(parentPaneID, frameName); ok {
		c.updatePendingPopupTarget(existing.WebView.ID(), targetURI)
		if err := existing.WebView.LoadURI(ctx, targetURI); err != nil {
			log.Warn().Err(err).
				Str("target_uri", logging.TruncateURL(targetURI, logURLMaxLen)).
				Msg("failed to load target URI in reused popup")
		}
		log.Info().
			Str("parent_pane", string(parentPaneID)).
			Str("frame_name", frameName).
			Str("target_uri", logging.TruncateURL(targetURI, logURLMaxLen)).
			Msg("reused named popup")
		return existing.WebView, true
	}

	return nil, false
}

func (c *Coordinator) finishPopupCreate(
	ctx context.Context,
	parentPaneID entity.PaneID,
	parentID port.WebViewID,
	parentURIAtOpen string,
	popupID port.WebViewID,
	popupWV port.WebView,
	popupPane *entity.Pane,
	paneID entity.PaneID,
	popupType PopupType,
	behavior entity.PopupBehavior,
	placement string,
	req port.PopupRequest,
) port.WebView {
	log := logging.FromContext(ctx)
	hasConfig := c.popupConfig != nil
	oauthEnabled := hasConfig && c.popupConfig.OAuthAutoClose
	isOAuth := IsOAuthURL(req.TargetURI)

	log.Debug().
		Bool("has_config", hasConfig).
		Bool("oauth_enabled", oauthEnabled).
		Bool("is_oauth", isOAuth).
		Str("uri", logging.TruncateURL(req.TargetURI, logURLMaxLen)).
		Msg("popup OAuth check")

	// Register the popup WebView before workspace insertion so split/stack UI
	// updates can reuse the real popup instead of acquiring a placeholder pane
	// WebView that races the actual popup flow.
	c.setWebViewLocked(paneID, popupWV)

	// Insert into workspace IMMEDIATELY (WebView stays hidden)
	// This is required for WebKit to establish window.opener relationship
	if c.onInsertPopup != nil {
		popupInput := InsertPopupInput{
			ParentPaneID: parentPaneID,
			PopupPane:    popupPane,
			WebView:      popupWV,
			Behavior:     behavior,
			Placement:    placement,
			PopupType:    popupType,
			TargetURI:    req.TargetURI,
		}

		if err := c.onInsertPopup(ctx, popupInput); err != nil {
			log.Error().Err(err).Msg("failed to insert popup into workspace")
			if current := c.getWebViewLocked(paneID); current == popupWV {
				c.deleteWebViewLocked(paneID)
			}
			popupWV.Destroy()
			return nil
		}
	}

	// WebView already registered before insertion.
	if !req.NoJavaScriptAccess {
		c.storeReusableNamedPopup(parentPaneID, req.FrameName, popupWV)
	}

	// Setup standard callbacks (after successful insertion to avoid leak)
	c.setupWebViewCallbacks(ctx, paneID, popupWV)

	// Engines without native popup lifecycle hooks can still support
	// programmatic popup closure (proxy.close(), OAuth auto-close) by exposing
	// the optional OAuthCallbackCapable interface.
	if _, hasNativePopupLifecycle := popupWV.(port.PopupCapable); !hasNativePopupLifecycle {
		if closeCapable, ok := popupWV.(port.OAuthCallbackCapable); ok {
			closeCapable.AddCloseCallback(func() {
				c.handlePopupClose(ctx, popupID)
			})
		}
	}

	// Setup OAuth auto-close if configured
	if hasConfig && oauthEnabled && isOAuth {
		popupPane.AutoClose = true
		c.trackOAuthPopup(popupID, parentPaneID, parentURIAtOpen)
		c.setupOAuthAutoClose(ctx, paneID, popupID, popupWV)
		log.Debug().Str("pane_id", string(paneID)).Msg("OAuth auto-close enabled for popup")
	}

	// Store pending popup for ready-to-show handling (just visibility now)
	pending := &PendingPopup{
		PaneID:          paneID,
		WebView:         popupWV,
		ParentPaneID:    parentPaneID,
		ParentWebViewID: parentID,
		TargetURI:       req.TargetURI,
		FrameName:       req.FrameName,
		IsUserGesture:   req.IsUserGesture,
		PopupType:       popupType,
		CreatedAt:       time.Now(),
	}

	c.popupMu.Lock()
	c.pendingPopups[popupID] = pending
	c.popupMu.Unlock()

	// Wire ready-to-show and close signals via the PopupCapable interface.
	if pc, ok := popupWV.(port.PopupCapable); ok {
		pc.SetOnReadyToShow(func() {
			c.handlePopupReadyToShow(ctx, popupID)
		})
		pc.SetOnClose(func() {
			c.handlePopupClose(ctx, popupID)
		})
		log.Info().
			Uint64("popup_id", uint64(popupID)).
			Str("pane_id", string(paneID)).
			Str("popup_type", popupType.String()).
			Str("target_uri", logging.TruncateURL(req.TargetURI, logURLMaxLen)).
			Msg("popup inserted (hidden), awaiting ready-to-show for visibility")
	} else {
		// Engine does not support PopupCapable (e.g. CEF OSR where we create
		// an independent browser, not a real CEF popup). The WebView is ready
		// immediately — fire ready-to-show inline.
		log.Info().
			Uint64("popup_id", uint64(popupID)).
			Str("pane_id", string(paneID)).
			Str("popup_type", popupType.String()).
			Str("target_uri", logging.TruncateURL(req.TargetURI, logURLMaxLen)).
			Msg("popup inserted, immediately ready (no PopupCapable)")
		c.handlePopupReadyToShow(ctx, popupID)
	}

	return popupWV
}

// handlePopupReadyToShow handles the WebKit "ready-to-show" signal.
// The popup WebView was already inserted into the workspace (hidden) during
// the create signal. This handler just makes it visible.
func (c *Coordinator) handlePopupReadyToShow(ctx context.Context, popupID port.WebViewID) {
	log := logging.FromContext(ctx)

	// Get pending popup
	c.popupMu.Lock()
	pending, ok := c.pendingPopups[popupID]
	if ok {
		delete(c.pendingPopups, popupID)
	}
	c.popupMu.Unlock()

	if !ok || pending == nil {
		log.Warn().Uint64("popup_id", uint64(popupID)).Msg("ready-to-show for unknown popup")
		return
	}

	log.Debug().
		Uint64("popup_id", uint64(popupID)).
		Str("popup_type", pending.PopupType.String()).
		Msg("popup ready to show - making visible")

	// Make the WebView visible now that it's ready.
	if pending.WebView != nil {
		if pc, ok := pending.WebView.(port.PopupCapable); ok {
			pc.Show()
		}
		// For engines that create independent browsers (not real popups),
		// the WebView has no pending navigation — load the target URI.
		if pending.TargetURI != "" && !pending.WebView.IsLoading() && pending.WebView.URI() == "" {
			if err := pending.WebView.LoadURI(ctx, pending.TargetURI); err != nil {
				log.Warn().Err(err).
					Str("uri", logging.TruncateURL(pending.TargetURI, logURLMaxLen)).
					Msg("failed to load target URI in popup")
			}
		}
	}

	log.Info().
		Uint64("popup_id", uint64(popupID)).
		Str("target_uri", logging.TruncateURL(pending.TargetURI, logURLMaxLen)).
		Msg("popup now visible")
}

// handlePopupClose handles the WebKit "close" signal for popup windows.
func (c *Coordinator) handlePopupClose(ctx context.Context, popupID port.WebViewID) {
	log := logging.FromContext(ctx)
	log.Debug().Uint64("popup_id", uint64(popupID)).Msg("popup close signal received")

	// Check if still pending (never shown)
	c.popupMu.Lock()
	pending, wasPending := c.pendingPopups[popupID]
	if wasPending {
		delete(c.pendingPopups, popupID)
	}
	c.popupMu.Unlock()

	if wasPending && pending != nil {
		c.handlePopupOAuthClose(ctx, popupID)
		if c.onClosePane != nil {
			if err := c.onClosePane(ctx, pending.PaneID); err != nil {
				log.Warn().Err(err).Str("pane_id", string(pending.PaneID)).Msg("failed to close pending popup pane")
			}
		}
		c.clearReusableNamedPopupByWebViewID(popupID)
		if c.getWebViewLocked(pending.PaneID) != nil {
			c.ReleaseWebView(ctx, pending.PaneID)
		}
		log.Debug().Str("pane_id", string(pending.PaneID)).Msg("cleaned up pending popup that was never shown")
		return
	}

	// Find pane by WebView ID
	var paneID entity.PaneID
	c.webViewsMu.RLock()
	for pid, wv := range c.webViews {
		if wv != nil && wv.ID() == popupID {
			paneID = pid
			break
		}
	}
	c.webViewsMu.RUnlock()

	if paneID == "" {
		c.handlePopupOAuthClose(ctx, popupID)
		c.clearReusableNamedPopupByWebViewID(popupID)
		log.Warn().Msg("popup close: could not find pane for webview")
		return
	}

	c.handlePopupOAuthClose(ctx, popupID)

	// Close the pane in workspace (this removes the UI element)
	if c.onClosePane != nil {
		if err := c.onClosePane(ctx, paneID); err != nil {
			log.Warn().Err(err).Str("pane_id", string(paneID)).Msg("failed to close popup pane")
		}
	}

	c.clearReusableNamedPopupByWebViewID(popupID)

	// ClosePaneByID usually releases the WebView already; only release here if it
	// is still registered.
	if c.getWebViewLocked(paneID) != nil {
		c.ReleaseWebView(ctx, paneID)
	}

	log.Info().Str("pane_id", string(paneID)).Msg("popup closed")
}

// handleLinkMiddleClick handles middle-click / Ctrl+click on links.
// Opens the link in a new pane using blank_target_behavior config.
func (c *Coordinator) handleLinkMiddleClick(ctx context.Context, parentPaneID entity.PaneID, uri string) bool {
	log := logging.FromContext(ctx)

	log.Info().
		Str("parent_pane", string(parentPaneID)).
		Str("uri", logging.TruncateURL(uri, logURLMaxLen)).
		Msg("middle-click/ctrl+click on link")

	// Check if popups are enabled
	if c.popupConfig != nil && !c.popupConfig.OpenInNewPane {
		log.Debug().Msg("popups disabled by config, ignoring middle-click")
		return false
	}

	// Check if factory is available
	if c.factory == nil {
		log.Warn().Msg("no webview factory, cannot handle middle-click")
		return false
	}

	// Create a new WebView (regular, not related - just opening a link)
	newWV, err := c.factory.Create(ctx)
	if err != nil {
		log.Error().Err(err).Msg("failed to create webview for middle-click")
		return false
	}

	// Generate pane ID
	var paneID entity.PaneID
	if c.generateID != nil {
		paneID = entity.PaneID(c.generateID())
	} else {
		paneID = entity.PaneID(fmt.Sprintf("link_%d", newWV.ID()))
	}

	// Create pane entity - uses blank_target_behavior
	newPane := entity.NewPane(paneID)
	newPane.WindowType = entity.WindowPopup
	newPane.URI = uri

	// Register WebView
	c.setWebViewLocked(paneID, newWV)

	// Setup callbacks
	c.setupWebViewCallbacks(ctx, paneID, newWV)

	// Get behavior from config (same as _blank links)
	behavior := entity.PopupBehaviorStacked // default
	if c.popupConfig != nil && c.popupConfig.BlankTargetBehavior != "" {
		behavior = entity.PopupBehavior(c.popupConfig.BlankTargetBehavior)
	}
	placement := "right"
	if c.popupConfig != nil {
		placement = c.popupConfig.Placement
	}

	// Request insertion into workspace
	if c.onInsertPopup != nil {
		popupInput := InsertPopupInput{
			ParentPaneID: parentPaneID,
			PopupPane:    newPane,
			WebView:      newWV,
			Behavior:     behavior,
			Placement:    placement,
			PopupType:    PopupTypeTab, // Treat like _blank
			TargetURI:    uri,
		}

		if err := c.onInsertPopup(ctx, popupInput); err != nil {
			log.Error().Err(err).Msg("failed to insert middle-click pane into workspace")
			// Clean up on failure — use the locking helper to avoid a data race.
			c.deleteWebViewLocked(paneID)
			newWV.Destroy()
			return false
		}
	}

	// Load the URI after insertion
	if err := newWV.LoadURI(ctx, uri); err != nil {
		log.Error().Err(err).Str("uri", logging.TruncateURL(uri, logURLMaxLen)).Msg("failed to load URI in new pane")
	}

	log.Info().
		Str("pane_id", string(paneID)).
		Str("behavior", string(behavior)).
		Str("uri", logging.TruncateURL(uri, logURLMaxLen)).
		Msg("middle-click link opened in new pane")

	return true
}
