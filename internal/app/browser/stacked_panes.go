package browser

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync/atomic"
	"time"

	"github.com/bnema/dumber/internal/cache"
	"github.com/bnema/dumber/pkg/webkit"
)

// StackedPaneManager handles all stacked pane operations
type StackedPaneManager struct {
	wm              *WorkspaceManager
	titleBarToPane  map[uintptr]*paneNode
	nextTitleBarID  uint64
}

// NewStackedPaneManager creates a new stacked pane manager
func NewStackedPaneManager(wm *WorkspaceManager) *StackedPaneManager {
	return &StackedPaneManager{
		wm:             wm,
		titleBarToPane: make(map[uintptr]*paneNode),
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
	if newLeaf.container != 0 && webkit.WidgetIsValid(newLeaf.container) {
		if webkit.WidgetGetParent(newLeaf.container) != 0 {
			webkit.WidgetUnparent(newLeaf.container)
		}
	}

	// Insert widgets at correct position for Zellij-style layout
	// Each pane has 2 widgets (titleBar, container), so position = insertIndex * 2

	// Find the widget to insert after (based on insertIndex)
	var insertAfterWidget uintptr = 0
	if insertIndex > 0 {
		// Insert after the previous pane's container
		prevPane := stackNode.stackedPanes[insertIndex-1]
		if prevPane.container != 0 {
			insertAfterWidget = prevPane.container
		}
	}

	if insertAfterWidget != 0 {
		// Insert titleBar after the previous pane's container
		if stackNode.stackWrapper != 0 && newLeaf.titleBar != 0 {
			webkit.BoxInsertChildAfter(stackNode.stackWrapper, newLeaf.titleBar, insertAfterWidget)
		}
		// Insert container after the newly inserted titleBar
		if stackNode.stackWrapper != 0 && newLeaf.container != 0 && newLeaf.titleBar != 0 {
			webkit.BoxInsertChildAfter(stackNode.stackWrapper, newLeaf.container, newLeaf.titleBar)
		}
		log.Printf("[workspace] inserted widgets at position %d (after widget %#x)", insertIndex, insertAfterWidget)
	} else {
		// Insert at the beginning (insertIndex = 0)
		if stackNode.stackWrapper != 0 && newLeaf.container != 0 {
			webkit.BoxPrepend(stackNode.stackWrapper, newLeaf.container)
		}
		if stackNode.stackWrapper != 0 && newLeaf.titleBar != 0 {
			webkit.BoxPrepend(stackNode.stackWrapper, newLeaf.titleBar)
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

	// Add CSS class to the stack container for proper styling
	webkit.WidgetAddCSSClass(stackWrapperContainer, stackContainerClass)

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
	if existingContainer != 0 {
		webkit.BoxAppend(stackInternalBox, existingContainer)
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
	if newLeaf.container != 0 && webkit.WidgetIsValid(newLeaf.container) {
		parent := webkit.WidgetGetParent(newLeaf.container)
		if parent != 0 {
			log.Printf("[workspace] unparenting new pane container %#x from parent %#x before stack append", newLeaf.container, parent)
			webkit.WidgetUnparent(newLeaf.container)
			// Verify unparent succeeded
			if finalParent := webkit.WidgetGetParent(newLeaf.container); finalParent != 0 {
				log.Printf("[workspace] WARNING: container %#x still has parent %#x after unparent", newLeaf.container, finalParent)
			}
		}
	}

	// Add the new widgets to the internal stack box
	if stackNode.stackWrapper != 0 && newLeaf.titleBar != 0 {
		webkit.BoxAppend(stackNode.stackWrapper, newLeaf.titleBar)
	}
	if stackNode.stackWrapper != 0 && newLeaf.container != 0 {
		webkit.BoxAppend(stackNode.stackWrapper, newLeaf.container)
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
		if target.container != 0 && webkit.WidgetIsValid(target.container) {
			if webkit.WidgetGetParent(target.container) != 0 {
				webkit.WidgetUnparent(target.container)
			}
		}
	} else if parent.container != 0 && webkit.WidgetIsValid(parent.container) {
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
	} else if parent.container != 0 && webkit.WidgetIsValid(parent.container) {
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

	// Mark stack operation timestamp to prevent focus conflicts
	spm.wm.lastStackOperation = time.Now()

	// This ensures Zellij-style layout where the previously active pane shows its current title
	currentActiveIndex := stackNode.activeStackIndex
	if currentActiveIndex >= 0 && currentActiveIndex < len(stackNode.stackedPanes) && currentActiveIndex != insertIndex {
		currentlyActivePane := stackNode.stackedPanes[currentActiveIndex]
		if currentlyActivePane.pane != nil && currentlyActivePane.pane.webView != nil && currentlyActivePane.titleBar != 0 {
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
			if pane.container != 0 {
				webkit.WidgetSetVisible(pane.container, true)
			}
			if pane.titleBar != 0 {
				webkit.WidgetSetVisible(pane.titleBar, false) // ABSOLUTE RULE: never visible for active pane
			}
			log.Printf("[workspace] active pane %d: container=visible, titleBar=HIDDEN", i)
		} else {
			// Inactive panes: hide container, show title bar
			if pane.container != 0 {
				webkit.WidgetSetVisible(pane.container, false)
			}
			if pane.titleBar != 0 {
				webkit.WidgetSetVisible(pane.titleBar, true)

				// Refresh title bar to pick up any newly downloaded favicons
				if pane.pane != nil && pane.pane.webView != nil {
					title := pane.pane.webView.GetTitle()
					if title == "" {
						title = "New Tab"
					}
					spm.updateTitleBarLabel(pane, title)
				}
			}
		}
	}
}

// NavigateStack handles navigation within a stacked pane container
func (spm *StackedPaneManager) NavigateStack(direction string) bool {
	if spm.wm.GetActiveNode() == nil {
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
	if currentActivePane.pane != nil && currentActivePane.pane.webView != nil && currentActivePane.titleBar != 0 {
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
func (spm *StackedPaneManager) createTitleBar(pane *paneNode) uintptr {
	// Get the actual title and URL from the WebView
	var titleText, pageURL string
	if pane.pane != nil && pane.pane.webView != nil {
		titleText = pane.pane.webView.GetTitle()
		if titleText == "" {
			titleText = "New Tab" // Fallback only when title is actually empty
		}
		pageURL = pane.pane.webView.GetCurrentURL()
	} else {
		titleText = "New Tab"
	}

	titleBar := spm.createTitleBarWithTitle(titleText, pageURL)
	if titleBar != 0 {
		// Store the mapping from titleBar ID to pane
		titleBarID := atomic.AddUint64(&spm.nextTitleBarID, 1)
		spm.titleBarToPane[uintptr(titleBarID)] = pane

		// Attach click handler
		webkit.WidgetAttachClickHandler(titleBar, func() {
			spm.handleTitleBarClick(uintptr(titleBarID))
		})
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
	if node.parent != nil && node.parent.isStacked && node.titleBar != 0 {
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
	if node == nil || node.titleBar == 0 || node.parent == nil || !node.parent.isStacked {
		return
	}

	// Get the current URL from the WebView for favicon
	var pageURL string
	if node.pane != nil && node.pane.webView != nil {
		pageURL = node.pane.webView.GetCurrentURL()
	}

	// Create new title bar with updated title and favicon
	newTitleBar := spm.createTitleBarWithTitle(title, pageURL)
	if newTitleBar == 0 {
		log.Printf("[workspace] failed to create new title bar")
		return
	}

	// Store the mapping from titleBar ID to pane and attach click handler
	titleBarID := atomic.AddUint64(&spm.nextTitleBarID, 1)
	spm.titleBarToPane[uintptr(titleBarID)] = node

	// Attach click handler
	webkit.WidgetAttachClickHandler(newTitleBar, func() {
		spm.handleTitleBarClick(uintptr(titleBarID))
	})

	// Replace the old title bar in the stack
	if node.parent != nil && node.parent.isStacked && node.parent.stackWrapper != 0 {
		// Find the correct insertion position based on the pane's index in the stack
		paneIndex := -1
		for i, stackedPane := range node.parent.stackedPanes {
			if stackedPane == node {
				paneIndex = i
				break
			}
		}

		// Remove old title bar
		if node.titleBar != 0 && webkit.WidgetIsValid(node.titleBar) {
			webkit.WidgetUnparent(node.titleBar)
		}

		// Update the node's title bar reference
		spm.wm.setTitleBar(node, newTitleBar)

		// Insert the new title bar at the correct position
		if paneIndex == 0 {
			// First pane - insert at the beginning
			if node.parent.stackWrapper != 0 {
				webkit.BoxPrepend(node.parent.stackWrapper, newTitleBar)
			}
		} else {
			// Insert after the previous pane's container
			prevPane := node.parent.stackedPanes[paneIndex-1]
			if node.parent.stackWrapper != 0 && prevPane.container != 0 {
				webkit.BoxInsertChildAfter(node.parent.stackWrapper, newTitleBar, prevPane.container)
			}
		}

		log.Printf("[workspace] replaced title bar for pane %d: %p", paneIndex, node)
	}
}

// createTitleBarWithTitle creates a title bar with a specific title and optional favicon
func (spm *StackedPaneManager) createTitleBarWithTitle(title string, pageURL string) uintptr {
	titleBox := webkit.NewBox(webkit.OrientationHorizontal, 8)
	if titleBox == 0 {
		return 0
	}

	webkit.WidgetAddCSSClass(titleBox, "stacked-pane-title")
	webkit.WidgetSetHExpand(titleBox, true)
	webkit.WidgetSetVExpand(titleBox, false)

	// Add favicon placeholder for debugging (always visible)
	var faviconImg uintptr
	var debugReason string

	if pageURL == "" {
		debugReason = "empty pageURL"
	} else if spm.wm == nil {
		debugReason = "nil workspace manager"
	} else if spm.wm.app == nil {
		debugReason = "nil app"
	} else if spm.wm.app.queries == nil {
		debugReason = "nil queries"
	} else {
		// Query database for favicon URL
		ctx := context.Background()
		entry, err := spm.wm.app.queries.GetHistoryEntry(ctx, pageURL)
		if err != nil {
			debugReason = fmt.Sprintf("DB query failed: %v", err)
			log.Printf("[favicon] Failed to get history entry for %s: %v", pageURL, err)
		} else if !entry.FaviconUrl.Valid || entry.FaviconUrl.String == "" {
			debugReason = "no favicon URL in DB"
			log.Printf("[favicon] No favicon URL in DB for %s", pageURL)
		} else {
			faviconURL := entry.FaviconUrl.String
			log.Printf("[favicon] Found favicon URL in DB for %s: %s", pageURL, faviconURL)

			// Use favicon cache with favicon URL (same as dmenu)
			faviconCache, err := cache.NewFaviconCache()
			if err != nil {
				debugReason = fmt.Sprintf("cache init failed: %v", err)
				log.Printf("[favicon] Failed to create cache: %v", err)
			} else {
				faviconPath := faviconCache.GetCachedPath(faviconURL)
				if faviconPath != "" {
					log.Printf("[favicon] Loading cached favicon from: %s", faviconPath)
					faviconImg = webkit.ImageNewFromFile(faviconPath)
					if faviconImg != 0 {
						webkit.ImageSetPixelSize(faviconImg, 16)
						webkit.WidgetAddCSSClass(faviconImg, "stacked-pane-favicon")
						log.Printf("[favicon] Successfully created favicon image widget")
					} else {
						debugReason = "image widget creation failed"
						log.Printf("[favicon] Failed to create image widget from %s", faviconPath)
					}
				} else {
					debugReason = "not cached yet"
					log.Printf("[favicon] Favicon not cached yet, starting download: %s", faviconURL)
					// Start async download for next time (same as dmenu)
					faviconCache.CacheAsync(faviconURL)
				}
			}
		}
	}

	// Add favicon or placeholder
	if faviconImg != 0 {
		webkit.BoxAppend(titleBox, faviconImg)
		webkit.WidgetShow(faviconImg)
	} else {
		// DEBUG: Add visible placeholder label to show code is running
		placeholderLabel := webkit.NewLabel("🌐")
		if placeholderLabel != 0 {
			webkit.WidgetAddCSSClass(placeholderLabel, "stacked-pane-favicon-placeholder")
			webkit.BoxAppend(titleBox, placeholderLabel)
			webkit.WidgetShow(placeholderLabel)
			log.Printf("[favicon] Using placeholder for %s (reason: %s)", pageURL, debugReason)
		}
	}

	// Create title label with the specified title
	titleLabel := webkit.NewLabel(title)
	if titleLabel == 0 {
		return titleBox
	}

	webkit.WidgetAddCSSClass(titleLabel, "stacked-pane-title-text")
	webkit.LabelSetEllipsize(titleLabel, webkit.EllipsizeEnd)

	webkit.BoxAppend(titleBox, titleLabel)
	webkit.WidgetShow(titleBox)
	webkit.WidgetShow(titleLabel)

	return titleBox
}

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
	if stackBox == 0 {
		stackBox = stackNode.container
	}
	if stackBox != 0 {
		if node.titleBar != 0 {
			webkit.BoxRemove(stackBox, node.titleBar)
		}
		if node.container != 0 {
			webkit.BoxRemove(stackBox, node.container)
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
			if stackNode.container != 0 && webkit.WidgetIsValid(stackNode.container) {
				if webkit.WidgetGetParent(stackNode.container) != 0 {
					webkit.WidgetUnparent(stackNode.container)
				}
			}
			if spm.wm.window != nil {
				spm.wm.window.SetChild(0)
			}
			_, err := spm.wm.cleanupAndExit(node)
			return err
		}

		// Detach the empty stack container from its GtkPaned parent.
		if parent.container != 0 && webkit.WidgetIsValid(parent.container) {
			if parent.left == stackNode {
				webkit.PanedSetStartChild(parent.container, 0)
			} else if parent.right == stackNode {
				webkit.PanedSetEndChild(parent.container, 0)
			}
			webkit.WidgetQueueAllocate(parent.container)
		}
		if stackNode.container != 0 && webkit.WidgetIsValid(stackNode.container) {
			if webkit.WidgetGetParent(stackNode.container) != 0 {
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
		stackNode.container = 0
		stackNode.stackWrapper = 0
		stackNode.stackedPanes = nil

		return nil

	case 1:
		// Promote the remaining pane back to a regular leaf node.
		lastPane := stackNode.stackedPanes[0]
		parent := stackNode.parent
		lastPaneContainer := lastPane.container

		if lastPane.titleBar != 0 && stackBox != 0 {
			webkit.BoxRemove(stackBox, lastPane.titleBar)
		}
		if stackBox != 0 && lastPaneContainer != 0 {
			webkit.BoxRemove(stackBox, lastPaneContainer)
		}
		if stackNode.container != 0 && webkit.WidgetIsValid(stackNode.container) {
			if webkit.WidgetGetParent(stackNode.container) != 0 {
				webkit.WidgetUnparent(stackNode.container)
			}
		}

		if parent == nil {
			spm.wm.root = lastPane
			lastPane.parent = nil
			if spm.wm.window != nil && lastPaneContainer != 0 {
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
			if parent.container != 0 && webkit.WidgetIsValid(parent.container) && lastPaneContainer != 0 {
				if parent.left == lastPane {
					webkit.PanedSetStartChild(parent.container, lastPaneContainer)
				} else {
					webkit.PanedSetEndChild(parent.container, lastPaneContainer)
				}
				webkit.WidgetQueueAllocate(parent.container)
			}
			if lastPaneContainer != 0 {
				webkit.WidgetSetVisible(lastPaneContainer, true)
			}
		}

		lastPane.isStacked = false
		lastPane.stackedPanes = nil
		lastPane.titleBar = 0
		spm.wm.viewToNode[lastPane.pane.webView] = lastPane

		generation := spm.wm.nextCleanupGeneration()
		spm.wm.cleanupPane(node, generation)

		spm.wm.SetActivePane(lastPane, SourceClose)
		if lastPaneWidget := lastPane.pane.webView.Widget(); lastPaneWidget != 0 {
			webkit.WidgetShow(lastPaneWidget)
			webkit.WidgetGrabFocus(lastPaneWidget)
		}

		stackNode.parent = nil
		stackNode.container = 0
		stackNode.stackWrapper = 0
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
func (spm *StackedPaneManager) handleTitleBarClick(titleBarID uintptr) {
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
				if currentActivePane.pane != nil && currentActivePane.pane.webView != nil && currentActivePane.titleBar != 0 {
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
