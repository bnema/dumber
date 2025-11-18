package browser

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/bnema/dumber/internal/app/messaging"
	"github.com/bnema/dumber/pkg/webkit"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// WindowShortcutHandler manages global shortcuts at the window level
type WindowShortcutHandler struct {
	window    *webkit.Window
	app       *BrowserApp
	shortcuts *webkit.WindowShortcuts
	mu        sync.Mutex

	// Debounce tracking
	lastOmniboxToggle time.Time
	lastFindToggle    time.Time
	lastCopyURL       time.Time
	lastDevTools      time.Time
	lastPrint         time.Time
}

// NewWindowShortcutHandler creates a new window-level shortcut handler
func NewWindowShortcutHandler(window *webkit.Window, app *BrowserApp) *WindowShortcutHandler {
	h := &WindowShortcutHandler{
		window: window,
		app:    app,
	}

	if err := h.initialize(); err != nil {
		log.Printf("[window-shortcuts] Failed to initialize: %v", err)
		return nil
	}

	return h
}

func (h *WindowShortcutHandler) initialize() error {
	// Create WindowShortcuts manager
	h.shortcuts = webkit.NewWindowShortcuts(h.window)
	if h.shortcuts == nil {
		return ErrFailedToInitialize
	}

	// Register hardware keycode-based shortcuts (for AZERTY/international keyboards)
	if err := h.registerKeycodeShortcuts(); err != nil {
		log.Printf("[window-shortcuts] Failed to register keycode shortcuts: %v", err)
	}

	return h.registerGlobalShortcuts()
}

func (h *WindowShortcutHandler) registerGlobalShortcuts() error {
	shortcuts := []struct {
		key     string
		handler func()
		desc    string
	}{
		{"ctrl+l", h.handleOmniboxToggle, "Omnibox toggle"},
		{"ctrl+f", h.handleFindToggle, "Find in page"},
		{"ctrl+shift+c", h.handleCopyURL, "Copy URL"},
		{"ctrl+shift+p", h.handlePrint, "Print page"},
		{"F12", h.handleDevTools, "Developer tools"},
		// Pane management: Ctrl+P handled by WebView keyboard bridge (not window shortcuts)
		// Tab management
		{"ctrl+t", h.handleTabMode, "Enter tab mode"},
		// Global tab navigation (Ctrl+Tab) is handled via EventControllerKey in registerKeycodeShortcuts
		// This prevents GTK's default focus cycling behavior
		// NOTE: Tab mode action keys (n, c, x, l, h, r, Tab, Escape, Enter) are NOT registered
		// as global shortcuts. They are handled by the WebView keyboard bridge which checks
		// if tab mode is active before processing them. This prevents them from consuming
		// keys when tab mode is not active (allowing normal typing in web pages).
		// Direct tab switching is now handled via hardware keycodes (see registerKeycodeShortcuts)
		// This supports all keyboard layouts (QWERTY, AZERTY, etc.)
		// Page reload shortcuts
		{"ctrl+r", h.handleReload, "Reload page"},
		{"ctrl+shift+r", h.handleHardReload, "Hard reload (bypass cache)"},
		{"F5", h.handleReload, "Reload page"},
		{"ctrl+F5", h.handleHardReload, "Hard reload (bypass cache)"},
		// Zoom shortcuts - global level for proper active pane targeting
		{"ctrl+plus", h.handleZoomIn, "Zoom in"},
		{"ctrl+equal", h.handleZoomIn, "Zoom in (=)"},
		{"ctrl+minus", h.handleZoomOut, "Zoom out"},
		{"ctrl+0", h.handleZoomReset, "Zoom reset"},
		// Workspace navigation shortcuts - global level for proper active pane targeting
		{"alt+ArrowLeft", func() { h.handleWorkspaceNavigation(DirectionLeft) }, "Navigate left pane"},
		{"alt+ArrowRight", func() { h.handleWorkspaceNavigation(DirectionRight) }, "Navigate right pane"},
		{"alt+ArrowUp", func() { h.handleWorkspaceNavigation(DirectionUp) }, "Navigate up pane"},
		{"alt+ArrowDown", func() { h.handleWorkspaceNavigation(DirectionDown) }, "Navigate down pane"},
		// Vim-style workspace navigation
		{"alt+h", func() { h.handleWorkspaceNavigation(DirectionLeft) }, "Navigate left pane (vim)"},
		{"alt+l", func() { h.handleWorkspaceNavigation(DirectionRight) }, "Navigate right pane (vim)"},
		{"alt+k", func() { h.handleWorkspaceNavigation(DirectionUp) }, "Navigate up pane (vim)"},
		{"alt+j", func() { h.handleWorkspaceNavigation(DirectionDown) }, "Navigate down pane (vim)"},
	}

	for _, shortcut := range shortcuts {
		h.shortcuts.RegisterShortcut(shortcut.key, shortcut.handler)
		log.Printf("[window-shortcuts] Registered global shortcut: %s (%s)",
			shortcut.key, shortcut.desc)
	}

	// NOTE: Ctrl+P is NOT registered here - it's handled by WebView's keyboard bridge
	// (RegisterPaneModeHandler in workspace_manager.go) which calls app.workspace.EnterPaneMode()

	return nil
}

