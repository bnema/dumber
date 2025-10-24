package browser

import (
	"errors"
	"fmt"
	"log"
	"sync/atomic"
	"time"

	"github.com/bnema/dumber/pkg/webkit"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

const (
	// faviconPlaceholder is the emoji displayed when no favicon is available
	faviconPlaceholder = "🌐"
)

// StackedPaneManager handles all stacked pane operations
type StackedPaneManager struct {
	wm             *WorkspaceManager
	titleBarToPane map[uint64]*paneNode
	nextTitleBarID uint64
}

// NewStackedPaneManager creates a new stacked pane manager
func NewStackedPaneManager(wm *WorkspaceManager) *StackedPaneManager {
	return &StackedPaneManager{
		wm:             wm,
		titleBarToPane: make(map[uint64]*paneNode),
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
	if newContainer == nil {
		return nil, errors.New("new pane missing container")
	}
	webkit.WidgetSetHExpand(newContainer, true)
	webkit.WidgetSetVExpand(newContainer, true)

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
	if newTitleBar != nil {
		webkit.WidgetSetVisible(newTitleBar, false)
	}

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
		if webkit.WidgetGetParent(newLeaf.container) != nil {
			webkit.WidgetUnparent(newLeaf.container)
		}
	}

	// Insert widgets at correct position for Zellij-style layout
	// Each pane has 2 widgets (titleBar, container), so position = insertIndex * 2

	// Find the widget to insert after (based on insertIndex)
	var insertAfterWidget gtk.Widgetter
	if insertIndex > 0 {
		// Insert after the previous pane's container
		prevPane := stackNode.stackedPanes[insertIndex-1]
		if prevPane.container != nil {
			insertAfterWidget = prevPane.container
		}
	}

	if insertAfterWidget != nil {
		// Insert titleBar after the previous pane's container
		if box, ok := stackNode.stackWrapper.(*gtk.Box); ok && newLeaf.titleBar != nil {
			box.InsertChildAfter(newLeaf.titleBar, insertAfterWidget)
		}
		// Insert container after the newly inserted titleBar
		if box, ok := stackNode.stackWrapper.(*gtk.Box); ok && newLeaf.container != nil && newLeaf.titleBar != nil {
			box.InsertChildAfter(newLeaf.container, newLeaf.titleBar)
		}
		log.Printf("[workspace] inserted widgets at position %d (after widget %p)", insertIndex, insertAfterWidget)
	} else {
		// Insert at the beginning (insertIndex = 0)
		if box, ok := stackNode.stackWrapper.(*gtk.Box); ok && newLeaf.container != nil {
			box.Prepend(newLeaf.container)
		}
		if box, ok := stackNode.stackWrapper.(*gtk.Box); ok && newLeaf.titleBar != nil {
			box.Prepend(newLeaf.titleBar)
		}
		log.Printf("[workspace] prepended widgets at position 0")
	}

	return stackNode, insertIndex, nil
}

// convertToStackedContainer converts a simple pane to a stacked container
func (spm *StackedPaneManager) convertToStackedContainer(target, newLeaf *paneNode) (*paneNode, int, error) {
	log.Printf("[workspace] converting pane to stacked: %p", target)

	// Create the wrapper container - this is what will be used by splitNode
	stackWrapperContainer := gtk.NewBox(gtk.OrientationVertical, 0)
	if stackWrapperContainer == nil {
		return nil, 0, errors.New("failed to create stack wrapper container")
	}
	stackWrapperContainer.SetHExpand(true)
	stackWrapperContainer.SetVExpand(true)
	// Note: Don't call Realize() here - GTK4 will realize automatically when added to toplevel

	// Add CSS class to the stack container for proper styling
	stackWrapperContainer.AddCSSClass(stackContainerClass)

	// Create the internal box for the actual stacked widgets (titles + webviews)
	stackInternalBox := gtk.NewBox(gtk.OrientationVertical, 0)
	if stackInternalBox == nil {
		return nil, 0, errors.New("failed to create stack internal box")
	}
	stackInternalBox.SetHExpand(true)
	stackInternalBox.SetVExpand(true)

	// The internal box goes inside the wrapper
	stackWrapperContainer.Append(stackInternalBox)

	// Get the existing container and parent info
	existingContainer := target.container
	parent := target.parent

	// Create title bar for the existing pane
	titleBar := spm.createTitleBar(target)

	// Keep existing container visible during transition to prevent rendering glitch
	if titleBar != nil {
		webkit.WidgetSetVisible(titleBar, false)
	}

	// Detach existing container from its current parent first
	spm.detachFromParent(target, parent)

	// Build the complete stack structure with hidden widgets
	if titleBar != nil {
		stackInternalBox.Append(titleBar)
	}
	if existingContainer != nil {
		stackInternalBox.Append(existingContainer)
	}

	// Immediately reattach the stack wrapper to minimize visibility gap
	spm.reattachToParent(stackWrapperContainer, target, parent)

	// Keep target as a regular leaf node (it's just inside a stack now)
	// Individual panes within a stack should NOT be marked as isStacked
	// Only the stack container itself has isStacked=true
	target.isStacked = false
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
	spm.wm.setStackWrapper(stackNode, stackInternalBox)                    // Internal box for stack operations

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
	// This is critical: even freshly created WebViews might have internal parent refs
	if newLeaf.container != nil {
		parentWidget := webkit.WidgetGetParent(newLeaf.container)
		if parentWidget != nil {
			log.Printf("[workspace] unparenting new pane container %p from parent %p before stack append", newLeaf.container, parentWidget)
			webkit.WidgetUnparent(newLeaf.container)
			// Verify unparent succeeded
			if finalParent := webkit.WidgetGetParent(newLeaf.container); finalParent != nil {
				log.Printf("[workspace] WARNING: container %p still has parent %p after unparent", newLeaf.container, finalParent)
			}
		}
	}

	// Add the new widgets to the internal stack box
	if box, ok := stackNode.stackWrapper.(*gtk.Box); ok && newLeaf.titleBar != nil {
		box.Append(newLeaf.titleBar)
	}
	if box, ok := stackNode.stackWrapper.(*gtk.Box); ok && newLeaf.container != nil {
		box.Append(newLeaf.container)
	}

	return stackNode, insertIndex, nil
}

// detachFromParent removes a pane from its current parent
func (spm *StackedPaneManager) detachFromParent(target *paneNode, parent *paneNode) {
	if parent == nil {
		// Target is the root - remove it from window
		if spm.wm.window != nil {
			spm.wm.window.SetChild(nil)
		}
		// Unparent if it has a GTK parent
		if target.container != nil {
			if webkit.WidgetGetParent(target.container) != nil {
				webkit.WidgetUnparent(target.container)
			}
		}
	} else if paned, ok := parent.container.(*gtk.Paned); ok && paned != nil {
		// Target has a parent paned - remove it (automatically unparents in GTK4)
		if parent.left == target {
			paned.SetStartChild(nil)
		} else if parent.right == target {
			paned.SetEndChild(nil)
		}
	}
}

// reattachToParent attaches a container to the parent
func (spm *StackedPaneManager) reattachToParent(container gtk.Widgetter, target *paneNode, parent *paneNode) {
	if parent == nil {
		if spm.wm.window != nil {
			spm.wm.window.SetChild(container)
		}
	} else if paned, ok := parent.container.(*gtk.Paned); ok && paned != nil {
		if parent.left == target {
			paned.SetStartChild(container)
		} else if parent.right == target {
			paned.SetEndChild(container)
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
	spm.wm.SetActivePane(newLeaf, SourceSplit)

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
				webkit.WidgetSetVisible(pane.container, true)
			}
			if pane.titleBar != nil {
				webkit.WidgetSetVisible(pane.titleBar, false) // ABSOLUTE RULE: never visible for active pane
			}
			log.Printf("[workspace] active pane %d: container=visible, titleBar=HIDDEN", i)
		} else {
			// Inactive panes: hide container, show title bar
			if pane.container != nil {
				webkit.WidgetSetVisible(pane.container, false)
			}
			if pane.titleBar != nil {
				webkit.WidgetSetVisible(pane.titleBar, true)

				// Refresh title bar to pick up any newly downloaded favicons
				if pane.pane != nil && pane.pane.webView != nil {
					title := pane.pane.webView.GetTitle()
					if title == "" {
						title = NewTabTitle
					}
					spm.updateTitleBarLabel(pane, title)
				}
			}
		}
	}
}

