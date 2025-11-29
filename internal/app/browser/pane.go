package browser

import (
	"fmt"
	"time"

	"github.com/bnema/dumber/internal/app/control"
	"github.com/bnema/dumber/internal/app/messaging"
	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/dumber/pkg/webkit"
)

// BrowserPane bundles the per-pane WebView and its controllers.
type BrowserPane struct {
	webView              *webkit.WebView
	navigationController *control.NavigationController
	zoomController       *control.ZoomController
	clipboardController  *control.ClipboardController
	messageHandler       *messaging.Handler
	shortcutHandler      *ShortcutHandler

	// Multi-pane support
	id            string
	hasGUI        bool
	guiComponents map[string]bool
	lastFocusTime time.Time
}

func (p *BrowserPane) WebView() *webkit.WebView { return p.webView }

func (p *BrowserPane) MessageHandler() *messaging.Handler { return p.messageHandler }

func (p *BrowserPane) NavigationController() *control.NavigationController {
	return p.navigationController
}

func (p *BrowserPane) ZoomController() *control.ZoomController { return p.zoomController }

func (p *BrowserPane) ClipboardController() *control.ClipboardController {
	return p.clipboardController
}

func (p *BrowserPane) ShortcutHandler() *ShortcutHandler { return p.shortcutHandler }

// ID returns the unique pane identifier
func (p *BrowserPane) ID() string { return p.id }

// HasGUI returns whether GUI components are injected
func (p *BrowserPane) HasGUI() bool { return p.hasGUI }

// SetHasGUI marks GUI components as injected
func (p *BrowserPane) SetHasGUI(has bool) { p.hasGUI = has }

// SetID sets the pane identifier
func (p *BrowserPane) SetID(id string) { p.id = id }

// UpdateLastFocus updates the last focus time
func (p *BrowserPane) UpdateLastFocus() { p.lastFocusTime = time.Now() }

// Initialize GUI component tracking
func (p *BrowserPane) initializeGUITracking() {
	if p.guiComponents == nil {
		p.guiComponents = make(map[string]bool)
	}
}

// HasGUIComponent checks if a specific GUI component is loaded
func (p *BrowserPane) HasGUIComponent(component string) bool {
	if p.guiComponents == nil {
		return false
	}
	return p.guiComponents[component]
}

// SetGUIComponent marks a GUI component as loaded
func (p *BrowserPane) SetGUIComponent(component string, loaded bool) {
	p.initializeGUITracking()
	p.guiComponents[component] = loaded
}

// Cleanup releases pane resources
func (p *BrowserPane) Cleanup() {
	if p == nil {
		return
	}

	if p.shortcutHandler != nil {
		p.shortcutHandler.Detach()
		logging.Debug(fmt.Sprintf("[pane-%s] Cleaned up shortcuts", p.id))
	}

	if p.navigationController != nil {
		p.navigationController.Detach()
	}

	if p.zoomController != nil {
		p.zoomController.DetachWebView()
	}

	if p.clipboardController != nil {
		p.clipboardController.Detach()
	}

	if p.messageHandler != nil {
		p.messageHandler.SetWebView(nil)
	}

	// Destroy WebView to unregister from global registry and release resources
	if p.webView != nil && !p.webView.IsDestroyed() {
		if err := p.webView.Destroy(); err != nil {
			logging.Error(fmt.Sprintf("[pane-%s] failed to destroy webview: %v", p.id, err))
		} else {
			logging.Debug(fmt.Sprintf("[pane-%s] successfully destroyed webview", p.id))
		}
	}

	// Clear tracking
	p.guiComponents = nil
	p.hasGUI = false
}

// CleanupFromWorkspace removes this pane from workspace tracking maps and app.panes slice
// This should be called before Cleanup() to ensure proper workspace state management
func (p *BrowserPane) CleanupFromWorkspace(wm interface{}) {
	if p == nil || p.webView == nil {
		return
	}

	// Type assert to avoid circular import - workspace manager will pass itself
	if workspaceManager, ok := wm.(interface {
		removeFromMaps(*webkit.WebView)
		removeFromAppPanes(*BrowserPane)
	}); ok {
		workspaceManager.removeFromMaps(p.webView)
		workspaceManager.removeFromAppPanes(p)
		logging.Debug(fmt.Sprintf("[pane-%s] Cleaned up from workspace tracking", p.id))
	}
}
