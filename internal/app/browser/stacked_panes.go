package browser

import (
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/bnema/dumber/pkg/webkit"
)

// StackedPaneManager handles all stacked pane operations
type StackedPaneManager struct {
	wm *WorkspaceManager
}

// NewStackedPaneManager creates a new stacked pane manager
func NewStackedPaneManager(wm *WorkspaceManager) *StackedPaneManager {
	return &StackedPaneManager{
		wm: wm,
	}
}

// StackPane creates a new pane stacked on top of the target pane.
// This is the main entry point for creating stacked panes.
func (spm *StackedPaneManager) StackPane(target *paneNode) (*paneNode, error) {
	if target == nil || !target.isLeaf || target.pane == nil {
		return nil, errors.New("stack target must be a leaf pane")
	}

	// Create the new pane first
	newLeaf, err := spm.prepareNewStackedPane()
	if err != nil {
		return nil, fmt.Errorf("failed to prepare new stacked pane: %v", err)
	}

	var stackNode *paneNode
	var insertIndex int

	// Check if target is already in a stack
	if target.parent != nil && target.parent.isStacked {
		// Target is already in a stack - add to existing stack
		stackNode, insertIndex, err = spm.addPaneToExistingStack(target, newLeaf)
		if err != nil {
			return nil, fmt.Errorf("failed to add pane to existing stack: %v", err)
		}
	} else {
		// Target is not stacked - create initial stack
		stackNode, insertIndex, err = spm.convertToStackedContainer(target, newLeaf)
		if err != nil {
			return nil, fmt.Errorf("failed to convert to stacked container: %v", err)
		}
	}

	// Finalize the stack creation
	return spm.finalizeStackCreation(stackNode, newLeaf, insertIndex)
}

// prepareNewStackedPane creates a new pane and its container
func (spm *StackedPaneManager) prepareNewStackedPane() (*paneNode, error) {
	newView, err := spm.wm.createWebView()
	if err != nil {
		return nil, err
	}

	newPane, err := spm.wm.createPane(newView)
	if err != nil {
		return nil, err
	}

	if handler := newPane.MessageHandler(); handler != nil {
		handler.SetWorkspaceObserver(spm.wm)
	}

	newContainer := newPane.webView.RootWidget()
	if newContainer == 0 {
		return nil, errors.New("new pane missing container")
	}
	webkit.WidgetSetHExpand(newContainer, true)
	webkit.WidgetSetVExpand(newContainer, true)
	webkit.WidgetRealizeInContainer(newContainer)

	// Create the new leaf node
	newLeaf := &paneNode{
		pane:      newPane,
		container: newContainer,
		isLeaf:    true,
	}

	// Create title bar for the new pane
	newTitleBar := spm.createTitleBar(newLeaf)
	newLeaf.titleBar = newTitleBar

	// Keep new container hidden initially - will be shown after transition
	webkit.WidgetSetVisible(newContainer, false)
	webkit.WidgetSetVisible(newTitleBar, false)

	return newLeaf, nil
}

// addPaneToExistingStack adds a new pane to an existing stack
func (spm *StackedPaneManager) addPaneToExistingStack(target, newLeaf *paneNode) (*paneNode, int, error) {
	stackNode := target.parent

	// Find the current position of the target pane in the stack
	currentIndex := -1
	for i, pane := range stackNode.stackedPanes {
		if pane == target {
			currentIndex = i
			break
		}
	}

	if currentIndex == -1 {
		return nil, 0, errors.New("target pane not found in stack")
	}

	// Insert the new pane right after the current pane
	insertIndex := currentIndex + 1
	log.Printf("[workspace] adding to existing stack: currentIndex=%d insertIndex=%d stackSize=%d",
		currentIndex, insertIndex, len(stackNode.stackedPanes))

	// Set parent relationship
	newLeaf.parent = stackNode

	// Insert the new pane at the correct position in the slice
	stackNode.stackedPanes = append(stackNode.stackedPanes, nil)                       // Expand slice
	copy(stackNode.stackedPanes[insertIndex+1:], stackNode.stackedPanes[insertIndex:]) // Shift elements
	stackNode.stackedPanes[insertIndex] = newLeaf                                      // Insert new pane

	// Unparent the new container before adding it to the stack (only if it has a parent)
	if webkit.WidgetGetParent(newLeaf.container) != 0 {
		webkit.WidgetUnparent(newLeaf.container)
	}

	// Add the new widgets to the internal stack box (not the wrapper)
	webkit.BoxAppend(stackNode.stackWrapper, newLeaf.titleBar)
	webkit.BoxAppend(stackNode.stackWrapper, newLeaf.container)

	return stackNode, insertIndex, nil
}

