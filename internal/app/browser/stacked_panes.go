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
		return nil, fmt.Errorf("failed to prepare new stacked pane: %w", err)
	}

	var stackNode *paneNode
	var insertIndex int

	// Check if target is already in a stack
	if target.parent != nil && target.parent.isStacked {
		// Target is already in a stack - add to existing stack
		stackNode, insertIndex, err = spm.addPaneToExistingStack(target, newLeaf)
		if err != nil {
			return nil, fmt.Errorf("failed to add pane to existing stack: %w", err)
		}
	} else {
		// Target is not stacked - create initial stack
		stackNode, insertIndex, err = spm.convertToStackedContainer(target, newLeaf)
		if err != nil {
			return nil, fmt.Errorf("failed to convert to stacked container: %w", err)
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
		pane:   newPane,
		isLeaf: true,
	}
	// Initialize widgets properly using workspace manager helper
	spm.wm.initializePaneWidgets(newLeaf, newContainer)

	// Create title bar for the new pane
	newTitleBar := spm.createTitleBar(newLeaf)
	spm.wm.setTitleBar(newLeaf, newTitleBar)

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
	if newLeaf.container != nil {
		newLeaf.container.Execute(func(containerPtr uintptr) error {
			if webkit.WidgetGetParent(containerPtr) != 0 {
				webkit.WidgetUnparent(containerPtr)
			}
			return nil
		})
	}

	// Insert widgets at correct position for Zellij-style layout
	// Each pane has 2 widgets (titleBar, container), so position = insertIndex * 2

	// Find the widget to insert after (based on insertIndex)
	var insertAfterWidget uintptr = 0
	if insertIndex > 0 {
		// Insert after the previous pane's container
		prevPane := stackNode.stackedPanes[insertIndex-1]
		if prevPane.container != nil {
			insertAfterWidget = prevPane.container.Ptr()
		}
	}

	if insertAfterWidget != 0 {
		// Insert titleBar after the previous pane's container
		if stackNode.stackWrapper != nil && newLeaf.titleBar != nil {
			webkit.BoxInsertChildAfter(stackNode.stackWrapper.Ptr(), newLeaf.titleBar.Ptr(), insertAfterWidget)
		}
		// Insert container after the newly inserted titleBar
		if stackNode.stackWrapper != nil && newLeaf.container != nil && newLeaf.titleBar != nil {
			webkit.BoxInsertChildAfter(stackNode.stackWrapper.Ptr(), newLeaf.container.Ptr(), newLeaf.titleBar.Ptr())
		}
		log.Printf("[workspace] inserted widgets at position %d (after widget %#x)", insertIndex, insertAfterWidget)
	} else {
		// Insert at the beginning (insertIndex = 0)
		if stackNode.stackWrapper != nil && newLeaf.container != nil {
			webkit.BoxPrepend(stackNode.stackWrapper.Ptr(), newLeaf.container.Ptr())
		}
		if stackNode.stackWrapper != nil && newLeaf.titleBar != nil {
			webkit.BoxPrepend(stackNode.stackWrapper.Ptr(), newLeaf.titleBar.Ptr())
		}
		log.Printf("[workspace] prepended widgets at position 0")
	}

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
	webkit.WidgetRealizeInContainer(stackWrapperContainer) // Ensures size request is set like regular panes

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
	if existingContainer != nil {
		webkit.BoxAppend(stackInternalBox, existingContainer.Ptr())
	}

	// Immediately reattach the stack wrapper to minimize visibility gap
	spm.reattachToParent(stackWrapperContainer, target, parent)

	// Convert target to a stacked leaf node
	target.isStacked = true
	target.isLeaf = true
	// container stays the same (existingContainer)
	spm.wm.setTitleBar(target, titleBar)

	// Create the stack container node - container points to wrapper, stackWrapper points to internal box
	stackNode := &paneNode{
		isStacked:        true,
		isLeaf:           false,
		stackedPanes:     []*paneNode{target},
		activeStackIndex: 0, // KEEP CURRENT PANE ACTIVE during transition (index 0)
		parent:           parent,
	}
	spm.wm.setContainer(stackNode, stackWrapperContainer, "stack-wrapper") // Wrapper for GTK operations (splits, etc.)
	spm.wm.setStackWrapper(stackNode, stackInternalBox)                     // Internal box for stack operations

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
	if newLeaf.container != nil {
		newLeaf.container.Execute(func(containerPtr uintptr) error {
			if webkit.WidgetGetParent(containerPtr) != 0 {
				webkit.WidgetUnparent(containerPtr)
			}
			return nil
		})
	}

	// Add the new widgets to the internal stack box
	if stackNode.stackWrapper != nil && newLeaf.titleBar != nil {
		webkit.BoxAppend(stackNode.stackWrapper.Ptr(), newLeaf.titleBar.Ptr())
	}
	if stackNode.stackWrapper != nil && newLeaf.container != nil {
		webkit.BoxAppend(stackNode.stackWrapper.Ptr(), newLeaf.container.Ptr())
	}

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
		if target.container != nil {
			target.container.Execute(func(containerPtr uintptr) error {
				if webkit.WidgetGetParent(containerPtr) != 0 {
					webkit.WidgetUnparent(containerPtr)
				}
				return nil
			})
		}
	} else if parent.container != nil {
		// Target has a parent paned - remove it (automatically unparents in GTK4)
		parent.container.Execute(func(panedPtr uintptr) error {
			if parent.left == target {
				webkit.PanedSetStartChild(panedPtr, 0)
			} else if parent.right == target {
				webkit.PanedSetEndChild(panedPtr, 0)
			}
			return nil
		})
	}
}

