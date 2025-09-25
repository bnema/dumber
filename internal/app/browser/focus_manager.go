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
	fm.updateVisualState(node)

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
func (fm *FocusManager) updateVisualState(newPane *paneNode) {
	activePaneClass := "workspace-pane-active"

	log.Printf("[focus-manager] updateVisualState: setting active pane to %p", newPane)

	// CRITICAL: For stacked panes, we need to remove active class from ALL panes in ALL stacks
	// collectLeaves() only returns the currently active pane in each stack, missing the others
	fm.removeActiveCSSFromAllPanes(activePaneClass)

	// Add active class to new pane only
	if newPane != nil && newPane.container != nil {
		newPane.container.Execute(func(containerPtr uintptr) error {
			log.Printf("[focus-manager] Adding CSS class '%s' to new active pane: container=%#x", activePaneClass, containerPtr)
			webkit.WidgetAddCSSClass(containerPtr, activePaneClass)
			webkit.WidgetQueueDraw(containerPtr)
			return nil
		})
	} else {
		log.Printf("[focus-manager] WARNING: newPane is nil or has no container, cannot add active CSS class")
	}

	// FATAL CHECK: Ensure only ONE pane has active class
	fm.verifyOnlyOneActivePaneOrPanic()

	// Handle stacked panes visibility
	fm.updateStackedPaneVisibility(newPane)
}

// removeActiveCSSFromAllPanes removes the active CSS class from ALL panes, including stacked panes
// This is critical for stacked panes where collectLeaves() only returns the active pane
// removeActiveCSSFromAllPanes removes the active CSS class from ALL panes, including stacked panes
// This is critical for stacked panes where collectLeaves() only returns the active pane
func (fm *FocusManager) removeActiveCSSFromAllPanes(activePaneClass string) {
	if fm.wm == nil || fm.wm.root == nil {
		return
	}

	log.Printf("[focus-manager] removeActiveCSSFromAllPanes: starting CSS cleanup for class '%s'", activePaneClass)

	// Walk the entire tree and remove active class from ALL leaf panes
	// This includes both regular panes and ALL panes within stacks
	var walkAndRemove func(*paneNode, int)
	walkAndRemove = func(n *paneNode, depth int) {
		const maxDepth = 50
		if n == nil || depth > maxDepth {
			return
		}

		if n.isLeaf {
			// Regular leaf pane - remove active class
			if n.container != nil {
				n.container.Execute(func(containerPtr uintptr) error {
					log.Printf("[focus-manager] Removing CSS class '%s' from regular pane: container=%#x", activePaneClass, containerPtr)
					webkit.WidgetRemoveCSSClass(containerPtr, activePaneClass)
					return nil
				})
			}
			return
		}

		if n.isStacked && len(n.stackedPanes) > 0 {
			// CRITICAL: For stacked panes, remove active class from ALL panes in the stack
			// not just the currently active one (which is what collectLeaves() returns)
			log.Printf("[focus-manager] Processing stacked pane: activeIndex=%d stackSize=%d", n.activeStackIndex, len(n.stackedPanes))
			for i, stackedPane := range n.stackedPanes {
				if stackedPane != nil && stackedPane.container != nil {
					stackedPane.container.Execute(func(containerPtr uintptr) error {
						log.Printf("[focus-manager] Removing CSS class '%s' from stacked pane[%d]: container=%#x", activePaneClass, i, containerPtr)
						webkit.WidgetRemoveCSSClass(containerPtr, activePaneClass)
						return nil
					})
				}
			}
			return
		}

		// Handle regular split nodes
		walkAndRemove(n.left, depth+1)
		walkAndRemove(n.right, depth+1)
	}

	walkAndRemove(fm.wm.root, 0)
	log.Printf("[focus-manager] removeActiveCSSFromAllPanes: completed CSS cleanup")
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
			pane.container.Execute(func(containerPtr uintptr) error {
				webkit.WidgetSetVisible(containerPtr, false)
				webkit.WidgetRemoveCSSClass(containerPtr, "stacked-pane-active")
				webkit.WidgetAddCSSClass(containerPtr, "stacked-pane-collapsed")
				return nil
			})
			pane.titleBar.Execute(func(titleBarPtr uintptr) error {
				webkit.WidgetSetVisible(titleBarPtr, true)
				return nil
			})
		}
	}

	// Show active pane
	activePaneNode := stackNode.stackedPanes[activeIndex]
	activePaneNode.container.Execute(func(containerPtr uintptr) error {
		webkit.WidgetSetVisible(containerPtr, true)
		webkit.WidgetAddCSSClass(containerPtr, "stacked-pane-active")
		webkit.WidgetRemoveCSSClass(containerPtr, "stacked-pane-collapsed")
		return nil
	})
	activePaneNode.titleBar.Execute(func(titleBarPtr uintptr) error {
		webkit.WidgetSetVisible(titleBarPtr, false)
		return nil
	})
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
	// Check if the immediate parent is a stack container
	if node != nil && node.parent != nil && node.parent.isStacked {
		return node.parent
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
		if leaf != nil && leaf.container != nil {
			leaf.container.Execute(func(containerPtr uintptr) error {
				if webkit.WidgetHasCSSClass(containerPtr, activePaneClass) {
					activeCount++
					activePanes = append(activePanes, containerPtr)
				}
				return nil
			})
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
