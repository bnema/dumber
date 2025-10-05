package browser

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/bnema/dumber/internal/app/control"
	"github.com/bnema/dumber/internal/config"
	"github.com/bnema/dumber/pkg/webkit"
)

// ShortcutHandler manages keyboard shortcuts for the browser
type ShortcutHandler struct {
	webView             *webkit.WebView
	clipboardController *control.ClipboardController
	config              *config.Config
	app                 *BrowserApp
}

// NewShortcutHandler creates a new shortcut handler
func NewShortcutHandler(webView *webkit.WebView, clipboardController *control.ClipboardController, cfg *config.Config, app *BrowserApp) *ShortcutHandler {
	return &ShortcutHandler{
		webView:             webView,
		clipboardController: clipboardController,
		config:              cfg,
		app:                 app,
	}
}

func (s *ShortcutHandler) isActivePane() bool {
	if s == nil || s.webView == nil {
		return false
	}
	if s.app == nil || s.app.workspace == nil {
		return true // No workspace context; allow
	}

	// Find node for this WebView
	node := s.app.workspace.viewToNode[s.webView]
	if node == nil {
		return false
	}

	// For related popups, they're active if parent chain contains the active node
	if node.isRelated && node.parentPane != nil {
		activeNode := s.app.workspace.GetActiveNode()
		cur := node
		for cur != nil {
			if cur == activeNode {
				return true
			}
			cur = cur.parentPane
		}
		return false
	}

	// Independent panes: equal to active node
	return node == s.app.workspace.GetActiveNode()
}

// RegisterShortcuts registers pane-specific keyboard shortcuts with focus guards
func (s *ShortcutHandler) RegisterShortcuts() {
	s.exposeWorkspaceConfig()

	// NOTE: Global shortcuts (Ctrl+L, Ctrl+F, Ctrl+Shift+C, F12) are now handled
	// by WindowShortcutHandler at the window level to prevent duplicates

	// Page refresh shortcuts
	_ = s.webView.RegisterKeyboardShortcut("cmdorctrl+r", func() {
		if !s.isActivePane() {
			return
		}
		log.Printf("Shortcut: Reload page")
		_ = s.webView.Reload()
	})

	_ = s.webView.RegisterKeyboardShortcut("cmdorctrl+shift+r", func() {
		if !s.isActivePane() {
			return
		}
		log.Printf("Shortcut: Hard reload page")
		_ = s.webView.ReloadBypassCache()
	})

	_ = s.webView.RegisterKeyboardShortcut("F5", func() {
		if !s.isActivePane() {
			return
		}
		log.Printf("Shortcut: F5 reload")
		_ = s.webView.Reload()
	})

	// Note: Workspace pane navigation shortcuts (Alt + Arrow keys) are handled by WorkspaceManager
	// and zoom shortcuts (Ctrl +/-/0) are handled by WindowShortcutHandler to ensure they work properly across all panes
}

func (s *ShortcutHandler) dispatchUIShortcut(action string, extra map[string]any) error {
	if s == nil || s.webView == nil {
		return fmt.Errorf("webview unavailable for action %s", action)
	}

	detail := map[string]any{
		"action": action,
	}
	for k, v := range extra {
		detail[k] = v
	}

	return s.webView.DispatchCustomEvent("dumber:ui:shortcut", detail)
}

func (s *ShortcutHandler) exposeWorkspaceConfig() {
	if s.webView == nil || s.config == nil {
		return
	}

	payload, err := json.Marshal(s.config.Workspace)
	if err != nil {
		log.Printf("Shortcut: failed to marshal workspace config: %v", err)
		return
	}

	script := fmt.Sprintf(`(function(){
	const cfg = %s;
	window.__dumber_workspace_config = cfg;
	document.dispatchEvent(new CustomEvent('dumber:workspace-config',{detail:cfg}));
})();`, string(payload))

	if err := s.webView.InjectScript(script); err != nil {
		log.Printf("Shortcut: failed to expose workspace config: %v", err)
	}
}

// Detach releases the WebView reference to prevent post-destruction usage.
func (s *ShortcutHandler) Detach() {
	if s == nil {
		return
	}
	s.webView = nil
}
