package browser

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync/atomic"

	webkitv6 "github.com/diamondburned/gotk4-webkitgtk/pkg/webkit/v6"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/dumber/internal/webext"
	"github.com/bnema/dumber/pkg/webkit"
)

// ExtensionsOverlayManager renders a floating panel with extension icons.
// Left click opens the extension popup/options page in a new pane.
// Right click shows a context menu for enable/disable/remove/options.
type ExtensionsOverlayManager struct {
	app       *BrowserApp
	manager   *webext.Manager
	panel     gtk.Widgetter
	iconRow   gtk.Widgetter
	isVisible bool

	// currentOverlay tracks which overlay the panel is currently attached to
	// so we can properly remove it when hiding or moving to a different pane
	currentOverlay *gtk.Overlay

	buttonToExtension map[uint64]*webext.Extension
	nextButtonID      uint64
}

// NewExtensionsOverlayManager builds a new overlay manager.
func NewExtensionsOverlayManager(app *BrowserApp, manager *webext.Manager) *ExtensionsOverlayManager {
	if app == nil || manager == nil {
		return nil
	}

	return &ExtensionsOverlayManager{
		app:               app,
		manager:           manager,
		buttonToExtension: make(map[uint64]*webext.Extension),
	}
}

// Initialize creates the overlay panel widget.
// The panel will be dynamically attached to the active pane's overlay when shown.
func (m *ExtensionsOverlayManager) Initialize(root gtk.Widgetter) error {
	if m == nil || m.manager == nil {
		return fmt.Errorf("extensions overlay manager missing dependencies")
	}

	// We no longer attach to the root overlay - instead we'll attach to the active pane
	// The root parameter is kept for API compatibility

	panel := gtk.NewBox(gtk.OrientationVertical, 8)
	if panel == nil {
		return fmt.Errorf("failed to create extensions overlay panel")
	}
	panel.AddCSSClass("extensions-overlay-panel")
	panel.SetHAlign(gtk.AlignEnd)
	panel.SetVAlign(gtk.AlignStart)
	panel.SetMarginTop(20)
	panel.SetMarginEnd(20)
	panel.SetHExpand(false)
	panel.SetVExpand(false)

	iconRow := gtk.NewBox(gtk.OrientationHorizontal, 6)
	if iconRow == nil {
		return fmt.Errorf("failed to create extensions icon row")
	}
	iconRow.SetHExpand(false)
	iconRow.SetVExpand(false)
	panel.Append(iconRow)

	m.panel = panel
	m.iconRow = iconRow
	m.refreshIcons()
	webkit.WidgetSetVisible(panel, false)
	// Don't attach to root - will be attached to active pane when shown

	logging.Info("[extensions] Overlay panel initialized (not attached yet)")
	return nil
}

// Toggle shows or hides the overlay, refreshing icons when showing.
// When showing, attaches the panel to the active pane's overlay.
func (m *ExtensionsOverlayManager) Toggle() {
	if m == nil || m.panel == nil {
		return
	}

	m.isVisible = !m.isVisible
	if m.isVisible {
		m.attachToActivePane()
		m.refreshIcons()
		webkit.WidgetSetVisible(m.panel, true)
	} else {
		webkit.WidgetSetVisible(m.panel, false)
		m.detachFromCurrentOverlay()
	}
}

// attachToActivePane attaches the panel to the active pane's overlay.
func (m *ExtensionsOverlayManager) attachToActivePane() {
	if m == nil || m.app == nil || m.panel == nil {
		return
	}

	// First, detach from any current overlay
	m.detachFromCurrentOverlay()

	// Get the active pane's node from the workspace
	ws := m.app.workspace
	if ws == nil {
		logging.Warn("[extensions] No workspace available for overlay attachment")
		return
	}

	activeNode := ws.GetActiveNode()
	if activeNode == nil || !activeNode.isLeaf {
		logging.Warn("[extensions] No active leaf pane for overlay attachment")
		return
	}

	// The container for leaf nodes is a *gtk.Overlay (from wrapPaneInOverlay)
	overlay, ok := activeNode.container.(*gtk.Overlay)
	if !ok || overlay == nil {
		logging.Warn("[extensions] Active pane container is not an overlay")
		return
	}

	// Attach our panel to this overlay
	overlay.AddOverlay(m.panel)
	m.currentOverlay = overlay
	logging.Info("[extensions] Attached overlay to active pane")
}

