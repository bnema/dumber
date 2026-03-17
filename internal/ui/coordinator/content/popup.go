package content

import (
	"context"
	"fmt"
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

// handlePopupCreate handles the WebKit "create" signal for popup windows.
// Returns a new related WebView if popup is allowed, nil to block.
//
// IMPORTANT: The WebView MUST be added to a GtkWindow hierarchy BEFORE this
// signal handler returns. Otherwise WebKit won't establish window.opener.
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

	// Create related WebView for session sharing (created hidden)
	parentID := parentWV.ID()
	popupWV, err := c.factory.CreateRelated(ctx, parentID)
	if err != nil {
		log.Error().Err(err).Msg("failed to create related webview for popup")
		return nil
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

	// Check OAuth configuration
	hasConfig := c.popupConfig != nil
	oauthEnabled := hasConfig && c.popupConfig.OAuthAutoClose
	isOAuth := IsOAuthURL(req.TargetURI)
	log.Debug().
		Bool("has_config", hasConfig).
		Bool("oauth_enabled", oauthEnabled).
		Bool("is_oauth", isOAuth).
		Str("uri", logging.TruncateURL(req.TargetURI, logURLMaxLen)).
		Msg("popup OAuth check")

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
			popupWV.Destroy()
			return nil
		}
	}

	// Register WebView in our map (after successful insertion)
	c.setWebViewLocked(paneID, popupWV)

	// Setup standard callbacks (after successful insertion to avoid leak)
	c.setupWebViewCallbacks(ctx, paneID, popupWV)

	// Setup OAuth auto-close if configured
	if hasConfig && oauthEnabled && isOAuth {
		popupPane.AutoClose = true
		c.trackOAuthPopup(popupID, parentPaneID)
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
	} else {
		log.Warn().Uint64("popup_id", uint64(popupID)).Msg("webview does not support popup lifecycle callbacks (PopupCapable)")
	}

	log.Info().
		Uint64("popup_id", uint64(popupID)).
		Str("pane_id", string(paneID)).
		Str("popup_type", popupType.String()).
		Str("target_uri", logging.TruncateURL(req.TargetURI, logURLMaxLen)).
		Msg("popup inserted (hidden), awaiting ready-to-show for visibility")

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

	// Make the WebView visible now that it's ready
	if pending.WebView != nil {
		if pc, ok := pending.WebView.(port.PopupCapable); ok {
			pc.Show()
		} else {
			log.Warn().Uint64("popup_id", uint64(popupID)).Msg("webview does not support Show() (PopupCapable)")
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
		if c.onClosePane != nil {
			if err := c.onClosePane(ctx, pending.PaneID); err != nil {
				log.Warn().Err(err).Str("pane_id", string(pending.PaneID)).Msg("failed to close pending popup pane")
			}
		}
		c.handlePopupOAuthClose(ctx, popupID)
		c.ReleaseWebView(ctx, pending.PaneID)
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
		log.Warn().Msg("popup close: could not find pane for webview")
		return
	}

	// Close the pane in workspace (this removes the UI element)
	if c.onClosePane != nil {
		if err := c.onClosePane(ctx, paneID); err != nil {
			log.Warn().Err(err).Str("pane_id", string(paneID)).Msg("failed to close popup pane")
		}
	}

	c.handlePopupOAuthClose(ctx, popupID)

	// Release the WebView (this will clean up tracking)
	c.ReleaseWebView(ctx, paneID)

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