// registerKeycodeShortcuts registers keyboard shortcuts based on hardware keycodes
// This allows Alt+number shortcuts to work on AZERTY and other keyboard layouts
func (h *WindowShortcutHandler) registerKeycodeShortcuts() error {
	if h.window == nil {
		return fmt.Errorf("window is nil")
	}

	gtkWindow := h.window.AsWindow()
	if gtkWindow == nil {
		return fmt.Errorf("gtk window is nil")
	}

	// Create event controller for keyboard events
	keyController := gtk.NewEventControllerKey()
	keyController.SetPropagationPhase(gtk.PhaseCapture)

	// GDK key constants
	const (
		gdkKeyTab    = 0xff09
		gdkKeyEscape = 0xff1b
		gdkKeyReturn = 0xff0d
		gdkKeyN      = 0x006e
		gdkKeyC      = 0x0063
		gdkKeyX      = 0x0078
		gdkKeyL      = 0x006c
		gdkKeyH      = 0x0068
		gdkKeyR      = 0x0072
	)

	// Map hardware keycodes to tab indices
	// These keycodes are the same across all keyboard layouts
	keycodeToTab := map[uint]int{
		10: 0, // 1/& key
		11: 1, // 2/é key
		12: 2, // 3/" key
		13: 3, // 4/' key
		14: 4, // 5/( key
		15: 5, // 6/- key
		16: 6, // 7/è key
		17: 7, // 8/_ key
		18: 8, // 9/ç key
		19: 9, // 0/à key
	}

	keyController.ConnectKeyPressed(func(keyval, keycode uint, state gdk.ModifierType) bool {
		// IMPORTANT: Let WebView's keyboard bridge handle pane mode action keys first
		// During pane mode, single-letter keys (h,j,k,l,x,s,v, etc.) must reach the WebView
		// So we only intercept specific shortcuts here and let everything else through

		// Handle Ctrl+Tab for next tab
		if keyval == gdkKeyTab && state.Has(gdk.ControlMask) && !state.Has(gdk.ShiftMask) {
			log.Printf("[window-shortcuts] Ctrl+Tab -> next tab")
			h.handleNextTab()
			return true // Consume event to prevent focus cycling
		}

		// Handle Ctrl+Shift+Tab for previous tab
		if keyval == gdkKeyTab && state.Has(gdk.ControlMask) && state.Has(gdk.ShiftMask) {
			log.Printf("[window-shortcuts] Ctrl+Shift+Tab -> previous tab")
			h.handlePrevTab()
			return true // Consume event to prevent focus cycling
		}

		// Check for Alt+number (but not with Ctrl or Shift)
		if state.Has(gdk.AltMask) && !state.Has(gdk.ControlMask) && !state.Has(gdk.ShiftMask) {
			// Check if this is a number key
			if tabIndex, ok := keycodeToTab[keycode]; ok {
				log.Printf("[window-shortcuts] Hardware keycode shortcut: Alt+keycode(%d) -> tab %d", keycode, tabIndex)
				h.handleDirectTabSwitch(tabIndex)
				return true // Handled
			}
		}

		// Handle tab mode action keys (only when tab mode is active)
		// IMPORTANT: If rename is in progress, let ALL keys through to the Entry widget
		// These keys need to be checked here so they can propagate when tab mode is not active
		if h.isTabModeActive() {
			// Check if rename is in progress - if so, don't intercept ANY keys
			// This allows the Entry widget to receive all keyboard input
			if h.isRenameInProgress() {
				return false // Let all keys propagate to Entry widget
			}

			handled := false
			action := ""

			// Check for no modifiers (plain key press)
			if !state.Has(gdk.ControlMask) && !state.Has(gdk.AltMask) {
				switch keyval {
				case gdkKeyN, gdkKeyC:
					action = "new-tab"
					handled = true
				case gdkKeyX:
					action = "close-tab"
					handled = true
				case gdkKeyL:
					action = "next-tab"
					handled = true
				case gdkKeyH:
					action = "previous-tab"
					handled = true
				case gdkKeyR:
					action = "rename-tab"
					handled = true
				case gdkKeyTab:
					action = "next-tab"
					handled = true
				case gdkKeyReturn:
					action = "confirm"
					handled = true
				case gdkKeyEscape:
					action = "cancel"
					handled = true
				}
			}

			// Handle Shift+Tab for previous tab in tab mode
			if keyval == gdkKeyTab && state.Has(gdk.ShiftMask) && !state.Has(gdk.ControlMask) {
				action = "previous-tab"
				handled = true
			}

			if handled {
				log.Printf("[window-shortcuts] Tab mode action: %s", action)
				h.handleTabModeAction(action)
				return true // Consume event
			}
		}

		// Let all other keys propagate to WebView's keyboard bridge
		// This includes Ctrl+P, Ctrl+T, and all pane mode action keys
		return false
	})

	gtkWindow.AddController(keyController)
	log.Printf("[window-shortcuts] Registered capture phase shortcuts: Ctrl+Tab, Alt+number")

	return nil
}