// NavigateStack handles navigation within a stacked pane container
func (spm *StackedPaneManager) NavigateStack(direction string) bool {
	if spm == nil || spm.wm == nil || spm.wm.GetActiveNode() == nil {
		return false
	}

	// Find the stack container this pane belongs to
	var stackNode *paneNode
	current := spm.wm.GetActiveNode()

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
	case DirectionUp:
		newIndex = currentIndex - 1
		if newIndex < 0 {
			newIndex = len(stackNode.stackedPanes) - 1 // Wrap to last
		}
	case DirectionDown:
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
	spm.wm.SetActivePane(newActivePane, SourceStackNav)

	log.Printf("[workspace] navigated stack: direction=%s from=%d to=%d stackSize=%d",
		direction, currentIndex, newIndex, len(stackNode.stackedPanes))
	return true
}

// createTitleBar creates a title bar widget for a pane in a stack
func (spm *StackedPaneManager) createTitleBar(pane *paneNode) gtk.Widgetter {
	// Get the actual title and URL from the WebView
	var titleText, pageURL string
	if pane.pane != nil && pane.pane.webView != nil {
		titleText = pane.pane.webView.GetTitle()
		if titleText == "" {
			titleText = NewTabTitle // Fallback only when title is actually empty
		}
		pageURL = pane.pane.webView.GetCurrentURL()
	} else {
		titleText = NewTabTitle
	}

	titleBar := spm.createTitleBarWithTitle(titleText, pageURL)
	if titleBar != nil {
		// Store the mapping from titleBar ID to pane
		titleBarID := atomic.AddUint64(&spm.nextTitleBarID, 1)
		spm.titleBarToPane[titleBarID] = pane

		// Attach click handler
		// TODO: Implement click handler with gtk.GestureClick
		// webkit.WidgetAttachClickHandler(titleBar, func() {
		// 	spm.handleTitleBarClick(titleBarID)
		// })
	}

	return titleBar
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
			log.Printf("[workspace] updated title bar for INACTIVE WebView %d: %s", webView.ID(), title)
		} else {
			// This is the ACTIVE pane - title bar should remain hidden
			log.Printf("[workspace] skipped title bar update for ACTIVE WebView %d: %s", webView.ID(), title)
		}
	}
}