// convertToStackedContainer converts a simple pane to a stacked container
func (spm *StackedPaneManager) convertToStackedContainer(target, newLeaf *paneNode) (*paneNode, int, error) {
	log.Printf("[workspace] converting pane to stacked: %p", target)

	// Create the wrapper container - this is what will be used by splitNode
	stackWrapperContainer := webkit.NewBox(webkit.OrientationVertical, 0)
	if stackWrapperContainer == 0 {
		return nil, 0, errors.New("failed to create stack wrapper container")
	}
	webkit.WidgetSetHExpand(stackWrapperContainer, true)
	webkit.WidgetSetVExpand(stackWrapperContainer, true)

	// Create the internal box for the actual stacked widgets (titles + webviews)
	stackInternalBox := webkit.NewBox(webkit.OrientationVertical, 0)
	if stackInternalBox == 0 {
		return nil, 0, errors.New("failed to create stack internal box")
	}
	webkit.WidgetSetHExpand(stackInternalBox, true)
	webkit.WidgetSetVExpand(stackInternalBox, true)

	// The internal box goes inside the wrapper
	webkit.BoxAppend(stackWrapperContainer, stackInternalBox)

	// Get the existing container and parent info
	existingContainer := target.container
	parent := target.parent

	// Create title bar for the existing pane
	titleBar := spm.createTitleBar(target)

	// Keep existing container visible during transition to prevent rendering glitch
	webkit.WidgetSetVisible(titleBar, false)

	// Detach existing container from its current parent first
	spm.detachFromParent(target, parent)

	// Build the complete stack structure with hidden widgets
	webkit.BoxAppend(stackInternalBox, titleBar)
	webkit.BoxAppend(stackInternalBox, existingContainer)

	// Immediately reattach the stack wrapper to minimize visibility gap
	spm.reattachToParent(stackWrapperContainer, target, parent)

	// Convert target to a stacked leaf node
	target.isStacked = true
	target.isLeaf = true
	target.container = existingContainer
	target.titleBar = titleBar

	// Create the stack container node - container points to wrapper, stackWrapper points to internal box
	stackNode := &paneNode{
		isStacked:        true,
		isLeaf:           false,
		container:        stackWrapperContainer, // Wrapper for GTK operations (splits, etc.)
		stackWrapper:     stackInternalBox,      // Internal box for stack operations
		stackedPanes:     []*paneNode{target},
		activeStackIndex: 0, // KEEP CURRENT PANE ACTIVE during transition (index 0)
		parent:           parent,
	}

	// Update target's parent to be the stack node
	target.parent = stackNode

	// Update parent references (GTK operations already done above)
	if parent == nil {
		spm.wm.root = stackNode
	} else {
		if parent.left == target {
			parent.left = stackNode
		} else if parent.right == target {
			parent.right = stackNode
		}
	}

	// Set up the new pane
	newLeaf.parent = stackNode

	// Insert the new pane at index 1 (after the original pane)
	insertIndex := 1
	stackNode.stackedPanes = append(stackNode.stackedPanes, newLeaf)

	// Unparent the new container before adding it to the stack
	if webkit.WidgetGetParent(newLeaf.container) != 0 {
		webkit.WidgetUnparent(newLeaf.container)
	}

	// Add the new widgets to the internal stack box
	webkit.BoxAppend(stackNode.stackWrapper, newLeaf.titleBar)
	webkit.BoxAppend(stackNode.stackWrapper, newLeaf.container)

	return stackNode, insertIndex, nil
}

