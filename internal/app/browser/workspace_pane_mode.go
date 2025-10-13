// workspace_pane_mode.go - Backend-driven pane mode implementation
package browser

import (
	"log"

	"github.com/bnema/dumber/pkg/webkit"
)

// EnterPaneMode activates pane mode on the currently focused pane
// This is called directly from the keyboard bridge, not from JavaScript
func (wm *WorkspaceManager) EnterPaneMode() {
	wm.paneMutex.Lock()
	defer wm.paneMutex.Unlock()

	// Get the currently focused pane
	activeNode := wm.GetActiveNode()
	if activeNode == nil {
		log.Printf("[pane-mode] Cannot enter pane mode: no active pane")
		return
	}

	// Already in pane mode on this pane?
	if wm.paneModeActive && wm.paneModeActivePane == activeNode {
		log.Printf("[pane-mode] Pane mode already active on this pane")
		return
	}

	log.Printf("[pane-mode] Entering pane mode on pane %p", activeNode)

	wm.paneModeActive = true
	wm.paneModeActivePane = activeNode

	if activeNode.pane != nil && activeNode.pane.webView != nil {
		wm.paneModeSource = activeNode.pane.webView
	}

	// Apply visual border to workspace root
	wm.applyPaneModeBorder()

	// Notify JavaScript for UI feedback (visual indicators)
	wm.dispatchPaneModeEvent("entered", "")
}

// ExitPaneMode deactivates pane mode
func (wm *WorkspaceManager) ExitPaneMode(reason string) {
	wm.paneMutex.Lock()
	defer wm.paneMutex.Unlock()

	if !wm.paneModeActive {
		return
	}

	log.Printf("[pane-mode] Exiting pane mode: %s", reason)

	wm.paneModeActive = false
	wm.paneModeActivePane = nil
	wm.paneModeSource = nil

	// Remove visual border from workspace root
	wm.removePaneModeBorder()

	// Notify JavaScript
	wm.dispatchPaneModeEvent("exited", reason)
}

// HandlePaneAction processes pane mode actions (close, split, etc.)
// Called from keyboard bridge when user presses action keys in pane mode
func (wm *WorkspaceManager) HandlePaneAction(action string) {
	wm.paneMutex.Lock()

	if !wm.paneModeActive {
		wm.paneMutex.Unlock()
		log.Printf("[pane-mode] Ignoring action '%s': pane mode not active", action)
		return
	}

	activePane := wm.paneModeActivePane
	wm.paneMutex.Unlock()

	if activePane == nil {
		log.Printf("[pane-mode] Ignoring action '%s': no active pane", action)
		wm.ExitPaneMode("no-active-pane")
		return
	}

	log.Printf("[pane-mode] Handling action '%s' on pane %p", action, activePane)

	switch action {
	case "close":
		wm.handlePaneClose(activePane)
	case "split-right", "split-left", "split-up", "split-down":
		wm.handlePaneSplit(activePane, action)
	default:
		log.Printf("[pane-mode] Unknown action: %s", action)
	}

	// Exit pane mode after action
	wm.ExitPaneMode(action)
}

// handlePaneClose closes the pane that has pane mode active
func (wm *WorkspaceManager) handlePaneClose(node *paneNode) {
	if node == nil {
		return
	}

	log.Printf("[pane-mode] Closing pane %p", node)

	// Notify JavaScript first so it can show toast
	wm.dispatchPaneModeEvent("action", "close")

	// Close the pane
	if err := wm.ClosePane(node); err != nil {
		log.Printf("[pane-mode] Failed to close pane: %v", err)
	}
}

// handlePaneSplit splits the pane in the specified direction
func (wm *WorkspaceManager) handlePaneSplit(node *paneNode, action string) {
	if node == nil {
		return
	}

	// Extract direction from action (e.g., "split-right" -> "right")
	var direction string
	switch action {
	case "split-right":
		direction = "right"
	case "split-left":
		direction = "left"
	case "split-up":
		direction = "up"
	case "split-down":
		direction = "down"
	default:
		log.Printf("[pane-mode] Invalid split action: %s", action)
		return
	}

	log.Printf("[pane-mode] Splitting pane %p in direction %s", node, direction)

	// Notify JavaScript first so it can show toast
	wm.dispatchPaneModeEvent("action", action)

	// Perform the split (pass nil so it creates a fresh pane, not popup mode)
	newNode, err := wm.splitNode(node, direction, nil)
	if err != nil {
		log.Printf("[pane-mode] Failed to split pane: %v", err)
		return
	}

	// Load initial URL in the new pane so scripts execute
	wm.clonePaneState(node, newNode)

	log.Printf("[pane-mode] Split successful: direction=%s", direction)
}

// dispatchPaneModeEvent sends a pane mode event to JavaScript for UI updates
func (wm *WorkspaceManager) dispatchPaneModeEvent(event string, detail string) {
	if wm.paneModeActivePane == nil || wm.paneModeActivePane.pane == nil || wm.paneModeActivePane.pane.webView == nil {
		return
	}

	view := wm.paneModeActivePane.pane.webView
	data := map[string]interface{}{
		"event":  event,
		"detail": detail,
	}

	if err := view.DispatchCustomEvent("dumber:pane-mode", data); err != nil {
		log.Printf("[pane-mode] Failed to dispatch event: %v", err)
	}
}

// applyPaneModeBorder adds the pane mode CSS class to the workspace root container
func (wm *WorkspaceManager) applyPaneModeBorder() {
	if wm.root == nil || wm.root.container == nil {
		return
	}

	// Add CSS class to root container for Zellij-style border
	if !webkit.WidgetHasCSSClass(wm.root.container, "workspace-pane-mode-active") {
		webkit.WidgetAddCSSClass(wm.root.container, "workspace-pane-mode-active")
		log.Printf("[pane-mode] Applied border to workspace root")
	}
}

// removePaneModeBorder removes the pane mode CSS class from the workspace root container
func (wm *WorkspaceManager) removePaneModeBorder() {
	if wm.root == nil || wm.root.container == nil {
		return
	}

	// Remove CSS class from root container
	if webkit.WidgetHasCSSClass(wm.root.container, "workspace-pane-mode-active") {
		webkit.WidgetRemoveCSSClass(wm.root.container, "workspace-pane-mode-active")
		log.Printf("[pane-mode] Removed border from workspace root")
	}
}