// updateTitleBarLabel updates the label widget within a title bar by recreating it
func (spm *StackedPaneManager) updateTitleBarLabel(node *paneNode, title string) {
	if node == nil || node.titleBar == nil || node.parent == nil || !node.parent.isStacked {
		return
	}

	// Get the current URL from the WebView for favicon
	var pageURL string
	if node.pane != nil && node.pane.webView != nil {
		pageURL = node.pane.webView.GetCurrentURL()
	}

	// Create new title bar with updated title and favicon
	newTitleBar := spm.createTitleBarWithTitle(title, pageURL)
	if newTitleBar == nil {
		log.Printf("[workspace] failed to create new title bar")
		return
	}

	// Store the mapping from titleBar ID to pane and attach click handler
	titleBarID := atomic.AddUint64(&spm.nextTitleBarID, 1)
	spm.titleBarToPane[titleBarID] = node

	// Attach click handler
	// TODO: Implement with gtk.GestureClick
	// webkit.WidgetAttachClickHandler(newTitleBar, func() {
	// 	spm.handleTitleBarClick(titleBarID)
	// })

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
			webkit.WidgetUnparent(node.titleBar)
		}

		// Update the node's title bar reference
		spm.wm.setTitleBar(node, newTitleBar)

		// Insert the new title bar at the correct position
		if box, ok := node.parent.stackWrapper.(*gtk.Box); ok {
			if paneIndex == 0 {
				// First pane - insert at the beginning
				box.Prepend(newTitleBar)
			} else {
				// Insert after the previous pane's container
				prevPane := node.parent.stackedPanes[paneIndex-1]
				if prevPane.container != nil {
					box.InsertChildAfter(newTitleBar, prevPane.container)
				}
			}
		}

		log.Printf("[workspace] replaced title bar for pane %d: %p", paneIndex, node)
	}
}