// detachFromParent removes a pane from its current parent
func (spm *StackedPaneManager) detachFromParent(target *paneNode, parent *paneNode) {
	if parent == nil {
		// Target is the root - remove it from window
		if spm.wm.window != nil {
			spm.wm.window.SetChild(0)
		}
		// Unparent if it has a GTK parent
		if webkit.WidgetGetParent(target.container) != 0 {
			webkit.WidgetUnparent(target.container)
		}
	} else if parent.container != 0 {
		// Target has a parent paned - remove it (automatically unparents in GTK4)
		if parent.left == target {
			webkit.PanedSetStartChild(parent.container, 0)
		} else if parent.right == target {
			webkit.PanedSetEndChild(parent.container, 0)
		}
	}
}

// reattachToParent attaches a container to the parent
func (spm *StackedPaneManager) reattachToParent(container uintptr, target *paneNode, parent *paneNode) {
	if parent == nil {
		if spm.wm.window != nil {
			spm.wm.window.SetChild(container)
		}
	} else if parent.container != 0 {
		if parent.left == target {
			webkit.PanedSetStartChild(parent.container, container)
		} else if parent.right == target {
			webkit.PanedSetEndChild(parent.container, container)
		}
	}
}

// finalizeStackCreation completes the stack creation process
func (spm *StackedPaneManager) finalizeStackCreation(stackNode, newLeaf *paneNode, insertIndex int) (*paneNode, error) {
	// Update workspace state first
	spm.wm.viewToNode[newLeaf.pane.webView] = newLeaf
	spm.wm.ensureHover(newLeaf)
	spm.wm.app.panes = append(spm.wm.app.panes, newLeaf.pane)
	if newLeaf.pane.zoomController != nil {
		newLeaf.pane.zoomController.ApplyInitialZoom()
	}

	// Update CSS classes
	spm.wm.ensurePaneBaseClasses()

	// Mark stack operation timestamp to prevent focus conflicts
	spm.wm.lastStackOperation = time.Now()

	// Update stack visibility to show current state (existing pane still active)
	spm.UpdateStackVisibility(stackNode)

	// Transition to the new pane after a brief delay to avoid rendering conflicts
	webkit.IdleAdd(func() bool {
		// Now switch to the new pane
		stackNode.activeStackIndex = insertIndex
		spm.UpdateStackVisibility(stackNode)
		spm.wm.currentlyFocused = newLeaf
		spm.wm.focusManager.SetActivePane(newLeaf)
		return false // Remove idle callback
	})

	log.Printf("[workspace] stacked new pane: stackNode=%p newLeaf=%p stackSize=%d activeIndex=%d insertIndex=%d",
		stackNode, newLeaf, len(stackNode.stackedPanes), stackNode.activeStackIndex, insertIndex)
	return newLeaf, nil
}

// UpdateStackVisibility updates the visibility of panes in a stack
func (spm *StackedPaneManager) UpdateStackVisibility(stackNode *paneNode) {
	log.Printf("[workspace] updateStackVisibility called: stackNode=%p", stackNode)

	if stackNode == nil {
		log.Printf("[workspace] updateStackVisibility aborted: stackNode is nil")
		return
	}

	if !stackNode.isStacked {
		log.Printf("[workspace] updateStackVisibility aborted: stackNode.isStacked=%v", stackNode.isStacked)
		return
	}

	if len(stackNode.stackedPanes) == 0 {
		log.Printf("[workspace] updateStackVisibility aborted: stackedPanes empty, len=%d", len(stackNode.stackedPanes))
		return
	}

	activeIndex := stackNode.activeStackIndex
	if activeIndex < 0 || activeIndex >= len(stackNode.stackedPanes) {
		log.Printf("[workspace] updateStackVisibility: correcting activeIndex from %d to 0", activeIndex)
		activeIndex = 0
		stackNode.activeStackIndex = activeIndex
	}

	log.Printf("[workspace] updating stack visibility: activeIndex=%d stackSize=%d", activeIndex, len(stackNode.stackedPanes))

	// First, ensure the active pane is visible (prevents rendering gaps)
	activePane := stackNode.stackedPanes[activeIndex]
	webkit.WidgetSetVisible(activePane.container, true)
	webkit.WidgetSetVisible(activePane.titleBar, false)
	webkit.WidgetAddCSSClass(activePane.container, "stacked-pane-active")
	webkit.WidgetRemoveCSSClass(activePane.container, "stacked-pane-collapsed")

	// Then hide other panes and show their title bars
	for i, pane := range stackNode.stackedPanes {
		if i != activeIndex {
			webkit.WidgetSetVisible(pane.container, false)
			webkit.WidgetSetVisible(pane.titleBar, true)
			webkit.WidgetAddCSSClass(pane.container, "stacked-pane-collapsed")
			webkit.WidgetRemoveCSSClass(pane.container, "stacked-pane-active")
		}
	}
}

