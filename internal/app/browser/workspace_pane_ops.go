// workspace_pane_ops.go - Pane creation, splitting, closing and tree operations
package browser

import (
	"errors"
	"log"

	"github.com/bnema/dumber/pkg/webkit"
)

// closeContext manages the context for a pane close operation
type closeContext struct {
	wm         *WorkspaceManager
	target     *paneNode
	remaining  int
	err        error
	generation uint
}

// beginClose initializes a close context with validation
func (wm *WorkspaceManager) beginClose(node *paneNode) closeContext {
	ctx := closeContext{wm: wm, target: node, generation: wm.nextCleanupGeneration()}
	switch {
	case wm == nil:
		ctx.err = errors.New("workspace manager nil")
	case node == nil || !node.isLeaf:
		ctx.err = errors.New("invalid close target")
	case node.pane == nil || node.pane.webView == nil:
		ctx.err = errors.New("close target missing webview")
	default:
		ctx.remaining = len(wm.app.panes)
	}
	return ctx
}

// Generation returns the cleanup generation for this context
func (ctx closeContext) Generation() uint {
	return ctx.generation
}

// finish handles cleanup operations after close completion
func (ctx closeContext) finish() {
	if ctx.err == nil {
		ctx.wm.updateMainPane()
	}
}

// collectLeaves returns all leaf nodes in the workspace tree
func (wm *WorkspaceManager) collectLeaves() []*paneNode {
	return wm.collectLeavesFrom(wm.root)
}

// collectLeavesFrom collects all leaf nodes from a given subtree
func (wm *WorkspaceManager) collectLeavesFrom(node *paneNode) []*paneNode {
	var leaves []*paneNode
	visited := make(map[*paneNode]bool)

	var walk func(*paneNode, int)
	walk = func(n *paneNode, depth int) {
		// Prevent infinite recursion and cycles
		const maxDepth = 50
		if n == nil || depth > maxDepth {
			return
		}
		if visited[n] {
			log.Printf("[workspace] collectLeavesFrom: cycle detected in tree")
			return
		}
		visited[n] = true

		if n.isLeaf {
			leaves = append(leaves, n)
			return
		}

		// Handle stacked panes - only include the currently ACTIVE pane as a leaf
		// This prevents multiple CSS classes and ensures correct focus management
		if n.isStacked && len(n.stackedPanes) > 0 {
			activeIndex := n.activeStackIndex
			if activeIndex >= 0 && activeIndex < len(n.stackedPanes) {
				// Only the active pane in the stack counts as a leaf for focus/navigation
				activePaneInStack := n.stackedPanes[activeIndex]
				walk(activePaneInStack, depth+1)
			}
			return
		}

		// Handle regular split nodes
		walk(n.left, depth+1)
		walk(n.right, depth+1)
	}
	walk(node, 0)
	return leaves
}

// createWebView creates a new WebView with the workspace's configuration
func (wm *WorkspaceManager) createWebView() (*webkit.WebView, error) {
	if wm == nil || wm.createWebViewFn == nil {
		return nil, errors.New("workspace manager missing webview factory")
	}
	return wm.createWebViewFn()
}

// createPane creates a new BrowserPane for the given WebView
func (wm *WorkspaceManager) createPane(view *webkit.WebView) (*BrowserPane, error) {
	if wm == nil || wm.createPaneFn == nil {
		return nil, errors.New("workspace manager missing pane factory")
	}
	return wm.createPaneFn(view)
}

// clonePaneState sets up a new pane with cloned state from another pane
func (wm *WorkspaceManager) clonePaneState(_ *paneNode, target *paneNode) {
	if wm == nil || target == nil {
		return
	}
	if target.pane == nil || target.pane.webView == nil {
		return
	}

	const blankURL = "about:blank"

	if err := target.pane.webView.LoadURL(blankURL); err != nil {
		log.Printf("[workspace] failed to prepare blank pane: %v", err)
	}

	// Wait for about:blank to load before opening omnibox
	target.pane.webView.RegisterURIChangedHandler(func(uri string) {
		if uri == blankURL {
			// Defer omnibox opening to allow page to fully initialize
			wm.scheduleIdleGuarded(func() bool {
				if target == nil || target.pane == nil || target.pane.webView == nil {
					return false
				}
				if injErr := target.pane.webView.InjectScript("window.__dumber_omnibox?.open('omnibox');"); injErr != nil {
					log.Printf("[workspace] failed to open omnibox: %v", injErr)
				}
				return false // Remove idle callback
			}, target)
		}
	})
}

// safelyDetachControllersBeforeReparent removes GTK controllers that GTK will auto-clean up during reparenting.
// It marks nodes for reattachment once the widget hierarchy settles after the split operation.
func (wm *WorkspaceManager) safelyDetachControllersBeforeReparent(node *paneNode) {
	if wm == nil || node == nil {
		return
	}

	markForDetachment := func(target *paneNode) {
		if target == nil {
			return
		}

		if target.hoverToken != 0 {
			target.pendingHoverReattach = true
			wm.detachHover(target)
		}

		if wm.focusStateMachine != nil && target.focusControllerToken != 0 {
			token := target.focusControllerToken
			target.pendingFocusReattach = true
			wm.focusStateMachine.detachGTKController(target, token)
		}
	}

	if node.isStacked {
		for _, child := range node.stackedPanes {
			markForDetachment(child)
		}
		return
	}

	if node.isLeaf {
		markForDetachment(node)
	}
}

// closeCurrentPane closes the currently focused pane
func (wm *WorkspaceManager) closeCurrentPane() {
	if wm == nil || wm.GetActiveNode() == nil {
		return
	}
	if err := wm.ClosePane(wm.GetActiveNode()); err != nil {
		log.Printf("[workspace] close current pane failed: %v", err)
	}
}