// reattachToParent attaches a container to the parent
func (spm *StackedPaneManager) reattachToParent(container uintptr, target *paneNode, parent *paneNode) {
	if parent == nil {
		if spm.wm.window != nil {
			spm.wm.window.SetChild(container)
		}
	} else if parent.container != nil {
		parent.container.Execute(func(panedPtr uintptr) error {
			if parent.left == target {
				webkit.PanedSetStartChild(panedPtr, container)
			} else if parent.right == target {
				webkit.PanedSetEndChild(panedPtr, container)
			}
			return nil
		})
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

	// This ensures Zellij-style layout where the previously active pane shows its current title
	currentActiveIndex := stackNode.activeStackIndex
	if currentActiveIndex >= 0 && currentActiveIndex < len(stackNode.stackedPanes) && currentActiveIndex != insertIndex {
		currentlyActivePane := stackNode.stackedPanes[currentActiveIndex]
		if currentlyActivePane.pane != nil && currentlyActivePane.pane.webView != nil && currentlyActivePane.titleBar != nil {
			currentTitle := currentlyActivePane.pane.webView.GetTitle()
			if currentTitle != "" {
				spm.updateTitleBarLabel(currentlyActivePane, currentTitle)
				log.Printf("[workspace] updated title bar for pane becoming inactive during stack creation: %s", currentTitle)
			}
		}
	}

	// Immediately switch to the new pane (don't use deprecated IdleAdd)
	stackNode.activeStackIndex = insertIndex
	spm.UpdateStackVisibility(stackNode)

	// Set focus on the new pane synchronously
	spm.wm.focusManager.SetActivePane(newLeaf)
	spm.wm.currentlyFocused = newLeaf

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

	// CRITICAL: Process ALL panes in a single pass to prevent flickering
	for i, pane := range stackNode.stackedPanes {
		if i == activeIndex {
			// Active pane: show container, NEVER show title bar
			if pane.container != nil {
				webkit.WidgetSetVisible(pane.container.Ptr(), true)
				webkit.WidgetAddCSSClass(pane.container.Ptr(), "stacked-pane-active")
				webkit.WidgetRemoveCSSClass(pane.container.Ptr(), "stacked-pane-collapsed")
			}
			if pane.titleBar != nil {
				webkit.WidgetSetVisible(pane.titleBar.Ptr(), false) // ABSOLUTE RULE: never visible for active pane
			}
			log.Printf("[workspace] active pane %d: container=visible, titleBar=HIDDEN", i)
		} else {
			// Inactive panes: hide container, show title bar
			if pane.container != nil {
				webkit.WidgetSetVisible(pane.container.Ptr(), false)
				webkit.WidgetAddCSSClass(pane.container.Ptr(), "stacked-pane-collapsed")
			}
			if pane.titleBar != nil {
				webkit.WidgetSetVisible(pane.titleBar.Ptr(), true)
			}
			if pane.container != nil {
				webkit.WidgetRemoveCSSClass(pane.container.Ptr(), "stacked-pane-active")
			}
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

	// Update title bar for the pane transitioning from ACTIVE to INACTIVE
	// This ensures the collapsed pane shows its current page title (Zellij-style layout)
	currentActivePane := stackNode.stackedPanes[currentIndex]
	if currentActivePane.pane != nil && currentActivePane.pane.webView != nil && currentActivePane.titleBar != nil {
		currentTitle := currentActivePane.pane.webView.GetTitle()
		if currentTitle != "" {
			spm.updateTitleBarLabel(currentActivePane, currentTitle)
			log.Printf("[workspace] updated title bar for pane transitioning to INACTIVE: %s", currentTitle)
		}
	}

	// Update active stack index and visibility
	stackNode.activeStackIndex = newIndex
	spm.UpdateStackVisibility(stackNode)

	// Focus the new active pane
	newActivePane := stackNode.stackedPanes[newIndex]
	spm.wm.focusManager.SetActivePane(newActivePane)
	spm.wm.currentlyFocused = newActivePane

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
	if node.parent != nil && node.parent.isStacked && node.titleBar != nil {
		// CRITICAL: Only update title bar for INACTIVE panes
		// Active panes should never show their title bar
		stackNode := node.parent
		activeIndex := stackNode.activeStackIndex

		// Find this pane's index in the stack
		paneIndex := -1
		for i, pane := range stackNode.stackedPanes {
			if pane == node {
				paneIndex = i
				break
			}
		}

		if paneIndex != -1 && paneIndex != activeIndex {
			// This is an INACTIVE pane - safe to update title bar
			spm.updateTitleBarLabel(node, title)
			log.Printf("[workspace] updated title bar for INACTIVE WebView %s: %s", webView.ID(), title)
		} else {
			// This is the ACTIVE pane - title bar should remain hidden
			log.Printf("[workspace] skipped title bar update for ACTIVE WebView %s: %s", webView.ID(), title)
		}
	}
}

// updateTitleBarLabel updates the label widget within a title bar by recreating it
func (spm *StackedPaneManager) updateTitleBarLabel(node *paneNode, title string) {
	if node == nil || node.titleBar == nil || node.parent == nil || !node.parent.isStacked {
		return
	}

	// Format the title for display
	displayTitle := title
	if len(displayTitle) > 50 {
		displayTitle = displayTitle[:47] + "..."
	}

	// Create new title bar with updated title
	newTitleBar := spm.createTitleBarWithTitle(displayTitle)
	if newTitleBar == 0 {
		log.Printf("[workspace] failed to create new title bar")
		return
	}

	// Replace the old title bar in the stack
	if node.parent != nil && node.parent.isStacked && node.parent.stackWrapper != nil {
		// Find the correct insertion position based on the pane's index in the stack
		paneIndex := -1
		for i, stackedPane := range node.parent.stackedPanes {
			if stackedPane == node {
				paneIndex = i
				break
			}
		}

		// Remove old title bar
		if node.titleBar != nil {
			node.titleBar.Execute(func(titleBarPtr uintptr) error {
				webkit.WidgetUnparent(titleBarPtr)
				return nil
			})
		}

		// Update the node's title bar reference
		spm.wm.setTitleBar(node, newTitleBar)

		// Insert the new title bar at the correct position
		if paneIndex == 0 {
			// First pane - insert at the beginning
			if node.parent.stackWrapper != nil {
				webkit.BoxPrepend(node.parent.stackWrapper.Ptr(), newTitleBar)
			}
		} else {
			// Insert after the previous pane's container
			prevPane := node.parent.stackedPanes[paneIndex-1]
			if node.parent.stackWrapper != nil && prevPane.container != nil {
				webkit.BoxInsertChildAfter(node.parent.stackWrapper.Ptr(), newTitleBar, prevPane.container.Ptr())
			}
		}

		log.Printf("[workspace] replaced title bar for pane %d: %p", paneIndex, node)
	}
}

// createTitleBarWithTitle creates a title bar with a specific title
func (spm *StackedPaneManager) createTitleBarWithTitle(title string) uintptr {
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
	node.pane.CleanupFromWorkspace(spm.wm)

	// Remove the pane's widgets from the stack container
	var stackBox *SafeWidget = stackNode.stackWrapper
	if stackBox == nil {
		stackBox = stackNode.container
	}

	if stackBox != nil {
		if node.titleBar != nil {
			webkit.BoxRemove(stackBox.Ptr(), node.titleBar.Ptr())
		}
		if node.container != nil {
			webkit.BoxRemove(stackBox.Ptr(), node.container.Ptr())
		}
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
		if lastPane.titleBar != nil && stackBox != nil {
			webkit.BoxRemove(stackBox.Ptr(), lastPane.titleBar.Ptr())
		}

		// Unparent the pane container from stack wrapper
		if stackBox != nil && lastPaneContainer != nil {
			webkit.BoxRemove(stackBox.Ptr(), lastPaneContainer.Ptr())
		}

		// CRITICAL FIX: Replace the stackNode completely with lastPane
		// Update parent child references FIRST
		if parent == nil {
			spm.wm.root = lastPane
			if spm.wm.window != nil && lastPaneContainer != nil {
				spm.wm.window.SetChild(lastPaneContainer.Ptr())
			}
		} else {
			if parent.left == stackNode {
				parent.left = lastPane
				if parent.container != nil && lastPaneContainer != nil {
					parent.container.Execute(func(panedPtr uintptr) error {
						webkit.PanedSetStartChild(panedPtr, lastPaneContainer.Ptr())
						return nil
					})
				}
			} else if parent.right == stackNode {
				parent.right = lastPane
				if parent.container != nil && lastPaneContainer != nil {
					parent.container.Execute(func(panedPtr uintptr) error {
						webkit.PanedSetEndChild(panedPtr, lastPaneContainer.Ptr())
						return nil
					})
				}
			}
		}

		// Convert lastPane back to regular pane
		lastPane.parent = parent
		lastPane.isStacked = false
		lastPane.stackedPanes = nil
		lastPane.titleBar = nil

		// CRITICAL: Ensure the container is visible after reparenting
		// The container may have been hidden during stack operations
		if lastPaneContainer != nil {
			webkit.WidgetSetVisible(lastPaneContainer.Ptr(), true)
		}

		// Update the viewToNode mapping to point to the lastPane, not stackNode
		spm.wm.viewToNode[lastPane.pane.webView] = lastPane

		// FIXED: Focus the remaining pane if any pane from this stack was currently focused
		// Check if currentlyFocused is part of this stack (including the pane being closed)
		shouldFocus := false
		if spm.wm.currentlyFocused != nil {
			log.Printf("[workspace] DEBUG: currentlyFocused=%p, node being closed=%p, lastPane=%p",
				spm.wm.currentlyFocused, node, lastPane)
			// Check if currentlyFocused is the pane being closed
			if spm.wm.currentlyFocused == node {
				shouldFocus = true
				log.Printf("[workspace] DEBUG: shouldFocus=true (closing focused pane)")
			} else if spm.wm.currentlyFocused.parent == stackNode {
				// Check if currentlyFocused is another pane in this stack
				shouldFocus = true
				log.Printf("[workspace] DEBUG: shouldFocus=true (focused pane in same stack)")
			}
		}

		if shouldFocus {
			log.Printf("[workspace] focusing remaining pane after stack conversion")

			// Log widget state before focus operations
			if lastPaneWidget := lastPane.pane.webView.Widget(); lastPaneWidget != 0 {
				visible := webkit.WidgetGetVisible(lastPaneWidget)
				log.Printf("[workspace] Widget state before focus: widget=%#x visible=%v",
					lastPaneWidget, visible)
			}

			// Update CSS classes (mirror finalizeStackCreation)
			spm.wm.ensurePaneBaseClasses()

			// Mirror the exact logic from finalizeStackCreation that works perfectly:
			// 1. First call SetActivePane (this does all the focus work)
			// 2. Then set currentlyFocused
			spm.wm.focusManager.SetActivePane(lastPane)
			spm.wm.currentlyFocused = lastPane

			// CRITICAL: Ensure widget visibility and focus after reparenting
			// The widget may need explicit show/focus calls after being reparented
			if lastPaneWidget := lastPane.pane.webView.Widget(); lastPaneWidget != 0 {
				log.Printf("[workspace] Ensuring widget visibility after reparenting")
				webkit.WidgetShow(lastPaneWidget)
				webkit.WidgetGrabFocus(lastPaneWidget)

				// Log widget state after focus operations
				visible := webkit.WidgetGetVisible(lastPaneWidget)
				log.Printf("[workspace] Widget state after focus: widget=%#x visible=%v",
					lastPaneWidget, visible)
			}

			log.Printf("[workspace] DEBUG: applied finalizeStackCreation focus logic in reverse")
		} else {
			log.Printf("[workspace] DEBUG: not focusing remaining pane (shouldFocus=false)")
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
