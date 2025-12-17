package coordinator

import (
	"time"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/infrastructure/config"
	"github.com/bnema/dumber/internal/infrastructure/webkit"
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

// PendingPopup tracks a popup WebView that has been created but not yet
// inserted into the workspace. This is used during the three-phase popup
// lifecycle: create → ready-to-show → (insert into workspace).
type PendingPopup struct {
	// WebView is the related WebView created in the "create" phase.
	// It shares cookies/session with the parent for OAuth support.
	WebView *webkit.WebView

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

// DetectPopupType determines if this is a tab-like or JS popup based on frame name.
func DetectPopupType(frameName string) PopupType {
	// _blank is the standard indicator for "open in new tab/window"
	if frameName == "_blank" {
		return PopupTypeTab
	}
	// Empty or custom frame names indicate JS window.open()
	return PopupTypePopup
}

// InsertPopupInput contains the data needed to insert a popup into the workspace.
type InsertPopupInput struct {
	// ParentPaneID is the pane that spawned this popup.
	ParentPaneID entity.PaneID

	// PopupPane is the pre-created pane entity for the popup.
	// It should have IsRelated=true and ParentPaneID set.
	PopupPane *entity.Pane

	// WebView is the related WebView for the popup.
	WebView *webkit.WebView

	// Behavior determines how the popup is inserted (split/stacked/tabbed).
	Behavior config.PopupBehavior

	// Placement specifies direction for split behavior (right/left/top/bottom).
	Placement string

	// PopupType indicates if this is a tab-like or JS popup.
	PopupType PopupType

	// TargetURI is the URL to load in the popup.
	TargetURI string
}

// GetBehavior returns the appropriate behavior based on popup type and config.
func GetBehavior(popupType PopupType, cfg *config.PopupBehaviorConfig) config.PopupBehavior {
	if cfg == nil {
		return config.PopupBehaviorSplit // Default
	}

	switch popupType {
	case PopupTypeTab:
		// Tab-like popups (_blank) use blank_target_behavior
		switch cfg.BlankTargetBehavior {
		case "split":
			return config.PopupBehaviorSplit
		case "stacked":
			return config.PopupBehaviorStacked
		case "tabbed":
			return config.PopupBehaviorTabbed
		default:
			return config.PopupBehaviorStacked // Default for _blank
		}
	case PopupTypePopup:
		// JS popups use the main behavior setting
		return cfg.Behavior
	default:
		return cfg.Behavior
	}
}