// leftmostLeaf finds the leftmost leaf node in a subtree
func (wm *WorkspaceManager) leftmostLeaf(node *paneNode) *paneNode {
	for node != nil && !node.isLeaf {
		if node.left != nil {
			node = node.left
			continue
		}
		node = node.right
	}
	return node
}

// findReplacementRoot finds a suitable replacement when closing the current root pane
func (wm *WorkspaceManager) findReplacementRoot(excludeNode *paneNode) *paneNode {
	if wm == nil || wm.root == nil {
		return nil
	}

	// If root is being closed and there are other panes, find a replacement
	leaves := wm.collectLeaves()
	for _, leaf := range leaves {
		if leaf != excludeNode && leaf != nil && leaf.isLeaf {
			// Find the topmost ancestor that's not the current root
			current := leaf
			for current.parent != nil && current.parent != wm.root {
				current = current.parent
			}

			// If this leaf is a direct child of root, or root only has one subtree,
			// we can promote this subtree to be the new root
			if current.parent == wm.root {
				// If the sibling is being excluded, promote this subtree
				var sibling *paneNode
				if wm.root.left == current {
					sibling = wm.root.right
				} else {
					sibling = wm.root.left
				}

				if sibling == excludeNode {
					// The sibling is being closed, so promote this subtree
					return current
				}
			}

			// Otherwise, return the first suitable leaf
			return leaf
		}
	}

	return nil
}

// updateMainPane updates the main pane reference based on current state
func (wm *WorkspaceManager) updateMainPane() {
	if len(wm.app.panes) == 1 {
		if leaf := wm.viewToNode[wm.app.panes[0].webView]; leaf != nil {
			wm.mainPane = leaf
		}
		return
	}

	if wm.mainPane == nil || !wm.mainPane.isLeaf {
		if wm.GetActiveNode() != nil && wm.GetActiveNode().isLeaf {
			wm.mainPane = wm.GetActiveNode()
		}
	}
}

// Helper methods to support clean pane removal from workspace tracking
// These methods implement the interface expected by BrowserPane.CleanupFromWorkspace()

// removeFromMaps removes a WebView from all workspace tracking maps
func (wm *WorkspaceManager) removeFromMaps(webView *webkit.WebView) {
	if wm == nil || webView == nil {
		return
	}

	delete(wm.viewToNode, webView)
	delete(wm.lastSplitMsg, webView)
	delete(wm.lastExitMsg, webView)
	log.Printf("[workspace] removed webview %p from tracking maps", webView)
}

// removeFromAppPanes removes a BrowserPane from the app.panes slice
func (wm *WorkspaceManager) removeFromAppPanes(pane *BrowserPane) {
	if wm == nil || wm.app == nil || pane == nil {
		return
	}

	for i, p := range wm.app.panes {
		if p == pane {
			wm.app.panes = append(wm.app.panes[:i], wm.app.panes[i+1:]...)
			log.Printf("[workspace] removed pane from app.panes slice, remaining: %d", len(wm.app.panes))
			return
		}
	}
}