// detachFromCurrentOverlay removes the panel from its current overlay parent.
func (m *ExtensionsOverlayManager) detachFromCurrentOverlay() {
	if m == nil || m.panel == nil || m.currentOverlay == nil {
		return
	}

	// Safety check: only try to remove if the overlay widget is still valid
	// (the pane might have been destroyed)
	defer func() {
		m.currentOverlay = nil
	}()

	// Remove from current overlay
	m.currentOverlay.RemoveOverlay(m.panel)
	logging.Debug("[extensions] Detached overlay from pane")
}

// OnActivePaneChanged should be called when the active pane changes.
// This auto-hides the overlay to prevent it from appearing on the wrong pane.
func (m *ExtensionsOverlayManager) OnActivePaneChanged() {
	if m == nil || !m.isVisible {
		return
	}

	// Auto-hide when pane changes - user can re-open on the new pane
	m.Hide()
	logging.Debug("[extensions] Auto-hidden overlay due to pane change")
}

// OnPaneClosing should be called when a pane is about to be closed.
// This ensures we detach before the overlay's parent is destroyed.
func (m *ExtensionsOverlayManager) OnPaneClosing(node *paneNode) {
	if m == nil || m.currentOverlay == nil || node == nil {
		return
	}

	// Check if our overlay is attached to the closing pane
	if overlay, ok := node.container.(*gtk.Overlay); ok && overlay == m.currentOverlay {
		m.Hide()
		logging.Debug("[extensions] Auto-hidden overlay due to pane closing")
	}
}

// Hide hides the overlay without toggling state.
func (m *ExtensionsOverlayManager) Hide() {
	if m == nil || m.panel == nil {
		return
	}
	m.isVisible = false
	webkit.WidgetSetVisible(m.panel, false)
	m.detachFromCurrentOverlay()
}

// refreshIcons rebuilds the icon row based on currently loaded extensions.
func (m *ExtensionsOverlayManager) refreshIcons() {
	if m == nil || m.manager == nil || m.iconRow == nil {
		return
	}

	rowBox, ok := m.iconRow.(*gtk.Box)
	if !ok || rowBox == nil {
		return
	}

	// Clear existing children
	for child := rowBox.FirstChild(); child != nil; child = rowBox.FirstChild() {
		rowBox.Remove(child)
	}
	m.buttonToExtension = make(map[uint64]*webext.Extension)

	exts := m.manager.ListExtensions()
	if len(exts) == 0 {
		emptyLabel := gtk.NewLabel("No extensions")
		if emptyLabel != nil {
			rowBox.Append(emptyLabel)
			webkit.WidgetSetVisible(emptyLabel, true)
		}
		return
	}

	for _, ext := range exts {
		m.addExtensionButton(rowBox, ext)
	}
}

