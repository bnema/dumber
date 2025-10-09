// workspace_hover.go - Hover handler management for workspace panes
package browser

import (
	"log"

	"github.com/bnema/dumber/pkg/webkit"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
)

// ensureHover attaches hover handlers to a pane node for mouse-based focus changes
func (wm *WorkspaceManager) ensureHover(node *paneNode) {
	if wm == nil || node == nil || !node.isLeaf {
		return
	}

	// Skip if already attached or no container
	if node.container == nil || node.hoverToken != nil {
		return
	}

	// Attach hover handler to the pane container
	controller := webkit.WidgetAddHoverHandler(node.container, func() {
		if wm == nil || node == nil {
			return
		}

		// Skip if this pane is already active to avoid redundant focus churn.
		if wm.GetActiveNode() == node {
			return
		}

		// Debounce hover events to prevent rapid focus changes (minimum 150ms between hover-triggered focus changes)
		now := glib.GetMonotonicTime()
		if node.lastHoverTime > 0 && (now - node.lastHoverTime) < 150000 { // 150ms in microseconds
			return
		}
		node.lastHoverTime = now

		// Request focus change when mouse enters the pane
		wm.SetActivePane(node, SourceMouse)
	})

	if controller != nil {
		node.hoverToken = controller
		log.Printf("[workspace] Attached hover handler to pane %p (controller=%p)", node, controller)
	} else {
		log.Printf("[workspace] Failed to attach hover handler to pane %p", node)
	}
}

// detachHover removes hover handlers from a pane node
func (wm *WorkspaceManager) detachHover(node *paneNode) {
	if wm == nil || node == nil || node.hoverToken == nil {
		return
	}

	if node.container == nil {
		node.hoverToken = nil
		return
	}

	webkit.WidgetRemoveHoverHandler(node.container, node.hoverToken)
	log.Printf("[workspace] Detached hover handler from pane %p (controller=%p)", node, node.hoverToken)
	node.hoverToken = nil
}

// attachHoverHandlersToAllPanes attaches hover handlers to all leaf panes
func (wm *WorkspaceManager) attachHoverHandlersToAllPanes() {
	if wm == nil {
		return
	}

	leaves := wm.collectLeaves()
	for _, leaf := range leaves {
		wm.ensureHover(leaf)
	}

	log.Printf("[workspace] Attached hover handlers to %d panes", len(leaves))
}
