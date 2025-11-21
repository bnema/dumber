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

// Initialize attaches the overlay panel to the root overlay.
func (m *ExtensionsOverlayManager) Initialize(root gtk.Widgetter) error {
	if m == nil || m.manager == nil {
		return fmt.Errorf("extensions overlay manager missing dependencies")
	}

	rootOverlay, ok := root.(*gtk.Overlay)
	if !ok || rootOverlay == nil {
		return fmt.Errorf("root overlay missing or invalid")
	}

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
	rootOverlay.AddOverlay(panel)

	logging.Info("[extensions] Overlay panel initialized")
	return nil
}

// Toggle shows or hides the overlay, refreshing icons when showing.
func (m *ExtensionsOverlayManager) Toggle() {
	if m == nil || m.panel == nil {
		return
	}

	m.isVisible = !m.isVisible
	if m.isVisible {
		m.refreshIcons()
		webkit.WidgetSetVisible(m.panel, true)
	} else {
		webkit.WidgetSetVisible(m.panel, false)
	}
}

// Hide hides the overlay without toggling state.
func (m *ExtensionsOverlayManager) Hide() {
	if m == nil || m.panel == nil {
		return
	}
	m.isVisible = false
	webkit.WidgetSetVisible(m.panel, false)
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

// handleIconClick opens the extension popup/options page in a new pane.
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

	ws := m.activeWorkspace()
	if ws == nil {
		logging.Warn("[extensions] No active workspace available for popup")
		return
	}

	pane, err := m.buildExtensionPane(ext)
	if err != nil {
		logging.Warn(fmt.Sprintf("[extensions] Failed to build pane for %s: %v", ext.ID, err))
		return
	}

	target := ws.GetActiveNode()
	if target == nil && ws.root != nil { // fallback to root if no active node yet
		target = ws.root
	}
	if target == nil {
		logging.Warn("[extensions] No target node for split")
		return
	}

	newNode, err := ws.SplitPaneWithPane(target, DirectionRight, pane)
	if err != nil {
		logging.Warn(fmt.Sprintf("[extensions] Failed to split pane for %s: %v", ext.ID, err))
		return
	}

	if newNode != nil && newNode.pane != nil && newNode.pane.WebView() != nil {
		if err := newNode.pane.WebView().LoadURL(url); err != nil {
			logging.Warn(fmt.Sprintf("[extensions] Failed to load popup for %s: %v", ext.ID, err))
		}
		ws.SetActivePane(newNode, SourceProgrammatic)
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

	var view *webkit.WebView

	// Prefer a related view to the extension's background page so storage/session are shared
	// (following Epiphany's pattern of using related-view for popups)
	if bg := m.app.getBackgroundView(ext.ID); bg != nil {
		// Use ManifestV2 default CSP if extension doesn't specify one
		csp := "script-src 'self'; object-src 'self';"
		if ext.Manifest != nil && ext.Manifest.ContentSecurityPolicy != "" {
			csp = ext.Manifest.ContentSecurityPolicy
		}

		bareView, factoryErr := webkit.NewExtensionWebView(&webkit.ExtensionViewConfig{
			Type:        webkit.ExtensionViewPopup,
			CSP:         csp,
			ParentView:  bg.GtkWebView(),
			ExtensionID: ext.ID,
		})

		if factoryErr == nil && bareView != nil {
			wrapped := webkit.WrapBareWebView(bareView)
			if wrapped != nil {
				if initErr := wrapped.InitializeFromBare(cfg); initErr == nil {
					view = wrapped
					logging.Info(fmt.Sprintf("[extensions] Created extension popup WebView for %s", ext.ID))
				} else {
					logging.Warn(fmt.Sprintf("[extensions] Failed to init popup view for %s: %v", ext.ID, initErr))
				}
			}
		} else {
			logging.Warn(fmt.Sprintf("[extensions] Failed to create popup WebView for %s: %v", ext.ID, factoryErr))
		}
	}

	// Fallback to independent WebView if related-view creation failed.
	if view == nil {
		view, err = webkit.NewWebView(cfg)
		if err != nil {
			return nil, err
		}
		logging.Warn(fmt.Sprintf("[extensions] Created fallback non-extension WebView for %s", ext.ID))
	}

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