// NavigateStack handles navigation within a stacked pane container
func (spm *StackedPaneManager) NavigateStack(direction string) bool {
	if spm.wm.currentlyFocused == nil {
		return false
	}

	// Find the stack container this pane belongs to
	var stackNode *paneNode
	current := spm.wm.currentlyFocused

	// Check if current pane is directly in a stack
	if current.parent != nil && current.parent.isStacked {
		stackNode = current.parent
	} else {
		// Current pane might be the stack container itself if it was the first pane converted to stack
		if current.isStacked {
			stackNode = current
		}
	}

	if stackNode == nil || !stackNode.isStacked || len(stackNode.stackedPanes) <= 1 {
		return false // Not in a stack or stack has only one pane
	}

	// Use the stackNode's activeStackIndex instead of searching for current pane
	// This is more reliable than trying to match panes
	currentIndex := stackNode.activeStackIndex
	if currentIndex < 0 || currentIndex >= len(stackNode.stackedPanes) {
		log.Printf("[workspace] navigateStack: invalid activeStackIndex=%d, resetting to 0", currentIndex)
		currentIndex = 0
		stackNode.activeStackIndex = 0
	}

	// Calculate new index based on direction
	var newIndex int
	switch direction {
	case "up":
		newIndex = currentIndex - 1
		if newIndex < 0 {
			newIndex = len(stackNode.stackedPanes) - 1 // Wrap to last
		}
	case "down":
		newIndex = currentIndex + 1
		if newIndex >= len(stackNode.stackedPanes) {
			newIndex = 0 // Wrap to first
		}
	default:
		return false
	}

	if newIndex == currentIndex {
		return false // No change
	}

	// Update active stack index and visibility
	stackNode.activeStackIndex = newIndex
	spm.UpdateStackVisibility(stackNode)

	// Focus the new active pane
	newActivePane := stackNode.stackedPanes[newIndex]
	spm.wm.currentlyFocused = newActivePane
	spm.wm.focusManager.SetActivePane(newActivePane)

	log.Printf("[workspace] navigated stack: direction=%s from=%d to=%d stackSize=%d",
		direction, currentIndex, newIndex, len(stackNode.stackedPanes))
	return true
}

// createTitleBar creates a title bar widget for a pane in a stack
func (spm *StackedPaneManager) createTitleBar(pane *paneNode) uintptr {
	titleBox := webkit.NewBox(webkit.OrientationHorizontal, 8)
	if titleBox == 0 {
		log.Printf("[workspace] failed to create title box")
		return 0
	}

	webkit.WidgetAddCSSClass(titleBox, "stacked-pane-title")
	webkit.WidgetSetHExpand(titleBox, true)
	webkit.WidgetSetVExpand(titleBox, false)

	// Get the actual title from the WebView instead of hardcoding "New Tab"
	var titleText string
	if pane.pane != nil && pane.pane.webView != nil {
		titleText = pane.pane.webView.GetTitle()
		if titleText == "" {
			titleText = "New Tab" // Fallback only when title is actually empty
		}
	} else {
		titleText = "New Tab"
	}

	// Create title label with the actual title
	titleLabel := webkit.NewLabel(titleText)
	if titleLabel == 0 {
		log.Printf("[workspace] failed to create title label")
		return titleBox
	}

	webkit.WidgetAddCSSClass(titleLabel, "stacked-pane-title-text")
	webkit.LabelSetMaxWidthChars(titleLabel, 50)
	webkit.LabelSetEllipsize(titleLabel, webkit.EllipsizeEnd)

	webkit.BoxAppend(titleBox, titleLabel)
	webkit.WidgetShow(titleBox)
	webkit.WidgetShow(titleLabel)

	return titleBox
}