// insertPopupPane inserts a pre-created popup pane into the workspace
func (wm *WorkspaceManager) insertPopupPane(target *paneNode, newPane *BrowserPane, direction string) error {
	if target == nil {
		return errors.New("insert target cannot be nil")
	}

	// Accept both leaf panes and stacked panes as valid targets
	if !target.isLeaf && !target.isStacked {
		return errors.New("insert target must be a leaf pane or stacked pane")
	}

	// For leaf panes, require a pane
	if target.isLeaf && target.pane == nil {
		return errors.New("leaf target missing pane")
	}

	// For stacked panes, we trust the caller - production stacks have varied structures
	// Do not validate internal stack structure here

	if newPane == nil || newPane.webView == nil {
		return errors.New("new pane missing webview")
	}

	if handler := newPane.MessageHandler(); handler != nil {
		handler.SetWorkspaceObserver(wm)
	}

	newContainer := newPane.webView.RootWidget()
	if newContainer == 0 {
		return errors.New("new pane missing container")
	}

	webkit.WidgetSetHExpand(newContainer, true)
	webkit.WidgetSetVExpand(newContainer, true)
	webkit.WidgetRealizeInContainer(newContainer)

	// Also realize the WebView widget itself for proper popup rendering
	webViewWidget := newPane.webView.Widget()
	if webViewWidget != 0 {
		webkit.WidgetRealizeInContainer(webViewWidget)
	}

	if target.container == 0 {
		return errors.New("existing pane missing container")
	}

	orientation, existingFirst := mapDirection(direction)
	log.Printf("[workspace] inserting popup direction=%s orientation=%v existingFirst=%v target.parent=%p", direction, orientation, existingFirst, target.parent)

	paned := webkit.NewPaned(orientation)
	if paned == 0 {
		return errors.New("failed to create GtkPaned")
	}
	webkit.WidgetSetHExpand(paned, true)
	webkit.WidgetSetVExpand(paned, true)
	// For popup panes: preserve existing pane size, let popup take remaining space
	webkit.PanedSetResizeStart(paned, false) // existing pane keeps its size
	webkit.PanedSetResizeEnd(paned, true)    // popup can resize
	log.Printf("[workspace] configured paned for popup: start=fixed, end=flexible")

	newLeaf := &paneNode{
		pane:   newPane,
		isLeaf: true,
	}
	wm.initializePaneWidgets(newLeaf, newContainer)

	split := &paneNode{
		parent:      target.parent,
		orientation: orientation,
		isLeaf:      false,
	}
	wm.initializePaneWidgets(split, paned)

	parent := split.parent

	// CRITICAL: Capture parent paned's divider position BEFORE reparenting to preserve layout
	var parentDividerPos int
	if parent != nil && parent.container != 0 && webkit.WidgetIsValid(parent.container) {
		parentDividerPos = webkit.PanedGetPosition(parent.container)
		log.Printf("[workspace] captured parent divider position: %d", parentDividerPos)
	}

	// Clear CSS classes before reparenting to avoid GTK bloom filter corruption
	existingContainer := target.container
	if existingContainer != 0 && webkit.WidgetIsValid(existingContainer) {
		// Remove active border class if present (prevents GTK bloom filter corruption during unparent)
		if webkit.WidgetHasCSSClass(existingContainer, activePaneClass) {
			webkit.WidgetRemoveCSSClass(existingContainer, activePaneClass)
		}

		// CRITICAL: Hide widget before reparenting to disconnect WebKitGTK rendering pipeline
		// This forces WebKit to detach its compositor, preventing rendering corruption
		webkit.WidgetHide(existingContainer)
		log.Printf("[workspace] Hidden existing container before reparenting: %#x", existingContainer)
	}

	// Prevent GTK from auto-removing controllers while the widget is temporarily unparented.
	wm.safelyDetachControllersBeforeReparent(target)

	// Detach existing container from its current GTK parent before inserting into new paned.
	if parent == nil {
		// Target is the root - remove it from the window
		log.Printf("[workspace] removing existing container=%#x from window", existingContainer)
		if wm.window != nil {
			wm.window.SetChild(0)
		}
		// GTK4 will auto-unparent when we add to new paned - no manual unparent needed
	} else if parent.container != 0 {
		// Target has a parent paned - unparent it from there
		log.Printf("[workspace] unparenting existing container from parent paned")
		if webkit.WidgetIsValid(parent.container) {
			log.Printf("[workspace] executing unparent: parentPtr=%#x", parent.container)
			if parent.left == target {
				webkit.PanedSetStartChild(parent.container, 0)
			} else if parent.right == target {
				webkit.PanedSetEndChild(parent.container, 0)
			}
			webkit.WidgetQueueAllocate(parent.container)
		}
	}

	// Add both containers to the new paned
	if existingFirst {
		split.left = target
		split.right = newLeaf
		webkit.PanedSetStartChild(paned, existingContainer)
		webkit.PanedSetEndChild(paned, newContainer)
		log.Printf("[workspace] added existing=%#x as start child, new=%#x as end child", existingContainer, newContainer)
	} else {
		split.left = newLeaf
		split.right = target
		webkit.PanedSetStartChild(paned, newContainer)
		webkit.PanedSetEndChild(paned, existingContainer)
		log.Printf("[workspace] added new=%#x as start child, existing=%#x as end child", newContainer, existingContainer)
	}

	target.parent = split
	newLeaf.parent = split

	if parent == nil {
		wm.root = split
		if wm.window != nil {
			wm.window.SetChild(paned)
			webkit.WidgetQueueAllocate(paned)
			log.Printf("[workspace] paned set as window child: paned=%#x", paned)
		}
	} else {
		if webkit.WidgetIsValid(parent.container) {
			log.Printf("[workspace] executing parent reparent: parentPtr=%#x, paned=%#x", parent.container, paned)
			if parent.left == target {
				parent.left = split
				webkit.PanedSetStartChild(parent.container, paned)
			} else if parent.right == target {
				parent.right = split
				webkit.PanedSetEndChild(parent.container, paned)
			}
			webkit.WidgetQueueAllocate(parent.container)
			webkit.WidgetQueueAllocate(paned)
			log.Printf("[workspace] paned inserted into parent successfully")

			// CRITICAL: Restore parent paned's divider position to preserve existing layout
			if parentDividerPos > 0 {
				webkit.PanedSetPosition(parent.container, parentDividerPos)
				log.Printf("[workspace] restored parent divider position: %d", parentDividerPos)
			}
		}
	}

	webkit.WidgetShow(paned)

	// CRITICAL: Set initial 50/50 split position after showing paned
	// GTK needs the widget to be realized before we can set position based on allocation
	// Schedule this to run after GTK has allocated space to the paned
	wm.scheduleIdleGuarded(func() bool {
		if !webkit.WidgetIsValid(paned) {
			return false
		}
		// Get paned allocation to calculate 50% position
		alloc := webkit.WidgetGetAllocation(paned)
		var splitPos int
		if orientation == webkit.OrientationHorizontal {
			splitPos = alloc.Width / 2
		} else {
			splitPos = alloc.Height / 2
		}
		if splitPos > 0 {
			webkit.PanedSetPosition(paned, splitPos)
			log.Printf("[workspace] Set initial paned position to 50%%: %d (orientation=%d, size=%dx%d)", splitPos, orientation, alloc.Width, alloc.Height)
		}
		return false
	}, split)

	// CRITICAL: Show the existing container and force GTK to recreate rendering surface after reparenting
	// This reconnects WebKitGTK's rendering pipeline and fixes compositor sync issues
	if target != nil && target.container != 0 {
		wm.scheduleIdleGuarded(func() bool {
			if target == nil || !target.widgetValid || target.container == 0 {
				return false
			}
			// Show widget to reconnect WebKit rendering pipeline
			webkit.WidgetShow(target.container)
			// Force GTK to recalculate size and recreate rendering surface
			webkit.WidgetQueueResize(target.container)
			webkit.WidgetQueueDraw(target.container)
			log.Printf("[workspace] Shown and queued resize+draw for target container after reparenting")
			return false
		}, target)
	}

	wm.viewToNode[newPane.webView] = newLeaf
	wm.ensureHover(newLeaf)
	wm.ensureHover(target)
	wm.app.panes = append(wm.app.panes, newPane)
	if newPane.zoomController != nil {
		newPane.zoomController.ApplyInitialZoom()
	}

	// Update CSS classes for all panes now that we have multiple panes

	reattachTargets := []*paneNode{target}
	if target != nil && target.isStacked {
		reattachTargets = append(reattachTargets, target.stackedPanes...)
	}
	guardNodes := append([]*paneNode{newLeaf}, reattachTargets...)
	wm.scheduleIdleGuarded(func() bool {
		if newLeaf == nil || !newLeaf.widgetValid {
			return false
		}
		if newContainer != 0 && webkit.WidgetIsValid(newContainer) {
			webkit.WidgetShow(newContainer)
		}
		// Attach GTK focus controller to new pane
		if wm.focusStateMachine != nil {
			wm.focusStateMachine.attachGTKController(newLeaf)
		}

		for _, candidate := range reattachTargets {
			if candidate == nil || !candidate.widgetValid {
				continue
			}
			if candidate.pendingHoverReattach {
				wm.ensureHover(candidate)
				if candidate.hoverToken != 0 {
					candidate.pendingHoverReattach = false
				}
			}
			if candidate.pendingFocusReattach && wm.focusStateMachine != nil {
				wm.focusStateMachine.attachGTKController(candidate)
				if candidate.focusControllerToken != 0 {
					candidate.pendingFocusReattach = false
				}
			}
		}

		wm.SetActivePane(newLeaf, SourceSplit)
		return false
	}, guardNodes...)

	return nil
}

