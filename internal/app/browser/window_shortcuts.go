package browser

import (
	"fmt"
	"log"
	"sync"
	"time"

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
	// Initialize GTK4 global shortcuts
	h.shortcuts = h.window.InitializeGlobalShortcuts()
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
		{"F12", h.handleDevTools, "Developer tools"},
		// Zoom shortcuts - global level for proper active pane targeting
		{"ctrl+plus", h.handleZoomIn, "Zoom in"},
		{"ctrl+equal", h.handleZoomIn, "Zoom in (=)"},
		{"ctrl+minus", h.handleZoomOut, "Zoom out"},
		{"ctrl+0", h.handleZoomReset, "Zoom reset"},
	}

	for _, shortcut := range shortcuts {
		if err := h.shortcuts.RegisterGlobalShortcut(shortcut.key, shortcut.handler); err != nil {
			log.Printf("[window-shortcuts] Failed to register %s (%s): %v",
				shortcut.key, shortcut.desc, err)
			return err
		}
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
	currentZoom, err := activeWebView.GetZoom()
	if err != nil {
		log.Printf("[window-shortcuts] Failed to get current zoom: %v", err)
		return
	}

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

func (h *WindowShortcutHandler) getPaneId(pane *BrowserPane) string {
	if pane == nil {
		return "unknown"
	}
	return pane.ID()
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
		"paneId":    h.getPaneId(h.app.activePane),
		"timestamp": time.Now().UnixMilli(),
		"source":    "window-global",
	}); err != nil {
		log.Printf("[window-shortcuts] Failed to dispatch %s toggle: %v", featureName, err)
	}
}

// Cleanup releases resources
func (h *WindowShortcutHandler) Cleanup() {
	if h.shortcuts != nil {
		h.shortcuts.Cleanup()
		h.shortcuts = nil
	}
}

// Error definitions
var (
	ErrFailedToInitialize = fmt.Errorf("failed to initialize window shortcuts")
)