// addExtensionButton creates a clickable icon for an extension.
func (m *ExtensionsOverlayManager) addExtensionButton(container *gtk.Box, ext *webext.Extension) {
	if container == nil || ext == nil {
		return
	}

	tooltip := ext.ID
	if ext.Manifest != nil && ext.Manifest.Name != "" {
		tooltip = ext.Manifest.Name
	}

	button := gtk.NewButton()
	if button == nil {
		return
	}
	button.SetCanFocus(false)
	button.SetFocusOnClick(false)
	button.AddCSSClass("extension-icon")
	button.SetTooltipText(tooltip)

	if !m.manager.IsEnabled(ext.ID) {
		button.AddCSSClass("extension-icon-disabled")
	}

	iconWidget := m.buildIconWidget(ext)
	if iconWidget != nil {
		button.SetChild(iconWidget)
		webkit.WidgetSetVisible(iconWidget, true)
	}

	// Map ID to extension for click handling (same pattern as tabs/stacked panes)
	buttonID := atomic.AddUint64(&m.nextButtonID, 1)
	m.buttonToExtension[buttonID] = ext

	button.ConnectClicked(func() {
		m.handleIconClick(buttonID)
	})

	// Right-click gesture for context menu
	contextGesture := gtk.NewGestureClick()
	contextGesture.SetButton(3)
	contextGesture.ConnectPressed(func(_ int, x, y float64) {
		m.showContextMenu(button, ext, x, y)
	})
	button.AddController(contextGesture)

	container.Append(button)
	webkit.WidgetSetVisible(button, true)
}

// buildIconWidget loads the extension icon or falls back to a placeholder label.
func (m *ExtensionsOverlayManager) buildIconWidget(ext *webext.Extension) gtk.Widgetter {
	if ext == nil || ext.Manifest == nil {
		return gtk.NewLabel("🧩")
	}

	iconPath := m.resolveIconPath(ext)
	if iconPath != "" {
		texture, err := gdk.NewTextureFromFilename(iconPath)
		if err != nil {
			logging.Warn(fmt.Sprintf("[extensions] Failed to load icon for %s: %v", ext.ID, err))
		} else if texture != nil {
			img := gtk.NewImageFromPaintable(texture)
			if img != nil {
				img.SetPixelSize(32)
				return img
			}
		}
	}

	// Fallback to emoji placeholder
	label := gtk.NewLabel("🧩")
	return label
}

// resolveIconPath picks the best icon from the manifest.
func (m *ExtensionsOverlayManager) resolveIconPath(ext *webext.Extension) string {
	if ext == nil || ext.Manifest == nil {
		return ""
	}

	iconSizes := []string{"48", "32", "64", "128"}
	for _, size := range iconSizes {
		if rel, ok := ext.Manifest.Icons[size]; ok && rel != "" {
			return filepath.Join(ext.Path, rel)
		}
	}
	return ""
}

// handleIconClick opens the extension popup using the popup manager.
func (m *ExtensionsOverlayManager) handleIconClick(buttonID uint64) {
	ext, ok := m.buttonToExtension[buttonID]
	if !ok || ext == nil {
		logging.Warn(fmt.Sprintf("[extensions] Unknown button ID: %d", buttonID))
		return
	}

	url := m.getExtensionEntryURL(ext)
	if url == "" {
		logging.Warn(fmt.Sprintf("[extensions] No popup or options page for %s", ext.ID))
		return
	}

	// Use the popup manager to open extension popups
	if m.app.popupManager != nil {
		if err := m.app.popupManager.OpenPopup(ext.ID, url); err != nil {
			logging.Warn(fmt.Sprintf("[extensions] Failed to open popup for %s: %v", ext.ID, err))
			return
		}
		logging.Info(fmt.Sprintf("[extensions] Opened popup for %s: %s", ext.ID, url))
	} else {
		logging.Warn("[extensions] Popup manager not available")
	}

	m.Hide()
}

// getExtensionEntryURL chooses popup -> options -> homepage.
func (m *ExtensionsOverlayManager) getExtensionEntryURL(ext *webext.Extension) string {
	if ext == nil || ext.Manifest == nil {
		return ""
	}

	if ext.Manifest.BrowserAction != nil && ext.Manifest.BrowserAction.DefaultPopup != "" {
		return m.extensionURL(ext.ID, ext.Manifest.BrowserAction.DefaultPopup)
	}

	if ext.Manifest.Options != nil && ext.Manifest.Options.Page != "" {
		return m.extensionURL(ext.ID, ext.Manifest.Options.Page)
	}

	if ext.Manifest.Homepage != "" {
		return ext.Manifest.Homepage
	}

	return ""
}

