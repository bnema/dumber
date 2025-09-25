package browser

import (
	"log"

	"github.com/bnema/dumber/pkg/webkit"
)

// FocusManager centralizes all focus/active pane management logic
// This is the Single Source of Truth for pane activation
type FocusManager struct {
	wm *WorkspaceManager

	// Callbacks for side-effects
	onFocusChanged []FocusChangeCallback
}

// FocusChangeCallback is called when focus changes between panes
type FocusChangeCallback func(oldPane, newPane *paneNode)

// NewFocusManager creates a new centralized focus manager
func NewFocusManager(wm *WorkspaceManager) *FocusManager {
	return &FocusManager{
		wm:             wm,
		onFocusChanged: make([]FocusChangeCallback, 0),
	}
}

// AddFocusChangeCallback registers a callback for focus changes
func (fm *FocusManager) AddFocusChangeCallback(callback FocusChangeCallback) {
	fm.onFocusChanged = append(fm.onFocusChanged, callback)
}

// SetActivePane is the ONLY method to change the active pane
// All other methods (focusNode, JS bridges, etc.) should call this
func (fm *FocusManager) SetActivePane(node *paneNode) {
	if fm == nil {
		log.Printf("[focus-manager] WARNING: SetActivePane called on nil FocusManager")
		return
	}

	if node == nil {
		log.Printf("[focus-manager] WARNING: SetActivePane called with nil node")
		return
	}

	// Validate node exists in the tree
	if !fm.isValidNode(node) {
		log.Printf("[focus-manager] WARNING: SetActivePane called with invalid node %p", node)
		return
	}

	oldPane := fm.getCurrentActivePane()

	// No-op if already active
	if oldPane == node {
		log.Printf("[focus-manager] SetActivePane: %p already active", node)
		return
	}

	log.Printf("[focus-manager] SetActivePane: %p -> %p", oldPane, node)

	// Step 1: GTK4 Focus (Single Source of Truth)
	fm.setGTKFocus(node)

	// Step 2: Update visual indicators
	fm.updateVisualState(oldPane, node)

	// Step 3: Notify JavaScript bridge
	fm.notifyJavaScript(oldPane, node)

	// Step 4: Sync app.activePane for window shortcuts
	if fm.wm != nil && fm.wm.app != nil && node.pane != nil {
		fm.wm.app.activePane = node.pane
		log.Printf("[focus-manager] Synced app.activePane to %p", node.pane.webView)
	}

	// Step 5: Execute callbacks
	fm.executeCallbacks(oldPane, node)
}

// GetActivePane returns the currently active pane
// This is derived from GTK4 focus state, not stored separately
func (fm *FocusManager) GetActivePane() *paneNode {
	return fm.getCurrentActivePane()
}

// ===== INTERNAL METHODS =====

// setGTKFocus sets the GTK4 focus on the pane's webview
func (fm *FocusManager) setGTKFocus(node *paneNode) {
	if node.pane == nil || node.pane.webView == nil {
		log.Printf("[focus-manager] ERROR: Cannot focus node with nil pane/webView")
		return
	}

	// Get the widget handle for the webview
	viewWidget := node.pane.webView.Widget()
	if viewWidget == 0 {
		log.Printf("[focus-manager] ERROR: Cannot focus node with invalid widget")
		return
	}

	// This is the Single Source of Truth for focus
	webkit.WidgetGrabFocus(viewWidget)
}

// updateVisualState manages CSS classes and visibility
func (fm *FocusManager) updateVisualState(oldPane, newPane *paneNode) {
	activePaneClass := "workspace-pane-active"

	// Remove active class from old pane
	if oldPane != nil && oldPane.container != 0 {
		webkit.WidgetRemoveCSSClass(oldPane.container, activePaneClass)
		webkit.WidgetQueueDraw(oldPane.container)
	}

	// Add active class to new pane
	if newPane.container != 0 {
		webkit.WidgetAddCSSClass(newPane.container, activePaneClass)
		webkit.WidgetQueueDraw(newPane.container)
	}

	// FATAL CHECK: Ensure only ONE pane has active class
	fm.verifyOnlyOneActivePaneOrPanic()

	// Handle stacked panes visibility
	fm.updateStackedPaneVisibility(newPane)
}

// updateStackedPaneVisibility handles special case for stacked panes
func (fm *FocusManager) updateStackedPaneVisibility(node *paneNode) {
	// Find if this node is part of a stack
	stackNode := fm.findStackContainer(node)
	if stackNode == nil {
		return // Not in a stack
	}

	// Update stack visibility based on the focused pane
	activeIndex := -1
	for i, stackedPane := range stackNode.stackedPanes {
		if stackedPane == node {
			activeIndex = i
			break
		}
	}

	if activeIndex >= 0 {
		stackNode.activeStackIndex = activeIndex
		fm.updateStackVisibility(stackNode)
	}
}

