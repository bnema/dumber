package browser

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/bnema/dumber/internal/app/messaging"
	"github.com/bnema/dumber/pkg/webkit"
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
		{"ctrl+w", h.handleClosePane, "Close current pane"},
		{"F12", h.handleDevTools, "Developer tools"},
		// Zoom shortcuts - global level for proper active pane targeting
		{"ctrl+plus", h.handleZoomIn, "Zoom in"},
		{"ctrl+equal", h.handleZoomIn, "Zoom in (=)"},
		{"ctrl+minus", h.handleZoomOut, "Zoom out"},
		{"ctrl+0", h.handleZoomReset, "Zoom reset"},
		// Workspace navigation shortcuts - global level for proper active pane targeting
		{"alt+ArrowLeft", func() { h.handleWorkspaceNavigation("left") }, "Navigate left pane"},
		{"alt+ArrowRight", func() { h.handleWorkspaceNavigation("right") }, "Navigate right pane"},
		{"alt+ArrowUp", func() { h.handleWorkspaceNavigation("up") }, "Navigate up pane"},
		{"alt+ArrowDown", func() { h.handleWorkspaceNavigation("down") }, "Navigate down pane"},
	}

	for _, shortcut := range shortcuts {
		h.shortcuts.RegisterShortcut(shortcut.key, shortcut.handler)
		log.Printf("[window-shortcuts] Registered global shortcut: %s (%s)",
			shortcut.key, shortcut.desc)
	}

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
				document.dispatchEvent(new CustomEvent('dumber:showToast', {
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

	// Show zoom toast notification
	zoomPercent := int(newZoom * 100)
	toastScript := fmt.Sprintf(`
		(function() {
			try {
				if (typeof window.__dumber_showZoomToast === 'function') {
					window.__dumber_showZoomToast(%f);
				} else {
					// Fallback: dispatch toast event directly
					document.dispatchEvent(new CustomEvent('dumber:showToast', {
						detail: { message: 'Zoom: %d%%', duration: 1500, type: 'info' }
					}));
				}
			} catch (e) {
				console.error('[zoom] Failed to show toast:', e);
			}
		})();
	`, newZoom, zoomPercent)

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
			log.Printf("[window-shortcuts] WebView %s: IsActive=%t", webView.ID(), isActive)
			if isActive {
				activeWebView = webView
				log.Printf("[window-shortcuts] Found active WebView: %s", webView.ID())
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

// Cleanup releases resources
func (h *WindowShortcutHandler) Cleanup() {
	// WindowShortcuts doesn't require explicit cleanup
	h.shortcuts = nil
}

// Error definitions
var (
	ErrFailedToInitialize = fmt.Errorf("failed to initialize window shortcuts")
)