func (h *WindowShortcutHandler) handleOmniboxToggle() {
	h.handleUIToggle(&h.lastOmniboxToggle, "omnibox", "omnibox-nav-toggle")
}

func (h *WindowShortcutHandler) handleFindToggle() {
	h.handleUIToggle(&h.lastFindToggle, "find", "omnibox-find-toggle")
}

func (h *WindowShortcutHandler) handleCopyURL() {
	h.mu.Lock()
	defer h.mu.Unlock()

	if time.Since(h.lastCopyURL) < 100*time.Millisecond {
		return
	}
	h.lastCopyURL = time.Now()

	if h.app.activePane == nil || h.app.activePane.webView == nil {
		log.Printf("[window-shortcuts] No active pane for copy URL")
		return
	}

	log.Printf("[window-shortcuts] Copy URL -> pane %p", h.app.activePane.webView)

	// Execute copy URL script directly on active pane
	script := `
		(async function() {
			const toast = (message, type) => {
				document.dispatchEvent(new CustomEvent('dumber:toast', {
					detail: { message, duration: 2000, type }
				}));
			};

			try {
				const url = window.location.href;
				await navigator.clipboard.writeText(url);
				console.log('🔗 Window shortcut copied URL:', url);
				toast('URL copied to clipboard!', 'success');
			} catch (error) {
				console.error('❌ Failed to copy URL:', error);
				toast('Failed to copy URL', 'error');
			}
		})();
	`

	if err := h.app.activePane.webView.InjectScript(script); err != nil {
		log.Printf("[window-shortcuts] Failed to inject copy URL script: %v", err)
	}
}