// UpdateTitleBar updates the title bar label for a WebView in stacked panes
func (spm *StackedPaneManager) UpdateTitleBar(webView *webkit.WebView, title string) {
	if spm.wm == nil || webView == nil || title == "" {
		return
	}

	// Find the pane node for this WebView
	node, exists := spm.wm.viewToNode[webView]
	if !exists || node == nil || !node.isLeaf {
		return
	}

	// Check if this pane is in a stack and has a title bar
	if node.parent != nil && node.parent.isStacked && node.titleBar != 0 {
		// Update the title bar label with the new title
		spm.updateTitleBarLabel(node, title)
		log.Printf("[workspace] updated title bar for WebView %s: %s", webView.ID(), title)
	}
}

// updateTitleBarLabel updates the label widget within a title bar by recreating it
func (spm *StackedPaneManager) updateTitleBarLabel(node *paneNode, title string) {
	if node == nil || node.titleBar == 0 || node.parent == nil || !node.parent.isStacked {
		return
	}

	// Format the title for display
	displayTitle := title
	if len(displayTitle) > 50 {
		displayTitle = displayTitle[:47] + "..."
	}

	// Create new title bar with updated title
	newTitleBar := spm.createTitleBarWithTitle(node, displayTitle)
	if newTitleBar == 0 {
		log.Printf("[workspace] failed to create new title bar")
		return
	}

	// Replace the old title bar in the stack
	if node.parent != nil && node.parent.isStacked && node.parent.stackWrapper != 0 {
		// Remove old title bar
		webkit.WidgetUnparent(node.titleBar)

		// Update the node's title bar reference
		node.titleBar = newTitleBar

		// Add new title bar to the stack (need to find correct position)
		webkit.BoxPrepend(node.parent.stackWrapper, newTitleBar)

		log.Printf("[workspace] replaced title bar for pane: %p", node)
	}
}

// createTitleBarWithTitle creates a title bar with a specific title
func (spm *StackedPaneManager) createTitleBarWithTitle(pane *paneNode, title string) uintptr {
	titleBox := webkit.NewBox(webkit.OrientationHorizontal, 8)
	if titleBox == 0 {
		return 0
	}

	webkit.WidgetAddCSSClass(titleBox, "stacked-pane-title")
	webkit.WidgetSetHExpand(titleBox, true)
	webkit.WidgetSetVExpand(titleBox, false)

	// Create title label with the specified title
	titleLabel := webkit.NewLabel(title)
	if titleLabel == 0 {
		return titleBox
	}

	webkit.WidgetAddCSSClass(titleLabel, "stacked-pane-title-text")
	webkit.LabelSetMaxWidthChars(titleLabel, 50)
	webkit.LabelSetEllipsize(titleLabel, webkit.EllipsizeEnd)

	webkit.BoxAppend(titleBox, titleLabel)
	webkit.WidgetShow(titleBox)
	webkit.WidgetShow(titleLabel)

	return titleBox
}

