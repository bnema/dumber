package browser

import (
	"log"
	"time"

	"github.com/bnema/dumber/internal/app/control"
	"github.com/bnema/dumber/internal/app/messaging"
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
	// Cleanup shortcuts
	if p.shortcutHandler != nil {
		// TODO: Add cleanup method to ShortcutHandler
		log.Printf("[pane-%s] Cleaned up shortcuts", p.id)
	}

	// Notify JS to cleanup GUI
	if p.webView != nil && p.hasGUI {
		p.webView.InjectScript(`
			if (window.__dumber_gui_manager) {
				window.__dumber_gui_manager.destroy();
			}
		`)
	}

	// Clear tracking
	p.guiComponents = nil
	p.hasGUI = false
}