func (h *WindowShortcutHandler) handleDevTools() {
	h.mu.Lock()
	defer h.mu.Unlock()

	if time.Since(h.lastDevTools) < 200*time.Millisecond {
		return
	}
	h.lastDevTools = time.Now()

	if h.app.activePane == nil || h.app.activePane.webView == nil {
		log.Printf("[window-shortcuts] No active pane for devtools")
		return
	}

	log.Printf("[window-shortcuts] DevTools -> pane %p", h.app.activePane.webView)

	if err := h.app.activePane.webView.ShowDevTools(); err != nil {
		log.Printf("[window-shortcuts] Failed to show devtools: %v", err)
	}
}

func (h *WindowShortcutHandler) handlePrint() {
	h.mu.Lock()
	defer h.mu.Unlock()

	if time.Since(h.lastPrint) < 200*time.Millisecond {
		return
	}
	h.lastPrint = time.Now()

	if h.app.activePane == nil || h.app.activePane.webView == nil {
		log.Printf("[window-shortcuts] No active pane for print")
		return
	}

	log.Printf("[window-shortcuts] Print -> pane %p", h.app.activePane.webView)

	if err := h.app.activePane.webView.ShowPrintDialog(); err != nil {
		log.Printf("[window-shortcuts] Failed to show print dialog: %v", err)
	}
}

func (h *WindowShortcutHandler) handleReload() {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.app.activePane == nil || h.app.activePane.webView == nil {
		log.Printf("[window-shortcuts] No active pane for reload")
		return
	}

	log.Printf("[window-shortcuts] Reload page -> pane %p", h.app.activePane.webView)

	if err := h.app.activePane.webView.Reload(); err != nil {
		log.Printf("[window-shortcuts] Failed to reload page: %v", err)
	}
}

func (h *WindowShortcutHandler) handleHardReload() {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.app.activePane == nil || h.app.activePane.webView == nil {
		log.Printf("[window-shortcuts] No active pane for hard reload")
		return
	}

	log.Printf("[window-shortcuts] Hard reload (bypass cache) -> pane %p", h.app.activePane.webView)

	if err := h.app.activePane.webView.ReloadBypassCache(); err != nil {
		log.Printf("[window-shortcuts] Failed to hard reload page: %v", err)
	}
}

// handleZoomIn increases zoom level by 10%
func (h *WindowShortcutHandler) handleZoomIn() {
	h.handleZoom("in", 1.1)
}

// handleZoomOut decreases zoom level by 10%
func (h *WindowShortcutHandler) handleZoomOut() {
	h.handleZoom("out", 1.0/1.1)
}

// handleZoomReset resets zoom to 100%
func (h *WindowShortcutHandler) handleZoomReset() {
	h.handleZoom("reset", 1.0)
}

// handleZoom applies zoom changes to the active pane
func (h *WindowShortcutHandler) handleZoom(action string, multiplier float64) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.app.activePane == nil || h.app.activePane.webView == nil {
		log.Printf("[window-shortcuts] No active pane for zoom %s", action)
		return
	}

	activeWebView := h.app.activePane.webView
	log.Printf("[window-shortcuts] Zoom %s -> pane %p", action, activeWebView)

	// Get current zoom level
	currentZoom := activeWebView.GetZoom()

	// Calculate new zoom level
	var newZoom float64
	if action == "reset" {
		newZoom = 1.0
	} else {
		newZoom = currentZoom * multiplier
		// Apply zoom limits
		if newZoom < 0.25 {
			newZoom = 0.25
		}
		if newZoom > 5.0 {
			newZoom = 5.0
		}
	}

	log.Printf("[window-shortcuts] Zoom %s: %.2f -> %.2f", action, currentZoom, newZoom)

	// Apply zoom to active pane
	if err := activeWebView.SetZoom(newZoom); err != nil {
		log.Printf("[window-shortcuts] Failed to set zoom: %v", err)
		return
	}

	// Ensure GUI/toast component is loaded before showing toast
	h.ensureGUIInActivePane("toast")

	// Show zoom toast notification
	toastScript := fmt.Sprintf(`
		(function() {
			try {
				if (typeof window.__dumber_showZoomToast === 'function') {
					window.__dumber_showZoomToast(%f);
				}
			} catch (e) {
				console.error('[window-shortcuts] Failed to show zoom toast:', e);
			}
		})();
	`, newZoom)

	if err := activeWebView.InjectScript(toastScript); err != nil {
		log.Printf("[window-shortcuts] Failed to show zoom toast: %v", err)
	}
}