// splitNode splits a target pane into two panes in the specified direction
func (wm *WorkspaceManager) splitNode(target *paneNode, direction string) (*paneNode, error) {
	if target == nil || !target.isLeaf || target.pane == nil {
		return nil, errors.New("split target must be a leaf pane")
	}

	// IMPORTANT: If the target is inside a stacked pane, we need to split AROUND the stack,
	// not the individual pane or the stack itself. We create a new split at the stack's parent level.
	originalTarget := target
	var stackContainer *paneNode
	if target.parent != nil && target.parent.isStacked {
		// Target is in a stack - we'll create the split around the entire stack
		stackContainer = target.parent
		log.Printf("[workspace] target is in stack, will split around the stack: originalTarget=%p stackContainer=%p", originalTarget, stackContainer)
	}

	// Determine what we're actually splitting
	var splitTarget *paneNode
	var splitTargetContainer uintptr

	if stackContainer != nil {
		// Case: splitting from inside a stack - we split around the entire stack
		splitTarget = stackContainer
		splitTargetContainer = stackContainer.container
		log.Printf("[workspace] splitting around stack: stackContainer=%p", splitTarget)
	} else {
		// Case: normal split from a simple pane
		splitTarget = target
		splitTargetContainer = target.container
		log.Printf("[workspace] normal split from pane: target=%p", splitTarget)
	}

	newView, err := wm.createWebView()
	if err != nil {
		return nil, err
	}

	newPane, err := wm.createPane(newView)
	if err != nil {
		return nil, err
	}

	if handler := newPane.MessageHandler(); handler != nil {
		handler.SetWorkspaceObserver(wm)
	}

	newContainer := newPane.webView.RootWidget()
	if newContainer == 0 {
		return nil, errors.New("new pane missing container")
	}
	webkit.WidgetSetHExpand(newContainer, true)
	webkit.WidgetSetVExpand(newContainer, true)
	webkit.WidgetRealizeInContainer(newContainer)

	// Use the determined split target container
	if splitTargetContainer == 0 {
		return nil, errors.New("split target missing container")
	}

	// Only apply GTK operations if this is a simple widget (leaf pane)
	// For stacked containers, we need to be more careful
	if splitTarget.isLeaf {
		if webkit.WidgetIsValid(splitTargetContainer) {
			webkit.WidgetSetHExpand(splitTargetContainer, true)
			webkit.WidgetSetVExpand(splitTargetContainer, true)
			webkit.WidgetRealizeInContainer(splitTargetContainer)
		}
	} else if splitTarget.isStacked {
		// Stack containers need the same setup as regular panes for proper splitting
		if webkit.WidgetIsValid(splitTargetContainer) {
			webkit.WidgetSetHExpand(splitTargetContainer, true)
			webkit.WidgetSetVExpand(splitTargetContainer, true)
			webkit.WidgetRealizeInContainer(splitTargetContainer)
			log.Printf("[workspace] configured stack wrapper for split: %#x", splitTargetContainer)
		}
	}

	orientation, existingFirst := mapDirection(direction)
	log.Printf("[workspace] splitting direction=%s orientation=%v existingFirst=%v splitTarget.parent=%p splitTarget.isStacked=%v",
		direction, orientation, existingFirst, splitTarget.parent, splitTarget.isStacked)

	paned := webkit.NewPaned(orientation)
	if paned == 0 {
		return nil, errors.New("failed to create GtkPaned")
	}
	webkit.WidgetSetHExpand(paned, true)
	webkit.WidgetSetVExpand(paned, true)
	webkit.PanedSetResizeStart(paned, true)
	webkit.PanedSetResizeEnd(paned, true)

	newLeaf := &paneNode{
		pane:   newPane,
		isLeaf: true,
	}
	wm.initializePaneWidgets(newLeaf, newContainer)

	split := &paneNode{
		parent:      splitTarget.parent,
		orientation: orientation,
		isLeaf:      false,
	}
	wm.initializePaneWidgets(split, paned)

	parent := split.parent

	// Clear CSS classes before reparenting to avoid GTK bloom filter corruption
	if splitTargetContainer != 0 && webkit.WidgetIsValid(splitTargetContainer) {
		// Remove active border class if present (prevents GTK bloom filter corruption during unparent)
		if webkit.WidgetHasCSSClass(splitTargetContainer, activePaneClass) {
			webkit.WidgetRemoveCSSClass(splitTargetContainer, activePaneClass)
		}
	}

	// Prevent GTK from auto-dropping controllers while the widget is temporarily unparented.
	wm.safelyDetachControllersBeforeReparent(splitTarget)

	// Detach split target container from its current GTK parent before inserting into new paned.
	if parent == nil {
		// Split target is the root - remove it from the window
		log.Printf("[workspace] removing split target container=%#x from window", splitTargetContainer)
		if wm.window != nil {
			wm.window.SetChild(0)
		}
		// GTK4 will auto-unparent when we add to new paned - no manual unparent needed
	} else if parent.container != 0 {
		// Split target has a parent paned - remove it (automatically unparents in GTK4)
		log.Printf("[workspace] unparenting split target container=%#x from parent paned=%#x", splitTargetContainer, parent.container)
		if webkit.WidgetIsValid(parent.container) {
			if parent.left == splitTarget {
				webkit.PanedSetStartChild(parent.container, 0)
			} else if parent.right == splitTarget {
				webkit.PanedSetEndChild(parent.container, 0)
			}
			// GTK4 automatically unparents when we set paned child to 0 - no manual unparent needed
			webkit.WidgetQueueAllocate(parent.container)
		}
	}

	// Set up the tree structure first
	if existingFirst {
		split.left = splitTarget
		split.right = newLeaf
	} else {
		split.left = newLeaf
		split.right = splitTarget
	}

	splitTarget.parent = split
	newLeaf.parent = split

	// Update tree root/parent references
	if parent == nil {
		wm.root = split
	} else {
		if parent.left == splitTarget {
			parent.left = split
		} else if parent.right == splitTarget {
			parent.right = split
		}
	}

	// GTK4 handles widget operations automatically - no need to force redraw

	// Add both containers to the new paned
	if existingFirst {
		if webkit.WidgetIsValid(splitTargetContainer) {
			webkit.PanedSetStartChild(paned, splitTargetContainer)
			webkit.PanedSetEndChild(paned, newContainer)
			log.Printf("[workspace] added splitTarget=%#x as start child, new=%#x as end child", splitTargetContainer, newContainer)
		}
	} else {
		if webkit.WidgetIsValid(splitTargetContainer) {
			webkit.PanedSetStartChild(paned, newContainer)
			webkit.PanedSetEndChild(paned, splitTargetContainer)
			log.Printf("[workspace] added new=%#x as start child, splitTarget=%#x as end child", newContainer, splitTargetContainer)
		}
	}

	// Attach the new paned to its parent
	if parent == nil {
		if wm.window != nil {
			wm.window.SetChild(paned)
			webkit.WidgetQueueAllocate(paned)
			log.Printf("[workspace] paned set as window child: paned=%#x", paned)
		}
	} else {
		if webkit.WidgetIsValid(parent.container) {
			if parent.left == split {
				webkit.PanedSetStartChild(parent.container, paned)
			} else if parent.right == split {
				webkit.PanedSetEndChild(parent.container, paned)
			}
			webkit.WidgetQueueAllocate(parent.container)
			webkit.WidgetQueueAllocate(paned)
			log.Printf("[workspace] paned inserted into parent=%#x", parent.container)
		}
	}

	webkit.WidgetShow(paned)

	// CRITICAL: Set initial 50/50 split position after showing paned
	// GTK needs the widget to be realized before we can set position based on allocation
	// Schedule this to run after GTK has allocated space to the paned
	wm.scheduleIdleGuarded(func() bool {
		if !webkit.WidgetIsValid(paned) {
			return false
		}
		// Get paned allocation to calculate 50% position
		alloc := webkit.WidgetGetAllocation(paned)
		var splitPos int
		if orientation == webkit.OrientationHorizontal {
			splitPos = alloc.Width / 2
		} else {
			splitPos = alloc.Height / 2
		}
		if splitPos > 0 {
			webkit.PanedSetPosition(paned, splitPos)
			log.Printf("[workspace] Set initial paned position to 50%%: %d (orientation=%d, size=%dx%d)", splitPos, orientation, alloc.Width, alloc.Height)
		}
		return false
	}, split)

	// CRITICAL: Show the existing container and force GTK to recreate rendering surface after reparenting
	// This reconnects WebKitGTK's rendering pipeline and fixes compositor sync issues
	if target != nil && target.container != 0 {
		wm.scheduleIdleGuarded(func() bool {
			if target == nil || !target.widgetValid || target.container == 0 {
				return false
			}
			// Show widget to reconnect WebKit rendering pipeline
			webkit.WidgetShow(target.container)
			// Force GTK to recalculate size and recreate rendering surface
			webkit.WidgetQueueResize(target.container)
			webkit.WidgetQueueDraw(target.container)
			log.Printf("[workspace] Shown and queued resize+draw for target container after reparenting")
			return false
		}, target)
	}

	wm.viewToNode[newPane.webView] = newLeaf
	wm.ensureHover(newLeaf)

	// Don't ensure hover on originalTarget when splitting from stack - it competes with new pane focus
	// The originalTarget is still in the stack and should not interfere with the new split pane
	if originalTarget == target {
		// Only ensure hover on target if it's a regular (non-stack) split
		if !target.pendingHoverReattach {
			wm.ensureHover(target)
		}
	}

	wm.app.panes = append(wm.app.panes, newPane)
	if newPane.zoomController != nil {
		newPane.zoomController.ApplyInitialZoom()
	}

	// Update CSS classes for all panes now that we have multiple panes

	reattachTargets := []*paneNode{splitTarget}
	if splitTarget != nil && splitTarget.isStacked {
		reattachTargets = append(reattachTargets, splitTarget.stackedPanes...)
	}
	guardNodes := append([]*paneNode{newLeaf}, reattachTargets...)
	wm.scheduleIdleGuarded(func() bool {
		if newLeaf == nil || !newLeaf.widgetValid {
			return false
		}
		if newContainer != 0 && webkit.WidgetIsValid(newContainer) {
			webkit.WidgetShow(newContainer)
		}
		// Attach GTK focus controller to new pane
		if wm.focusStateMachine != nil {
			wm.focusStateMachine.attachGTKController(newLeaf)
		}

		for _, candidate := range reattachTargets {
			if candidate == nil || !candidate.widgetValid {
				continue
			}
			if candidate.pendingHoverReattach {
				wm.ensureHover(candidate)
				if candidate.hoverToken != 0 {
					candidate.pendingHoverReattach = false
				}
			}
			if candidate.pendingFocusReattach && wm.focusStateMachine != nil {
				wm.focusStateMachine.attachGTKController(candidate)
				if candidate.focusControllerToken != 0 {
					candidate.pendingFocusReattach = false
				}
			}
		}
		wm.SetActivePane(newLeaf, SourceSplit)
		return false
	}, guardNodes...)

	return newLeaf, nil
}