// CloseStackedPane handles closing a pane that is part of a stack
func (spm *StackedPaneManager) CloseStackedPane(node *paneNode) error {
	if node.parent == nil || !node.parent.isStacked {
		return errors.New("node is not part of a stacked pane")
	}

	stackNode := node.parent

	// Find the index of the node to be closed
	nodeIndex := -1
	for i, stackedPane := range stackNode.stackedPanes {
		if stackedPane == node {
			nodeIndex = i
			break
		}
	}

	if nodeIndex == -1 {
		return errors.New("node not found in stack")
	}

	log.Printf("[workspace] closing stacked pane: index=%d stackSize=%d", nodeIndex, len(stackNode.stackedPanes))

	// Clean up the pane
	spm.wm.detachHover(node)
	delete(spm.wm.viewToNode, node.pane.webView)

	// Remove from app.panes
	for i, pane := range spm.wm.app.panes {
		if pane == node.pane {
			spm.wm.app.panes = append(spm.wm.app.panes[:i], spm.wm.app.panes[i+1:]...)
			break
		}
	}

	// Remove the pane's widgets from the stack container
	stackBox := stackNode.stackWrapper
	if stackBox == 0 {
		stackBox = stackNode.container
	}

	if node.titleBar != 0 {
		webkit.BoxRemove(stackBox, node.titleBar)
	}
	if node.container != 0 {
		webkit.BoxRemove(stackBox, node.container)
	}

	// Clean up the pane
	node.pane.Cleanup()

	// Remove from the stack
	stackNode.stackedPanes = append(stackNode.stackedPanes[:nodeIndex], stackNode.stackedPanes[nodeIndex+1:]...)

	// Handle the remaining panes in the stack
	if len(stackNode.stackedPanes) == 0 {
		// No panes left in stack - this should not happen normally
		log.Printf("[workspace] warning: empty stack after closing pane")
		return errors.New("stack became empty")
	} else if len(stackNode.stackedPanes) == 1 {
		// Only one pane left - convert back to a regular pane
		lastPane := stackNode.stackedPanes[0]
		parent := stackNode.parent
		lastPaneContainer := lastPane.container

		// Remove the title bar since we're going back to single pane
		if lastPane.titleBar != 0 {
			webkit.BoxRemove(stackBox, lastPane.titleBar)
		}

		// Unparent the pane container from stack wrapper
		webkit.BoxRemove(stackBox, lastPaneContainer)

		// CRITICAL FIX: Replace the stackNode completely with lastPane
		// Update parent child references FIRST
		if parent == nil {
			spm.wm.root = lastPane
			if spm.wm.window != nil {
				spm.wm.window.SetChild(lastPaneContainer)
			}
		} else {
			if parent.left == stackNode {
				parent.left = lastPane
				webkit.PanedSetStartChild(parent.container, lastPaneContainer)
			} else if parent.right == stackNode {
				parent.right = lastPane
				webkit.PanedSetEndChild(parent.container, lastPaneContainer)
			}
		}

		// Convert lastPane back to regular pane
		lastPane.parent = parent
		lastPane.isStacked = false
		lastPane.stackedPanes = nil
		lastPane.titleBar = 0

		// Update the viewToNode mapping to point to the lastPane, not stackNode
		spm.wm.viewToNode[lastPane.pane.webView] = lastPane

		// Focus the remaining pane if it was the active one being closed
		if spm.wm.currentlyFocused == node {
			spm.wm.currentlyFocused = lastPane
			spm.wm.focusManager.SetActivePane(lastPane)
		}

		log.Printf("[workspace] converted single-pane stack back to regular pane")
	} else {
		// Multiple panes remain in stack - adjust active index
		if stackNode.activeStackIndex >= nodeIndex && stackNode.activeStackIndex > 0 {
			stackNode.activeStackIndex--
		}
		if stackNode.activeStackIndex >= len(stackNode.stackedPanes) {
			stackNode.activeStackIndex = len(stackNode.stackedPanes) - 1
		}

		// Update visibility for remaining panes
		spm.UpdateStackVisibility(stackNode)

		// Focus the new active pane if we closed the currently active one
		if spm.wm.currentlyFocused == node {
			newActivePaneInStack := stackNode.stackedPanes[stackNode.activeStackIndex]
			spm.wm.currentlyFocused = newActivePaneInStack
			spm.wm.focusManager.SetActivePane(newActivePaneInStack)
		}

		log.Printf("[workspace] closed pane from stack: remaining=%d activeIndex=%d",
			len(stackNode.stackedPanes), stackNode.activeStackIndex)
	}

	// Update CSS classes after pane count changes
	spm.wm.ensurePaneBaseClasses()
	log.Printf("[workspace] stacked pane closed; panes remaining=%d", len(spm.wm.app.panes))

	return nil
}