func (m *ExtensionsOverlayManager) extensionURL(id, path string) string {
	trimmed := strings.TrimPrefix(path, "/")
	return fmt.Sprintf("dumb-extension://%s/%s", id, trimmed)
}

func (m *ExtensionsOverlayManager) activeWorkspace() *WorkspaceManager {
	if m == nil || m.app == nil {
		return nil
	}
	return m.app.workspace
}

// buildExtensionPane creates a pane for an extension popup, reusing the background WebView's
// process via the related-view property when available (similar to Epiphany's approach).
func (m *ExtensionsOverlayManager) buildExtensionPane(ext *webext.Extension) (*BrowserPane, error) {
	if m == nil || m.app == nil || ext == nil {
		return nil, fmt.Errorf("missing app or extension")
	}

	cfg, err := m.app.buildWebkitConfig()
	if err != nil {
		return nil, err
	}
	cfg.CreateWindow = false
	cfg.IsExtensionWebView = true // Critical: skip UserContentManager injection for extension popups

	var view *webkit.WebView

	// TODO: This should use the PopupManager once Phase 3 is implemented
	// For now, create an unrelated view (no parent background WebView exists anymore)
	// Extension popups will be properly handled by the PopupManager which will
	// connect them to the background context for API access

	// Use the extension's CSP from manifest or the default if not specified
	// (matches Firefox/Epiphany behavior - "script-src 'self'; object-src 'self';")
	csp := ext.Manifest.GetContentSecurityPolicy()

	// Build CORS allowlist from extension permissions
	corsAllowlist := buildCORSAllowlist(ext)

	bareView, factoryErr := webkit.NewExtensionWebView(&webkit.ExtensionViewConfig{
		Type:          webkit.ExtensionViewPopup,
		CSP:           csp,
		ParentView:    nil, // No parent view (background is now Goja context, not WebView)
		ExtensionID:   ext.ID,
		CORSAllowlist: corsAllowlist,
	})

	if factoryErr == nil && bareView != nil {
		wrapped := webkit.WrapBareWebView(bareView)
		if wrapped != nil {
			if initErr := wrapped.InitializeFromBare(cfg); initErr == nil {
				if len(corsAllowlist) > 0 {
					webkit.SetCORSAllowlist(wrapped.GetWebView(), corsAllowlist)
				}
				view = wrapped
				logging.Info(fmt.Sprintf("[extensions] Created extension popup WebView for %s", ext.ID))
			} else {
				logging.Warn(fmt.Sprintf("[extensions] Failed to init popup view for %s: %v", ext.ID, initErr))
			}
		}
	} else {
		logging.Warn(fmt.Sprintf("[extensions] Failed to create popup WebView for %s: %v", ext.ID, factoryErr))
	}

	// Fallback to independent WebView if related-view creation failed.
	if view == nil {
		view, err = webkit.NewWebView(cfg)
		if err != nil {
			return nil, err
		}
		logging.Warn(fmt.Sprintf("[extensions] Created fallback non-extension WebView for %s", ext.ID))
	}

	// Register extension message handler for WebProcess communication
	m.app.registerExtensionMessageHandler(view)

	// Register navigation policy to block navigations outside extension scope
	// (following Epiphany's decide_policy_cb pattern)
	if view != nil {
		view.GetWebView().ConnectDecidePolicy(func(decision webkitv6.PolicyDecisioner, decisionType webkitv6.PolicyDecisionType) bool {
			if decisionType != webkitv6.PolicyDecisionTypeNavigationAction {
				return false
			}

			// Use the base policy decision to access common methods
			baseDecision := webkitv6.BasePolicyDecision(decision)
			if baseDecision == nil {
				return false
			}

			// Type assert to NavigationPolicyDecision
			navDecision, ok := decision.(*webkitv6.NavigationPolicyDecision)
			if !ok {
				return false
			}

			navAction := navDecision.NavigationAction()
			if navAction == nil {
				return false
			}

			request := navAction.Request()
			if request == nil {
				return false
			}

			uri := request.URI()
			expectedPrefix := fmt.Sprintf("dumb-extension://%s/", ext.ID)

			if !strings.HasPrefix(uri, expectedPrefix) {
				logging.Info(fmt.Sprintf("[extensions] Blocking popup navigation from %s to %s (out of extension scope)", ext.ID, uri))
				baseDecision.Ignore()
				return true
			}

			return false
		})
	}

	return m.app.createPaneForView(view)
}