// Helper functions for the simplified closePane implementation

// getSibling returns the sibling node for a given node
func (wm *WorkspaceManager) getSibling(node *paneNode) *paneNode {
	if node.parent == nil {
		return nil
	}
	if node.parent.left == node {
		return node.parent.right
	}
	return node.parent.left
}

// nextCleanupGeneration returns the next cleanup generation counter
func (wm *WorkspaceManager) nextCleanupGeneration() uint {
	wm.cleanupCounter++
	return wm.cleanupCounter
}

// promoteSibling promotes a sibling node to replace its parent in the tree
func (wm *WorkspaceManager) promoteSibling(grand *paneNode, parent *paneNode, sibling *paneNode) {
	if grand == nil {
		wm.root = sibling
		sibling.parent = nil
		return
	}
	sibling.parent = grand
	if grand.left == parent {
		grand.left = sibling
	} else {
		grand.right = sibling
	}
}

// swapContainers updates GTK widget hierarchy for promoted siblings
func (wm *WorkspaceManager) swapContainers(grand *paneNode, sibling *paneNode) {
	if grand == nil {
		// When TreeRebalancer is enabled, skip GTK attachment here as it will be
		// handled by the rebalancer's promotion transaction to avoid double-attachment
		if wm.treeRebalancer == nil || !wm.treeRebalancer.enabled {
			wm.attachRoot(sibling)
		}
		return
	}
	// Clear CSS classes before reparenting to avoid GTK bloom filter corruption
	if sibling.container != 0 && webkit.WidgetIsValid(sibling.container) {
		if webkit.WidgetHasCSSClass(sibling.container, activePaneClass) {
			webkit.WidgetRemoveCSSClass(sibling.container, activePaneClass)
		}
	}

	if webkit.WidgetIsValid(grand.container) && webkit.WidgetIsValid(sibling.container) {
		// GTK4 PanedSetStartChild/EndChild auto-unparent from current parent
		// Focus was already cleared before unparenting closed pane
		if parent := webkit.WidgetGetParent(sibling.container); parent != 0 && parent != grand.container {
			log.Printf("[workspace] widget %#x reparenting from %#x to %#x (GTK4 will auto-unparent)", sibling.container, parent, grand.container)
		}

		if grand.left == sibling {
			webkit.PanedSetStartChild(grand.container, sibling.container)
			log.Printf("[workspace] swapContainers: set widget %#x as start child of paned %#x", sibling.container, grand.container)
		} else {
			webkit.PanedSetEndChild(grand.container, sibling.container)
			log.Printf("[workspace] swapContainers: set widget %#x as end child of paned %#x", sibling.container, grand.container)
		}
		webkit.WidgetQueueAllocate(grand.container)
	}
}