func (h *WindowShortcutHandler) ensureGUIInActivePane(component string) {
	if h.app.activePane == nil {
		return
	}

	pane := h.app.activePane
	if !pane.HasGUI() {
		log.Printf("[window-shortcuts] Injecting GUI into pane %s for %s", pane.ID(), component)
		if h.app.workspace != nil {
			h.app.workspace.ensureGUIInPane(pane)
		}
	}

	// Ensure specific component is available
	if !pane.HasGUIComponent(component) {
		log.Printf("[window-shortcuts] Ensuring %s component in pane %s", component, pane.ID())
		pane.SetGUIComponent(component, true)
	}
}

func (h *WindowShortcutHandler) getWebViewId(pane *BrowserPane) string {
	if pane == nil || pane.webView == nil {
		return "unknown"
	}
	return pane.webView.IDString()
}

func (h *WindowShortcutHandler) handleUIToggle(lastToggle *time.Time, featureName, action string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if time.Since(*lastToggle) < 50*time.Millisecond {
		log.Printf("[window-shortcuts] %s toggle debounced", featureName)
		return
	}
	*lastToggle = time.Now()

	if h.app.activePane == nil || h.app.activePane.webView == nil {
		log.Printf("[window-shortcuts] No active pane for %s", featureName)
		return
	}

	log.Printf("[window-shortcuts] %s toggle -> pane %p", featureName, h.app.activePane.webView)

	h.ensureGUIInActivePane("omnibox")

	if err := h.app.activePane.webView.DispatchCustomEvent("dumber:ui:shortcut", map[string]any{
		"action":    action,
		"webviewId": h.getWebViewId(h.app.activePane),
		"timestamp": time.Now().UnixMilli(),
		"source":    "window-global",
	}); err != nil {
		log.Printf("[window-shortcuts] Failed to dispatch %s toggle: %v", featureName, err)
	}
}

func (h *WindowShortcutHandler) handleWorkspaceNavigation(direction string) {
	if h.app == nil || h.app.workspace == nil {
		log.Printf("[window-shortcuts] No workspace for navigation")
		return
	}

	log.Printf("[window-shortcuts] Workspace navigation: %s", direction)

	if h.app.workspace.FocusNeighbor(direction) {
		log.Printf("[window-shortcuts] Workspace navigation %s successful", direction)
	} else {
		log.Printf("[window-shortcuts] Workspace navigation %s failed", direction)
	}
}

func (h *WindowShortcutHandler) handleClosePane() {
	if h.app == nil || h.app.workspace == nil {
		log.Printf("[window-shortcuts] No workspace for close pane")
		return
	}

	// Find the currently active WebView
	var activeWebView *webkit.WebView
	log.Printf("[window-shortcuts] Searching for active WebView among %d WebViews", len(h.app.workspace.viewToNode))
	for webView := range h.app.workspace.viewToNode {
		if webView != nil {
			isActive := webView.IsActive()
			log.Printf("[window-shortcuts] WebView %d: IsActive=%t", webView.ID(), isActive)
			if isActive {
				activeWebView = webView
				log.Printf("[window-shortcuts] Found active WebView: %d", webView.ID())
				break
			}
		}
	}

	if activeWebView == nil {
		log.Printf("[window-shortcuts] No active WebView found, using workspace closeCurrentPane")
		h.app.workspace.closeCurrentPane()
		return
	}

	// Check if the active WebView is a popup
	node := h.app.workspace.viewToNode[activeWebView]
	if node != nil && node.isPopup {
		log.Printf("[window-shortcuts] Closing popup via OnWorkspaceMessage")
		// Use the proper close-popup message for popups
		msg := messaging.Message{
			Event:     "close-popup",
			WebViewID: activeWebView.IDString(),
			Reason:    "user-ctrl-w",
		}
		h.app.workspace.OnWorkspaceMessage(activeWebView, msg)
	} else {
		log.Printf("[window-shortcuts] Closing regular pane via workspace closeCurrentPane")
		// Use regular close for non-popup panes
		h.app.workspace.closeCurrentPane()
	}
}