// createTitleBarWithTitle creates a title bar with a specific title and optional favicon
func (spm *StackedPaneManager) createTitleBarWithTitle(title string, pageURL string) gtk.Widgetter {
	titleBox := gtk.NewBox(gtk.OrientationHorizontal, 8)
	if titleBox == nil {
		return nil
	}

	titleBox.AddCSSClass("stacked-pane-title")
	titleBox.SetHExpand(true)
	titleBox.SetVExpand(false)

	placeholderLabel := gtk.NewLabel(faviconPlaceholder)
	if placeholderLabel != nil {
		placeholderLabel.AddCSSClass("stacked-pane-favicon-placeholder")
		titleBox.Append(placeholderLabel)
		webkit.WidgetSetVisible(placeholderLabel, true)
	}

	if pageURL != "" && spm.wm != nil && spm.wm.app != nil && spm.wm.app.faviconService != nil {
		spm.wm.app.faviconService.GetFaviconTexture(pageURL, func(texture *gdk.Texture, err error) {
			if err != nil {
				log.Printf("[favicon] Failed to load favicon for title bar: %s - %v", pageURL, err)
				return
			}

			if texture != nil && titleBox != nil {
				log.Printf("[favicon] Favicon ready to display in title bar for: %s", pageURL)

				// Create favicon image
				faviconImg := gtk.NewImageFromPaintable(texture)
				faviconImg.SetPixelSize(16)
				webkit.WidgetAddCSSClass(faviconImg, "stacked-pane-favicon")

				// IDEMPOTENT: Remove placeholder only if it still has titleBox as parent
				// This handles the case where multiple async callbacks fire for the same title bar
				if placeholderLabel != nil && webkit.WidgetGetParent(placeholderLabel) != nil {
					titleBox.Remove(placeholderLabel)
				}

				// Prepend favicon image (multiple calls are safe - GTK handles it)
				titleBox.Prepend(faviconImg)
				webkit.WidgetSetVisible(faviconImg, true)
			}
		})
	}

	// Create title label with the specified title
	titleLabel := gtk.NewLabel(title)
	if titleLabel == nil {
		return titleBox
	}

	titleLabel.AddCSSClass("stacked-pane-title-text")
	titleLabel.SetEllipsize(2)

	titleBox.Append(titleLabel)
	webkit.WidgetSetVisible(titleBox, true)
	webkit.WidgetSetVisible(titleLabel, true)

	return titleBox
}

// TODO(#8): Refactor this function to reduce cyclomatic complexity (49 -> <30)
//
// CloseStackedPane handles closing a pane that is part of a stack.
// The logic mirrors the regular close path: we rebuild the tree, update focus,
// and let WorkspaceManager perform the actual cleanup/destroy work so that all
// bookkeeping stays consistent.
func (spm *StackedPaneManager) CloseStackedPane(node *paneNode) error {
	if node == nil || node.parent == nil || !node.parent.isStacked {
		return errors.New("node is not part of a stacked pane")
	}

	stackNode := node.parent

	// Locate the pane inside the stack slice.
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

	// Detach hover/focus controllers before tearing down widgets to avoid GTK
	// callbacks referencing freed memory during destruction.
	if spm.wm != nil {
		spm.wm.detachHover(node)
		spm.wm.detachFocus(node)
	}

	// Remove the pane widgets from the stack container before mutating the tree.
	stackBox := stackNode.stackWrapper
	if stackBox == nil {
		stackBox = stackNode.container
	}
	if box, ok := stackBox.(*gtk.Box); ok && box != nil {
		if node.titleBar != nil {
			box.Remove(node.titleBar)
		}
		if node.container != nil {
			box.Remove(node.container)
		}
	}

	// Remove the pane from the stack slice.
	stackNode.stackedPanes = append(stackNode.stackedPanes[:nodeIndex], stackNode.stackedPanes[nodeIndex+1:]...)

	switch len(stackNode.stackedPanes) {
	case 0:
		// Stack container is now empty: remove it from the layout entirely.
		parent := stackNode.parent
		if parent == nil {
			// Stack was the root of the workspace — closing this pane exits the app.
			if stackNode.container != nil {
				if webkit.WidgetGetParent(stackNode.container) != nil {
					webkit.WidgetUnparent(stackNode.container)
				}
			}
			if spm.wm.window != nil {
				spm.wm.window.SetChild(nil)
			}
			_, err := spm.wm.cleanupAndExit(node)
			return err
		}

		// Detach the empty stack container from its GtkPaned parent.
		if paned, ok := parent.container.(*gtk.Paned); ok && paned != nil {
			if parent.left == stackNode {
				paned.SetStartChild(nil)
			} else if parent.right == stackNode {
				paned.SetEndChild(nil)
			}
			webkit.WidgetQueueAllocate(paned)
		}
		if stackNode.container != nil {
			if webkit.WidgetGetParent(stackNode.container) != nil {
				webkit.WidgetUnparent(stackNode.container)
			}
		}

		sibling := spm.wm.getSibling(stackNode)
		grandparent := parent.parent

		spm.wm.promoteSibling(grandparent, parent, sibling)
		spm.wm.swapContainers(grandparent, sibling)

		generation := spm.wm.nextCleanupGeneration()
		spm.wm.cleanupPane(node, generation)
		spm.wm.decommissionParent(parent, generation)

		if sibling != nil {
			spm.wm.setFocusToLeaf(sibling)
		} else {
			spm.wm.updateMainPane()
		}

		stackNode.parent = nil
		stackNode.container = nil
		stackNode.stackWrapper = nil
		stackNode.stackedPanes = nil

		return nil

	case 1:
		// Promote the remaining pane back to a regular leaf node.
		lastPane := stackNode.stackedPanes[0]
		parent := stackNode.parent
		lastPaneContainer := lastPane.container

		if box, ok := stackBox.(*gtk.Box); ok && box != nil && lastPane.titleBar != nil {
			box.Remove(lastPane.titleBar)
		}
		if box, ok := stackBox.(*gtk.Box); ok && box != nil && lastPaneContainer != nil {
			box.Remove(lastPaneContainer)
		}
		if stackNode.container != nil {
			if webkit.WidgetGetParent(stackNode.container) != nil {
				webkit.WidgetUnparent(stackNode.container)
			}
		}

		if parent == nil {
			spm.wm.root = lastPane
			lastPane.parent = nil
			if spm.wm.window != nil && lastPaneContainer != nil {
				spm.wm.window.SetChild(lastPaneContainer)
				webkit.WidgetQueueAllocate(lastPaneContainer)
				webkit.WidgetShow(lastPaneContainer)
			}
		} else {
			if parent.left == stackNode {
				parent.left = lastPane
			} else if parent.right == stackNode {
				parent.right = lastPane
			}
			lastPane.parent = parent
			if paned, ok := parent.container.(*gtk.Paned); ok && paned != nil && lastPaneContainer != nil {
				if parent.left == lastPane {
					paned.SetStartChild(lastPaneContainer)
				} else {
					paned.SetEndChild(lastPaneContainer)
				}
				webkit.WidgetQueueAllocate(paned)
			}
			if lastPaneContainer != nil {
				webkit.WidgetSetVisible(lastPaneContainer, true)
			}
		}

		lastPane.isStacked = false
		lastPane.stackedPanes = nil
		lastPane.titleBar = nil
		spm.wm.viewToNode[lastPane.pane.webView] = lastPane

		generation := spm.wm.nextCleanupGeneration()
		spm.wm.cleanupPane(node, generation)

		spm.wm.SetActivePane(lastPane, SourceClose)
		if lastPaneWidget := lastPane.pane.webView.Widget(); lastPaneWidget != nil {
			webkit.WidgetShow(lastPaneWidget)
			webkit.WidgetGrabFocus(lastPaneWidget)
		}

		stackNode.parent = nil
		stackNode.container = nil
		stackNode.stackWrapper = nil
		stackNode.stackedPanes = nil

		log.Printf("[workspace] converted stack back to regular pane")
		return nil

	default:
		// Still multiple panes in stack — adjust active index and visibility.
		if stackNode.activeStackIndex >= nodeIndex && stackNode.activeStackIndex > 0 {
			stackNode.activeStackIndex--
		}
		if stackNode.activeStackIndex >= len(stackNode.stackedPanes) {
			stackNode.activeStackIndex = len(stackNode.stackedPanes) - 1
		}

		spm.UpdateStackVisibility(stackNode)
		if spm.wm.GetActiveNode() == node {
			newActivePane := stackNode.stackedPanes[stackNode.activeStackIndex]
			spm.wm.SetActivePane(newActivePane, SourceClose)
		}

		generation := spm.wm.nextCleanupGeneration()
		spm.wm.cleanupPane(node, generation)

		log.Printf("[workspace] closed pane from stack: remaining=%d activeIndex=%d",
			len(stackNode.stackedPanes), stackNode.activeStackIndex)
		return nil
	}
}

