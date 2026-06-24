package content

import (
	"context"
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

// NativePopupInput contains the data needed to host a native-required popup in
// a dedicated top-level GTK window instead of the workspace.
type NativePopupInput struct {
	ParentPaneID          entity.PaneID
	ParentWebViewID       port.WebViewID
	ParentURIAtOpen       string
	PopupWebView          port.WebView
	TargetURI             string
	Request               port.PopupRequest
	ObserveOAuthAutoClose bool
}

// GetBehavior returns the appropriate behavior based on popup type and config.
func GetBehavior(popupType PopupType, cfg *entity.BrowsingContextConfig) entity.PopupBehavior {
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
	popupConfig *entity.BrowsingContextConfig,
	generateID func() string,
) {
	c.ensurePopupManager().setConfig(factory, popupConfig, generateID)
}

func (c *Coordinator) UpdatePopupConfig(popupConfig entity.BrowsingContextConfig) {
	c.ensurePopupManager().updatePopupConfig(popupConfig)
}

func (c *Coordinator) SetPopupWindowIDResolver(fn func(entity.PaneID) (string, bool)) {
	c.ensurePopupManager().setWindowIDResolver(fn)
}

func (c *Coordinator) ClearPopupNamedContextsForWindow(windowID string) {
	c.ensurePopupManager().clearReusableNamedPopupsForWindow(windowID)
}

// SetOnInsertPopup sets the callback to insert popups into the workspace.
func (c *Coordinator) SetOnInsertPopup(fn func(ctx context.Context, input InsertPopupInput) error) {
	c.ensurePopupManager().setOnInsertPopup(fn)
}

// SetOnClosePane sets the callback to close a pane when its popup closes.
func (c *Coordinator) SetOnClosePane(fn func(ctx context.Context, paneID entity.PaneID) error) {
	c.ensurePopupManager().setOnClosePane(fn)
}

func (c *Coordinator) SetOnOpenNativePopup(fn func(ctx context.Context, input NativePopupInput) error) {
	c.ensurePopupManager().setOnOpenNativePopup(fn)
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
	return c.ensurePopupManager().handlePopupCreate(ctx, c.popupHooks(), parentPaneID, parentWV, req)
}

// handleLinkMiddleClick handles middle-click / Ctrl+click on links.
// Opens the link in a new pane using blank_target_behavior config.
func (c *Coordinator) handleLinkMiddleClick(ctx context.Context, parentPaneID entity.PaneID, uri string) bool {
	return c.ensurePopupManager().handleLinkMiddleClick(ctx, c.popupHooks(), parentPaneID, uri)
}
