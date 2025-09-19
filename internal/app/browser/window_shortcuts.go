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
	h.mu.Lock()
	defer h.mu.Unlock()

	// Debounce: 50ms minimum between toggles
	if time.Since(h.lastOmniboxToggle) < 50*time.Millisecond {
		log.Printf("[window-shortcuts] Omnibox toggle debounced")
		return
	}
	h.lastOmniboxToggle = time.Now()

	if h.app.activePane == nil || h.app.activePane.webView == nil {
		log.Printf("[window-shortcuts] No active pane for omnibox")
		return
	}

	log.Printf("[window-shortcuts] Omnibox toggle -> pane %p", h.app.activePane.webView)

	// Ensure GUI is available in active pane
	h.ensureGUIInActivePane("omnibox")

	// Dispatch to active pane only
	if err := h.app.activePane.webView.DispatchCustomEvent("dumber:ui:shortcut", map[string]any{
		"action":    "omnibox-toggle",
		"paneId":    h.getPaneId(h.app.activePane),
		"timestamp": time.Now().UnixMilli(),
		"source":    "window-global",
	}); err != nil {
		log.Printf("[window-shortcuts] Failed to dispatch omnibox toggle: %v", err)
	}
}

func (h *WindowShortcutHandler) handleFindToggle() {
	h.mu.Lock()
	defer h.mu.Unlock()

	if time.Since(h.lastFindToggle) < 50*time.Millisecond {
		log.Printf("[window-shortcuts] Find toggle debounced")
		return
	}
	h.lastFindToggle = time.Now()

	if h.app.activePane == nil || h.app.activePane.webView == nil {
		log.Printf("[window-shortcuts] No active pane for find")
		return
	}

	log.Printf("[window-shortcuts] Find toggle -> pane %p", h.app.activePane.webView)

	h.ensureGUIInActivePane("omnibox")

	if err := h.app.activePane.webView.DispatchCustomEvent("dumber:ui:shortcut", map[string]any{
		"action":    "omnibox-find",
		"paneId":    h.getPaneId(h.app.activePane),
		"timestamp": time.Now().UnixMilli(),
		"source":    "window-global",
	}); err != nil {
		log.Printf("[window-shortcuts] Failed to dispatch find toggle: %v", err)
	}
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