// handleTitleBarClick handles clicks on title bars to switch the active pane
func (spm *StackedPaneManager) handleTitleBarClick(titleBarID uint64) {
	pane, exists := spm.titleBarToPane[titleBarID]
	if !exists || pane == nil || pane.parent == nil || !pane.parent.isStacked {
		return
	}

	stackNode := pane.parent

	// Find the index of the clicked pane
	for i, p := range stackNode.stackedPanes {
		if p == pane {
			// Only switch if it's not already active
			if i != stackNode.activeStackIndex {
				log.Printf("[workspace] title bar clicked: switching from pane %d to pane %d",
					stackNode.activeStackIndex, i)

				// Update title bar for the pane transitioning from ACTIVE to INACTIVE
				currentActivePane := stackNode.stackedPanes[stackNode.activeStackIndex]
				if currentActivePane.pane != nil && currentActivePane.pane.webView != nil && currentActivePane.titleBar != nil {
					currentTitle := currentActivePane.pane.webView.GetTitle()
					if currentTitle != "" {
						spm.updateTitleBarLabel(currentActivePane, currentTitle)
					}
				}

				stackNode.activeStackIndex = i
				spm.UpdateStackVisibility(stackNode)
				spm.wm.SetActivePane(pane, SourceStackNav)
			}
			return
		}
	}
}