// updateStackVisibility updates visibility of panes in a stack
func (fm *FocusManager) updateStackVisibility(stackNode *paneNode) {
	activeIndex := stackNode.activeStackIndex
	if activeIndex < 0 || activeIndex >= len(stackNode.stackedPanes) {
		return
	}

	// Hide all panes in stack
	for i, pane := range stackNode.stackedPanes {
		if i != activeIndex {
			webkit.WidgetSetVisible(pane.container, false)
			webkit.WidgetSetVisible(pane.titleBar, true)
			webkit.WidgetRemoveCSSClass(pane.container, "stacked-pane-active")
			webkit.WidgetAddCSSClass(pane.container, "stacked-pane-collapsed")
		}
	}

	// Show active pane
	activePaneNode := stackNode.stackedPanes[activeIndex]
	webkit.WidgetSetVisible(activePaneNode.container, true)
	webkit.WidgetSetVisible(activePaneNode.titleBar, false)
	webkit.WidgetAddCSSClass(activePaneNode.container, "stacked-pane-active")
	webkit.WidgetRemoveCSSClass(activePaneNode.container, "stacked-pane-collapsed")
}

// notifyJavaScript dispatches focus events to the JavaScript bridge
func (fm *FocusManager) notifyJavaScript(oldPane, newPane *paneNode) {
	// Notify old pane it lost focus
	if oldPane != nil && oldPane.pane != nil && oldPane.pane.webView != nil {
		// Update WebView internal active state
		oldPane.pane.webView.SetActive(false)
		
		oldDetail := map[string]any{
			"active": false,
			"paneId": fm.getPaneID(oldPane),
		}
		if err := oldPane.pane.webView.DispatchCustomEvent("dumber:workspace-focus", oldDetail); err != nil {
			log.Printf("[focus-manager] Failed to dispatch blur event: %v", err)
		}
	}

	// Notify new pane it gained focus
	if newPane != nil && newPane.pane != nil && newPane.pane.webView != nil {
		// Update WebView internal active state
		newPane.pane.webView.SetActive(true)
		
		newDetail := map[string]any{
			"active": true,
			"paneId": fm.getPaneID(newPane),
		}
		if err := newPane.pane.webView.DispatchCustomEvent("dumber:workspace-focus", newDetail); err != nil {
			log.Printf("[focus-manager] Failed to dispatch focus event: %v", err)
		}
	}
}

// executeCallbacks runs all registered focus change callbacks
func (fm *FocusManager) executeCallbacks(oldPane, newPane *paneNode) {
	for _, callback := range fm.onFocusChanged {
		callback(oldPane, newPane)
	}
}

// ===== HELPER METHODS =====

// getCurrentActivePane determines the currently active pane from GTK4 focus
func (fm *FocusManager) getCurrentActivePane() *paneNode {
	// TODO: Implement proper GTK4 focus traversal
	// For now, we'll track this internally until GTK4 focus callbacks are implemented
	return fm.wm.currentlyFocused
}

// isValidNode checks if a node exists in the current tree
func (fm *FocusManager) isValidNode(node *paneNode) bool {
	if fm == nil || fm.wm == nil || node == nil || node.pane == nil || node.pane.webView == nil {
		return false
	}
	return fm.wm.viewToNode[node.pane.webView] == node
}

// findStackContainer finds the stack container that contains this node
func (fm *FocusManager) findStackContainer(node *paneNode) *paneNode {
	// Traverse up the tree looking for a stack container
	current := node
	for current != nil {
		if current.stackedPanes != nil {
			// This is a stack container, check if it contains our node
			for _, stackedPane := range current.stackedPanes {
				if stackedPane == node {
					return current
				}
			}
		}
		// Move up the tree (we'd need parent pointers for this)
		// For now, traverse the entire tree
		break
	}
	return nil
}

// getPaneID gets a unique identifier for a pane (for JS bridge)
func (fm *FocusManager) getPaneID(node *paneNode) string {
	if node.pane == nil || node.pane.webView == nil {
		return ""
	}
	// Use the webview ID as a unique identifier
	return node.pane.webView.ID()
}

// SetActivePaneByView sets active pane by WebView reference
func (fm *FocusManager) SetActivePaneByView(view *webkit.WebView) {
	if node := fm.wm.viewToNode[view]; node != nil {
		fm.SetActivePane(node)
	} else {
		log.Printf("[focus-manager] WARNING: SetActivePaneByView called with unknown view %p", view)
	}
}

// verifyOnlyOneActivePaneOrPanic performs a FATAL check to ensure only one pane is active
// This is a fundamental rule that MUST NEVER be violated
func (fm *FocusManager) verifyOnlyOneActivePaneOrPanic() {
	if fm == nil || fm.wm == nil {
		return
	}

	activePaneClass := "workspace-pane-active"
	activeCount := 0
	var activePanes []uintptr

	// Check all leaf panes in the workspace
	leaves := fm.wm.collectLeaves()
	for _, leaf := range leaves {
		if leaf != nil && leaf.container != 0 {
			if webkit.WidgetHasCSSClass(leaf.container, activePaneClass) {
				activeCount++
				activePanes = append(activePanes, leaf.container)
			}
		}
	}

	// FATAL: Multiple active panes detected
	if activeCount > 1 {
		log.Fatalf("[focus-manager] FATAL: Multiple active panes detected! Count=%d, Containers=%v. Only ONE pane can be active at a time. This violates a fundamental rule.", activeCount, activePanes)
	}

	// FATAL: No active panes when there should be one
	if activeCount == 0 && len(leaves) > 0 {
		log.Fatalf("[focus-manager] FATAL: No active panes detected but %d panes exist. There should always be exactly ONE active pane.", len(leaves))
	}

	log.Printf("[focus-manager] ✓ Single active pane rule verified: %d active out of %d total panes", activeCount, len(leaves))
}