// cascadePromotion handles the case where a paned node has only one child
// This happens when closing a pane leaves the grandparent with only one child
func (wm *WorkspaceManager) cascadePromotion(singleChildPaned *paneNode) {
	if singleChildPaned == nil {
		return
	}

	// Find the single child
	var onlyChild *paneNode
	if singleChildPaned.left != nil && singleChildPaned.right == nil {
		onlyChild = singleChildPaned.left
	} else if singleChildPaned.right != nil && singleChildPaned.left == nil {
		onlyChild = singleChildPaned.right
	} else {
		// Either no children or two children - nothing to cascade
		return
	}

	log.Printf("[workspace] Cascading promotion: replacing paned %p with its only child %p", singleChildPaned, onlyChild)

	greatGrandparent := singleChildPaned.parent

	// Update tree structure
	onlyChild.parent = greatGrandparent
	if greatGrandparent == nil {
		// Single child becomes new root
		wm.root = onlyChild
		log.Printf("[workspace] Only child %p promoted to root", onlyChild)

		// Attach to window
		if onlyChild.container != 0 && webkit.WidgetIsValid(onlyChild.container) {
			// GTK4 window.SetChild automatically handles unparenting
			wm.window.SetChild(onlyChild.container)
			webkit.WidgetSetHExpand(onlyChild.container, true)
			webkit.WidgetSetVExpand(onlyChild.container, true)
			webkit.WidgetQueueAllocate(onlyChild.container)
			webkit.WidgetShow(onlyChild.container)
		}

		// Cleanup the now-orphaned paned
		wm.decommissionParent(singleChildPaned, wm.cleanupCounter)
	} else {
		// Replace paned in great-grandparent
		if greatGrandparent.left == singleChildPaned {
			greatGrandparent.left = onlyChild
		} else {
			greatGrandparent.right = onlyChild
		}

		log.Printf("[workspace] Only child %p attached to great-grandparent %p", onlyChild, greatGrandparent)

		// Reparent widget in GTK
		if onlyChild.container != 0 && webkit.WidgetIsValid(onlyChild.container) && greatGrandparent.container != 0 && webkit.WidgetIsValid(greatGrandparent.container) {
			// GTK4 PanedSet*Child automatically handles unparenting
			if greatGrandparent.left == onlyChild {
				webkit.PanedSetStartChild(greatGrandparent.container, onlyChild.container)
			} else {
				webkit.PanedSetEndChild(greatGrandparent.container, onlyChild.container)
			}

			webkit.WidgetSetHExpand(onlyChild.container, true)
			webkit.WidgetSetVExpand(onlyChild.container, true)
			webkit.WidgetQueueAllocate(onlyChild.container)
			webkit.WidgetQueueAllocate(greatGrandparent.container)
			wm.scheduleIdleGuarded(func() bool {
				if onlyChild == nil || !onlyChild.widgetValid {
					return false
				}
				if onlyChild.container != 0 && webkit.WidgetIsValid(onlyChild.container) {
					webkit.WidgetShow(onlyChild.container)
					webkit.WidgetQueueResize(onlyChild.container)
					webkit.WidgetQueueDraw(onlyChild.container)
				}
				return false
			}, onlyChild)
		}

		// Cleanup the now-orphaned paned
		wm.decommissionParent(singleChildPaned, wm.cleanupCounter)

		// Check if great-grandparent now also has only one child (recursive case)
		if greatGrandparent != nil && ((greatGrandparent.left == nil) != (greatGrandparent.right == nil)) {
			log.Printf("[workspace] Great-grandparent %p also has only one child, continuing cascade", greatGrandparent)
			wm.cascadePromotion(greatGrandparent)
		}
	}
}