// handlePaneMode enters pane mode for modal pane management.
func (h *WindowShortcutHandler) handlePaneMode() {
	if h.app == nil || h.app.workspace == nil {
		log.Printf("[window-shortcuts] Cannot enter pane mode: workspace not available")
		return
	}

	log.Printf("[window-shortcuts] Entering pane mode")
	h.app.workspace.EnterPaneMode()
}

// handleTabMode enters tab mode for modal tab management.
func (h *WindowShortcutHandler) handleTabMode() {
	if h.app == nil || h.app.tabManager == nil {
		log.Printf("[window-shortcuts] Cannot enter tab mode: tab manager not available")
		return
	}

	log.Printf("[window-shortcuts] Entering tab mode")
	h.app.tabManager.EnterTabMode()
}

// handleNextTab switches to the next tab.
func (h *WindowShortcutHandler) handleNextTab() {
	if h.app == nil || h.app.tabManager == nil {
		log.Printf("[window-shortcuts] Cannot switch tab: tab manager not available")
		return
	}

	log.Printf("[window-shortcuts] Switching to next tab")
	if err := h.app.tabManager.NextTab(); err != nil {
		log.Printf("[window-shortcuts] Failed to switch to next tab: %v", err)
	}
}

// handlePrevTab switches to the previous tab.
func (h *WindowShortcutHandler) handlePrevTab() {
	if h.app == nil || h.app.tabManager == nil {
		log.Printf("[window-shortcuts] Cannot switch tab: tab manager not available")
		return
	}

	log.Printf("[window-shortcuts] Switching to previous tab")
	if err := h.app.tabManager.PreviousTab(); err != nil {
		log.Printf("[window-shortcuts] Failed to switch to previous tab: %v", err)
	}
}

// handleDirectTabSwitch switches to a specific tab by index (0-based).
func (h *WindowShortcutHandler) handleDirectTabSwitch(index int) {
	if h.app == nil || h.app.tabManager == nil {
		log.Printf("[window-shortcuts] Cannot switch tab: tab manager not available")
		return
	}

	log.Printf("[window-shortcuts] Direct tab switch to index %d", index)
	if err := h.app.tabManager.SwitchToTab(index); err != nil {
		log.Printf("[window-shortcuts] Failed to switch to tab %d: %v", index, err)
	}
}

// isTabModeActive checks if tab mode is currently active
func (h *WindowShortcutHandler) isTabModeActive() bool {
	if h.app == nil || h.app.tabManager == nil {
		return false
	}
	return h.app.tabManager.IsTabModeActive()
}

// isRenameInProgress checks if a tab rename is currently in progress
func (h *WindowShortcutHandler) isRenameInProgress() bool {
	if h.app == nil || h.app.tabManager == nil {
		return false
	}
	return h.app.tabManager.IsRenameInProgress()
}

// handleTabModeAction handles tab mode action keys (n, x, l, h, etc.)
// Only processes actions when tab mode is active
func (h *WindowShortcutHandler) handleTabModeAction(action string) {
	if h.app == nil || h.app.tabManager == nil {
		return
	}

	// Tab mode is active, handle the action
	log.Printf("[window-shortcuts] Tab mode action: %s", action)
	h.app.tabManager.HandleTabAction(action)
}

// Cleanup releases resources
func (h *WindowShortcutHandler) Cleanup() {
	// WindowShortcuts doesn't require explicit cleanup
	h.shortcuts = nil
}

// Error definitions
var (
	ErrFailedToInitialize = fmt.Errorf("failed to initialize window shortcuts")
)
