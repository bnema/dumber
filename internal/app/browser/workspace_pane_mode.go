// workspace_pane_mode.go - Backend-driven pane mode implementation
package browser

import (
	"log"

	"github.com/bnema/dumber/pkg/webkit"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// EnterPaneMode activates pane mode on the currently focused pane
// This is called directly from the keyboard bridge, not from JavaScript
func (wm *WorkspaceManager) EnterPaneMode() {
	// Note: Caller (RegisterPaneModeHandler) ensures this is called on app.workspace (active tab)

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

	log.Printf("[pane-mode] Entering pane mode on pane %p (workspace=%p)", activeNode, wm)

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
	// Note: No IsInActiveTab check here - caller (RegisterPaneModeHandler) ensures
	// this is called on app.workspace (active tab's workspace)

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
	case "stack":
		wm.handlePaneStack(activePane)
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

	// Extract direction from action (e.g., "split-right" -> DirectionRight)
	var direction string
	switch action {
	case "split-right":
		direction = DirectionRight
	case "split-left":
		direction = DirectionLeft
	case "split-up":
		direction = DirectionUp
	case "split-down":
		direction = DirectionDown
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

// handlePaneStack creates a stacked pane
func (wm *WorkspaceManager) handlePaneStack(node *paneNode) {
	if node == nil {
		return
	}

	log.Printf("[pane-mode] Stacking pane %p", node)

	// Notify JavaScript first so it can show toast
	wm.dispatchPaneModeEvent("action", "stack")

	// Perform the stack operation
	newNode, err := wm.stackedPaneManager.StackPane(node)
	if err != nil {
		log.Printf("[pane-mode] Failed to stack pane: %v", err)
		return
	}

	// Load initial URL in the new pane so scripts execute
	wm.clonePaneState(node, newNode)

	log.Printf("[pane-mode] Stack successful")
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

// applyPaneModeBorder applies pane mode visual indicator using GTK margins
func (wm *WorkspaceManager) applyPaneModeBorder() {
	if wm.window == nil {
		return
	}

	// Determine which container to apply margins to:
	// - If tab manager exists, apply to its ContentArea (so window background shows through)
	// - Otherwise, apply to workspace root (direct child of window)
	var targetContainer gtk.Widgetter
	if wm.app != nil && wm.app.tabManager != nil && wm.app.tabManager.ContentArea != nil {
		// Tab environment: apply margins to tab manager's content area
		targetContainer = wm.app.tabManager.ContentArea
		log.Printf("[pane-mode] Using tab manager's content area for border")
	} else if wm.root != nil && wm.root.container != nil {
		// Non-tab environment: apply margins to workspace root
		targetContainer = wm.root.container
		log.Printf("[pane-mode] Using workspace root for border")
	} else {
		return
	}

	// Save the container reference so we can remove margins from it later
	wm.paneModeContainer = targetContainer

	// Apply 4px margins to create space for the border
	webkit.WidgetSetMargin(targetContainer, 4)

	// Add CSS class to window for background color (the "border" color shows in the margin space)
	webkit.WidgetAddCSSClass(wm.window.AsWindow(), "pane-mode-active")

	// Force resize/allocation to apply margin changes immediately
	webkit.WidgetQueueResize(targetContainer)
	webkit.WidgetQueueAllocate(targetContainer)

	// Queue redraw to show changes
	webkit.WidgetQueueDraw(wm.window.AsWindow())
	webkit.WidgetQueueDraw(targetContainer)

	log.Printf("[pane-mode] Applied border using margins (container=%p)", targetContainer)
}

// removePaneModeBorder removes the pane mode visual indicator
func (wm *WorkspaceManager) removePaneModeBorder() {
	if wm.window == nil {
		return
	}

	// Use the saved container reference (the one that had margins applied)
	// This is important because wm.root.container may have changed due to splits
	container := wm.paneModeContainer
	if container == nil {
		log.Printf("[pane-mode] Warning: no saved container, trying current root")
		if wm.root == nil || wm.root.container == nil {
			return
		}
		container = wm.root.container
	}

	// Remove margins from the container that had them applied
	webkit.WidgetSetMargin(container, 0)

	// Remove CSS class from window
	webkit.WidgetRemoveCSSClass(wm.window.AsWindow(), "pane-mode-active")

	// Force resize/allocation to apply margin changes immediately
	webkit.WidgetQueueResize(container)
	webkit.WidgetQueueAllocate(container)

	// Queue redraw to show changes
	webkit.WidgetQueueDraw(wm.window.AsWindow())
	webkit.WidgetQueueDraw(container)

	// Clear the saved container reference
	wm.paneModeContainer = nil

	log.Printf("[pane-mode] Removed border from workspace root (container=%p)", container)
}