// attachRoot attaches a node as the new window root
func (wm *WorkspaceManager) attachRoot(root *paneNode) {
	if root == nil || root.container == 0 || wm.window == nil {
		return
	}
	if webkit.WidgetIsValid(root.container) {
		// GTK4 window.SetChild automatically handles unparenting from old parent
		// DO NOT unparent manually as it can destroy complex widgets like GtkBox
		wm.window.SetChild(root.container)
		webkit.WidgetQueueAllocate(root.container)
		webkit.WidgetShow(root.container)

		// Verify attachment succeeded
		if finalParent := webkit.WidgetGetParent(root.container); finalParent == 0 {
			log.Printf("[workspace] WARNING: widget %#x has no parent after SetChild", root.container)
		} else {
			log.Printf("[workspace] attachRoot successful: widget %#x now child of %#x", root.container, finalParent)
		}
	}
}

// cleanupPane safely cleans up a pane node with generation tracking
func (wm *WorkspaceManager) cleanupPane(node *paneNode, generation uint) {
	if node == nil {
		return
	}
	if !node.widgetValid {
		return
	}

	wm.cancelIdleHandles(node)

	node.widgetValid = false
	node.cleanupGeneration = generation

	if node.pane != nil {
		pane := node.pane
		pane.Cleanup()
		pane.CleanupFromWorkspace(wm)
		if pane.webView != nil {
			log.Printf("[workspace] releasing WebView reference: %p", pane.webView)
			pane.webView = nil
		}
		node.pane = nil
	}

	if node.container != 0 {
		// Container widget will be destroyed by GTK when unparented
		node.container = 0
	}

	node.pendingIdleHandles = nil
	node.parent = nil
	node.left = nil
	node.right = nil
}

// decommissionParent cleans up a parent node after promotion
func (wm *WorkspaceManager) decommissionParent(parent *paneNode, generation uint) {
	if parent == nil {
		return
	}

	// For branch nodes (paneds), destroy the widget
	// Children have already been unparented during promotion/close, no need to clear them
	if !parent.isLeaf && parent.container != 0 && webkit.WidgetIsValid(parent.container) {
		webkit.WidgetHookDestroy(parent.container)
		log.Printf("[workspace] destroyed paned widget: %#x", parent.container)
	}

	wm.cleanupPane(parent, generation)
}

// promoteNewRoot handles root replacement when closing the root pane
func (wm *WorkspaceManager) promoteNewRoot(ctx closeContext, oldRoot *paneNode) (*paneNode, error) {
	candidate := wm.findReplacementRoot(oldRoot)
	if candidate == nil {
		return wm.cleanupAndExit(oldRoot)
	}

	sibling := wm.getSibling(candidate)
	if sibling != nil {
		wm.promoteSibling(candidate.parent.parent, candidate.parent, sibling)
	}

	candidate.parent = nil
	wm.root = candidate

	wm.attachRoot(candidate)

	wm.cleanupPane(oldRoot, ctx.Generation())
	return candidate, nil
}

// cleanupAndExit handles the final pane cleanup and application exit
func (wm *WorkspaceManager) cleanupAndExit(node *paneNode) (*paneNode, error) {
	log.Printf("[workspace] closing final pane; exiting browser")
	wm.cleanupPane(node, wm.nextCleanupGeneration())
	wm.detachHover(node)
	webkit.QuitMainLoop()
	return nil, nil
}

// setFocusToLeaf sets focus to the leftmost leaf of a node
func (wm *WorkspaceManager) setFocusToLeaf(node *paneNode) {
	focusTarget := wm.leftmostLeaf(node)
	if focusTarget != nil {
		wm.SetActivePane(focusTarget, SourceClose)
	}
}