// showContextMenu displays a small popover with extension actions.
func (m *ExtensionsOverlayManager) showContextMenu(anchor *gtk.Button, ext *webext.Extension, x, y float64) {
	if anchor == nil || ext == nil {
		return
	}

	popover := gtk.NewPopover()
	if popover == nil {
		return
	}
	popover.SetHasArrow(true)
	popover.SetAutohide(true)
	popover.SetParent(anchor)

	box := gtk.NewBox(gtk.OrientationVertical, 4)
	if box == nil {
		return
	}

	enableBtn := gtk.NewButtonWithLabel("Enable")
	disableBtn := gtk.NewButtonWithLabel("Disable")
	optionsBtn := gtk.NewButtonWithLabel("Open Options")
	removeBtn := gtk.NewButtonWithLabel("Remove")

	box.Append(enableBtn)
	box.Append(disableBtn)
	box.Append(optionsBtn)
	box.Append(removeBtn)

	if m.manager.IsEnabled(ext.ID) {
		enableBtn.SetSensitive(false)
	} else {
		disableBtn.SetSensitive(false)
	}

	enableBtn.ConnectClicked(func() {
		if err := m.manager.Enable(ext.ID); err != nil {
			logging.Warn(fmt.Sprintf("[extensions] Failed to enable %s: %v", ext.ID, err))
		}
		m.refreshIcons()
		popover.Popdown()
	})

	disableBtn.ConnectClicked(func() {
		if err := m.manager.Disable(ext.ID); err != nil {
			logging.Warn(fmt.Sprintf("[extensions] Failed to disable %s: %v", ext.ID, err))
		}
		m.refreshIcons()
		popover.Popdown()
	})

	optionsBtn.ConnectClicked(func() {
		if ext.Manifest != nil && ext.Manifest.Options != nil && ext.Manifest.Options.Page != "" {
			url := m.extensionURL(ext.ID, ext.Manifest.Options.Page)
			m.openURLInNewPane(url)
		}
		popover.Popdown()
	})

	removeBtn.ConnectClicked(func() {
		if err := m.manager.Remove(ext.ID); err != nil {
			logging.Warn(fmt.Sprintf("[extensions] Failed to remove %s: %v", ext.ID, err))
		}
		m.refreshIcons()
		popover.Popdown()
	})

	popover.SetChild(box)
	popover.Popup()
}

// openURLInNewPane opens a URL in a fresh split pane.
func (m *ExtensionsOverlayManager) openURLInNewPane(url string) {
	if url == "" {
		return
	}
	ws := m.activeWorkspace()
	if ws == nil {
		return
	}

	target := ws.GetActiveNode()
	if target == nil && ws.root != nil {
		target = ws.root
	}
	if target == nil {
		return
	}

	newNode, err := ws.SplitPane(target, DirectionRight)
	if err != nil {
		logging.Warn(fmt.Sprintf("[extensions] Failed to split for URL %s: %v", url, err))
		return
	}

	if newNode != nil && newNode.pane != nil && newNode.pane.WebView() != nil {
		_ = newNode.pane.WebView().LoadURL(url)
		ws.SetActivePane(newNode, SourceProgrammatic)
	}
}