// closePane closes a specific pane and handles tree restructuring (simplified implementation)
func (wm *WorkspaceManager) closePane(node *paneNode) (*paneNode, error) {
	ctx := wm.beginClose(node)
	defer ctx.finish()

	// STEP 1: Basic validation (quick fail)
	if ctx.err != nil {
		return nil, ctx.err
	}

	// STEP 2: Handle stacked panes via compatibility shim
	if node.parent != nil && node.parent.isStacked {
		return wm.closeStackedPaneCompat(node)
	}

	// Ensure we detach hover/focus controllers before the widget hierarchy changes.
	wm.detachHover(node)
	wm.detachFocus(node)

	// STEP 3: Handle trivial exit cases
	if ctx.remaining == 1 {
		return wm.cleanupAndExit(node)
	}
	if node == wm.root {
		return wm.promoteNewRoot(ctx, node)
	}

	// STEP 4: Unparent closed pane FIRST to avoid leaving paned with single child
	parent := node.parent
	sibling := wm.getSibling(node)
	grandparent := parent.parent

	// Clear grandparent's focus child BEFORE unparenting closed pane
	// GTK propagates focus events upward, so we must clear grandparent focus first
	if grandparent != nil && grandparent.container != 0 && webkit.WidgetIsValid(grandparent.container) {
		webkit.WidgetSetFocusChild(grandparent.container, 0)
	}

	// Unparent the closed pane's container from the paned BEFORE promoting sibling
	if node.container != 0 && webkit.WidgetIsValid(node.container) {
		if containerParent := webkit.WidgetGetParent(node.container); containerParent == parent.container {
			if parent.left == node {
				webkit.PanedSetStartChild(parent.container, 0)
			} else if parent.right == node {
				webkit.PanedSetEndChild(parent.container, 0)
			}
			log.Printf("[workspace] unparented closed pane container: %#x", node.container)
		}
	}

	// STEP 5: Promote sibling in-place (tree structure)
	wm.promoteSibling(grandparent, parent, sibling)

	// STEP 6: GTK handles all reparenting automatically
	// PanedSetStartChild/EndChild and window.SetChild auto-unparent widgets
	wm.swapContainers(grandparent, sibling)

	// Reconnect the promoted sibling's rendering pipeline after GTK reparenting.
	if sibling != nil && sibling.container != 0 {
		wm.scheduleIdleGuarded(func() bool {
			if sibling == nil || !sibling.widgetValid {
				return false
			}
			if sibling.container != 0 && webkit.WidgetIsValid(sibling.container) {
				webkit.WidgetShow(sibling.container)
				webkit.WidgetQueueResize(sibling.container)
				webkit.WidgetQueueDraw(sibling.container)
			}
			return false
		}, sibling)
	}

	// STEP 7: Check if grandparent now has only one child (cascade promotion needed)
	if grandparent != nil && ((grandparent.left == nil) != (grandparent.right == nil)) {
		// Grandparent has exactly one child - it should be promoted too
		log.Printf("[workspace] Grandparent %p now has only one child, cascading promotion", grandparent)
		wm.cascadePromotion(grandparent)
	}

	// STEP 8: Cleanup & focus
	wm.cleanupPane(node, ctx.Generation())
	wm.decommissionParent(parent, ctx.Generation())
	if sibling != nil {
		wm.scheduleIdleGuarded(func() bool {
			if sibling == nil || !sibling.widgetValid {
				return false
			}
			wm.setFocusToLeaf(sibling)
			return false
		}, sibling)
	}

	return sibling, nil
}

// closeStackedPaneCompat provides compatibility layer for closing stacked panes
func (wm *WorkspaceManager) closeStackedPaneCompat(node *paneNode) (*paneNode, error) {
	stack := node.parent

	// Find index
	index := -1
	for i, pane := range stack.stackedPanes {
		if pane == node {
			index = i
			break
		}
	}

	if index == -1 {
		return nil, errors.New("pane not in stack")
	}

	// Remove from array
	stack.stackedPanes = append(
		stack.stackedPanes[:index],
		stack.stackedPanes[index+1:]...,
	)

	// If only one pane left, unstack it
	if len(stack.stackedPanes) == 1 {
		remaining := stack.stackedPanes[0]

		// Replace stack with remaining pane in tree
		remaining.parent = stack.parent
		if stack.parent != nil {
			if stack.parent.left == stack {
				stack.parent.left = remaining
			} else {
				stack.parent.right = remaining
			}

			// Update GTK widgets
			if webkit.WidgetIsValid(stack.parent.container) && webkit.WidgetIsValid(remaining.container) {
				if stack.parent.left == remaining {
					webkit.PanedSetStartChild(stack.parent.container, remaining.container)
				} else {
					webkit.PanedSetEndChild(stack.parent.container, remaining.container)
				}
			}
		} else {
			// Stack was root
			wm.root = remaining
			if webkit.WidgetIsValid(remaining.container) {
				wm.window.SetChild(remaining.container)
			}
		}

		// Cleanup stack container
		wm.cleanupPane(stack, wm.nextCleanupGeneration())

		return remaining, nil
	}

	// Update active index if needed
	if stack.activeStackIndex >= len(stack.stackedPanes) {
		stack.activeStackIndex = len(stack.stackedPanes) - 1
	}

	// Remove widget from GTK box
	if webkit.WidgetIsValid(stack.container) && webkit.WidgetIsValid(node.container) {
		webkit.BoxRemove(stack.container, node.container)
	}

	// Show new active pane
	if stack.activeStackIndex >= 0 {
		active := stack.stackedPanes[stack.activeStackIndex]
		if webkit.WidgetIsValid(active.container) {
			webkit.WidgetShow(active.container)
		}
	}

	// Cleanup closed pane
	wm.cleanupPane(node, wm.nextCleanupGeneration())

	return stack, nil
}
