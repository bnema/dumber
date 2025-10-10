// workspace_pane_ops.go - Pane creation, splitting, closing and tree operations
package browser

import (
	"errors"
	"log"

	"github.com/bnema/dumber/pkg/webkit"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
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

	// Omnibox will auto-open on about:blank via client-side detection
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
		if node.isStacked {
			if node.activeStackIndex >= 0 && node.activeStackIndex < len(node.stackedPanes) {
				candidate := node.stackedPanes[node.activeStackIndex]
				if candidate != nil {
					return candidate
				}
			}
			if len(node.stackedPanes) > 0 {
				return node.stackedPanes[0]
			}
			return nil
		}
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
	if newContainer == nil {
		return errors.New("new pane missing container")
	}

	webkit.WidgetSetHExpand(newContainer, true)
	webkit.WidgetSetVExpand(newContainer, true)
	webkit.WidgetRealize(newContainer)

	// Also realize the WebView widget itself for proper popup rendering
	webViewWidget := newPane.webView.Widget()
	if webViewWidget != nil {
		webkit.WidgetRealize(webViewWidget)
	}

	// CRITICAL: Handle stacked panes correctly - same logic as splitNode
	// When the target is inside a stacked pane, we need to split around the entire stack,
	// not the individual pane within the stack
	originalTarget := target
	var stackContainer *paneNode
	if target.parent != nil && target.parent.isStacked {
		// Target is in a stack - we'll create the split around the entire stack
		stackContainer = target.parent
		log.Printf("[workspace] target is in stack, will split around the stack: originalTarget=%p stackContainer=%p", originalTarget, stackContainer)
	}

	// Determine what we're actually splitting
	var splitTarget *paneNode
	var splitTargetContainer gtk.Widgetter

	if stackContainer != nil {
		// Case: splitting from inside a stack - we split around the entire stack
		splitTarget = stackContainer
		splitTargetContainer = stackContainer.container
		log.Printf("[workspace] splitting around stack: stackContainer=%p", splitTarget)
	} else {
		// Case: normal split from a simple pane or direct stack target
		splitTarget = target
		splitTargetContainer = target.container
		log.Printf("[workspace] normal split from pane: target=%p", splitTarget)
	}

	if splitTargetContainer == nil {
		return errors.New("split target missing container")
	}

	orientation, existingFirst := mapDirection(direction)
	log.Printf("[workspace] inserting popup direction=%s orientation=%v existingFirst=%v splitTarget.parent=%p", direction, orientation, existingFirst, splitTarget.parent)

	paned := gtk.NewPaned(orientation)
	if paned == nil {
		return errors.New("failed to create GtkPaned")
	}
	paned.SetHExpand(true)
	paned.SetVExpand(true)
	// For popup panes: preserve existing pane size, let popup take remaining space
	paned.SetResizeStartChild(false) // existing pane keeps its size
	paned.SetResizeEndChild(true)    // popup can resize
	log.Printf("[workspace] configured paned for popup: start=fixed, end=flexible")

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

	// CRITICAL: Capture parent paned's divider position BEFORE reparenting to preserve layout
	// Only attempt this if the parent container is actually a paned widget
	var parentDividerPos int
	if parent != nil && parent.container != nil {
		if parentPaned, ok := parent.container.(*gtk.Paned); ok && parentPaned != nil {
			parentDividerPos = parentPaned.Position()
			log.Printf("[workspace] captured parent divider position: %d", parentDividerPos)
		} else {
			log.Printf("[workspace] parent container is not a paned widget (likely a stacked container)")
		}
	}

	// Clear CSS classes before reparenting to avoid GTK bloom filter corruption
	if splitTargetContainer != nil {
		// Remove active border class if present (prevents GTK bloom filter corruption during unparent)
		if webkit.WidgetHasCSSClass(splitTargetContainer, activePaneClass) {
			webkit.WidgetRemoveCSSClass(splitTargetContainer, activePaneClass)
		}

		// CRITICAL: Hide widget before reparenting to disconnect WebKitGTK rendering pipeline
		// This forces WebKit to detach its compositor, preventing rendering corruption
		webkit.WidgetHide(splitTargetContainer)
		log.Printf("[workspace] Hidden split target container before reparenting: %p", splitTargetContainer)
	}

	// Prevent GTK from auto-removing controllers while the widget is temporarily unparented.
	wm.safelyDetachControllersBeforeReparent(splitTarget)

	// Detach existing container from its current GTK parent before inserting into new paned.
	if parent == nil {
		// Split target is the root - remove it from the window
		log.Printf("[workspace] removing split target container=%p from window", splitTargetContainer)
		if wm.window != nil {
			wm.window.SetChild(nil)
		}
		// GTK4 will auto-unparent when we add to new paned - no manual unparent needed
	} else if parent.container != nil {
		// Split target has a parent paned - unparent it from there
		log.Printf("[workspace] unparenting split target container from parent paned")
		if parentPaned, ok := parent.container.(*gtk.Paned); ok && parentPaned != nil {
			log.Printf("[workspace] executing unparent: parentPtr=%p", parent.container)
			if parent.left == splitTarget {
				parentPaned.SetStartChild(nil)
			} else if parent.right == splitTarget {
				parentPaned.SetEndChild(nil)
			}
			webkit.WidgetQueueAllocate(parent.container)
		}
	}

	// Add both containers to the new paned
	if existingFirst {
		split.left = splitTarget
		split.right = newLeaf
		paned.SetStartChild(splitTargetContainer)
		paned.SetEndChild(newContainer)
		log.Printf("[workspace] added splitTarget=%p as start child, new=%p as end child", splitTargetContainer, newContainer)
	} else {
		split.left = newLeaf
		split.right = splitTarget
		paned.SetStartChild(newContainer)
		paned.SetEndChild(splitTargetContainer)
		log.Printf("[workspace] added new=%p as start child, splitTarget=%p as end child", newContainer, splitTargetContainer)
	}

	splitTarget.parent = split
	newLeaf.parent = split

	if parent == nil {
		wm.root = split
		if wm.window != nil {
			wm.window.SetChild(paned)
			paned.QueueAllocate()
			log.Printf("[workspace] paned set as window child: paned=%p", paned)
		}
	} else {
		if parentPaned, ok := parent.container.(*gtk.Paned); ok && parentPaned != nil {
			log.Printf("[workspace] executing parent reparent: parentPtr=%p, paned=%p", parent.container, paned)
			if parent.left == splitTarget {
				parent.left = split
				parentPaned.SetStartChild(paned)
			} else if parent.right == splitTarget {
				parent.right = split
				parentPaned.SetEndChild(paned)
			}
			webkit.WidgetQueueAllocate(parent.container)
			paned.QueueAllocate()
			log.Printf("[workspace] paned inserted into parent successfully")

			// CRITICAL: Restore parent paned's divider position to preserve existing layout
			// Only attempt this if we captured a position and the parent is actually a paned widget
			if parentDividerPos > 0 {
				parentPaned.SetPosition(parentDividerPos)
				log.Printf("[workspace] restored parent divider position: %d", parentDividerPos)
			}
		}
	}

	paned.Show()

	// Force GTK to re-layout the new GtkPaned container once both children are attached.
	wm.scheduleIdleGuarded(func() bool {
		if split == nil || !split.widgetValid {
			return false
		}
		if split.container != nil {
			webkit.WidgetQueueResize(split.container)
			webkit.WidgetQueueDraw(split.container)
		}
		return false
	}, split)

	// CRITICAL: Set initial 50/50 split position after showing paned
	// GTK needs the widget to be realized before we can set position based on allocation
	// Schedule this to run after GTK has allocated space to the paned
	wm.scheduleIdleGuarded(func() bool {
		if paned == nil {
			return false
		}
		// Get paned allocation to calculate 50% position
		alloc := paned.Allocation()
		var splitPos int
		if orientation == gtk.OrientationHorizontal {
			splitPos = alloc.Width() / 2
		} else {
			splitPos = alloc.Height() / 2
		}
		if splitPos > 0 {
			paned.SetPosition(splitPos)
			log.Printf("[workspace] Set initial paned position to 50%%: %d (orientation=%d, size=%dx%d)", splitPos, orientation, alloc.Width(), alloc.Height())
		}
		return false
	}, split)

	// CRITICAL: Show the split target container and force GTK to recreate rendering surface after reparenting
	// This reconnects WebKitGTK's rendering pipeline and fixes compositor sync issues
	if splitTarget != nil && splitTarget.container != nil {
		wm.scheduleIdleGuarded(func() bool {
			if splitTarget == nil || !splitTarget.widgetValid || splitTarget.container == nil {
				return false
			}
			// Show widget to reconnect WebKit rendering pipeline
			webkit.WidgetShow(splitTarget.container)
			// Force GTK to recalculate size and recreate rendering surface
			webkit.WidgetQueueResize(splitTarget.container)
			webkit.WidgetQueueDraw(splitTarget.container)
			log.Printf("[workspace] Shown and queued resize+draw for split target container after reparenting")
			return false
		}, splitTarget)
	}

	wm.viewToNode[newPane.webView] = newLeaf
	wm.ensureHover(newLeaf)
	wm.ensureHover(splitTarget)
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
		if newContainer != nil {
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
	var splitTargetContainer gtk.Widgetter

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
	if newContainer == nil {
		return nil, errors.New("new pane missing container")
	}
	webkit.WidgetSetHExpand(newContainer, true)
	webkit.WidgetSetVExpand(newContainer, true)
	webkit.WidgetRealize(newContainer)

	// Use the determined split target container
	if splitTargetContainer == nil {
		return nil, errors.New("split target missing container")
	}

	// Only apply GTK operations if this is a simple widget (leaf pane)
	// For stacked containers, we need to be more careful
	if splitTarget.isLeaf {
		if splitTargetContainer != nil {
			webkit.WidgetSetHExpand(splitTargetContainer, true)
			webkit.WidgetSetVExpand(splitTargetContainer, true)
			webkit.WidgetRealize(splitTargetContainer)
		}
	} else if splitTarget.isStacked {
		// Stack containers need the same setup as regular panes for proper splitting
		if splitTargetContainer != nil {
			webkit.WidgetSetHExpand(splitTargetContainer, true)
			webkit.WidgetSetVExpand(splitTargetContainer, true)
			webkit.WidgetRealize(splitTargetContainer)
			log.Printf("[workspace] configured stack wrapper for split: %p", splitTargetContainer)
		}
	}

	orientation, existingFirst := mapDirection(direction)
	log.Printf("[workspace] splitting direction=%s orientation=%v existingFirst=%v splitTarget.parent=%p splitTarget.isStacked=%v",
		direction, orientation, existingFirst, splitTarget.parent, splitTarget.isStacked)

	paned := gtk.NewPaned(orientation)
	if paned == nil {
		return nil, errors.New("failed to create GtkPaned")
	}
	paned.SetHExpand(true)
	paned.SetVExpand(true)
	paned.SetResizeStartChild(true)
	paned.SetResizeEndChild(true)

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
	if splitTargetContainer != nil {
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
		log.Printf("[workspace] removing split target container=%p from window", splitTargetContainer)
		if wm.window != nil {
			wm.window.SetChild(nil)
		}
		// GTK4 will auto-unparent when we add to new paned - no manual unparent needed
	} else if parent.container != nil {
		// Split target has a parent paned - remove it (automatically unparents in GTK4)
		log.Printf("[workspace] unparenting split target container=%p from parent paned=%p", splitTargetContainer, parent.container)
		if parentPaned, ok := parent.container.(*gtk.Paned); ok && parentPaned != nil {
			if parent.left == splitTarget {
				parentPaned.SetStartChild(nil)
			} else if parent.right == splitTarget {
				parentPaned.SetEndChild(nil)
			}
			// GTK4 automatically unparents when we set paned child to nil - no manual unparent needed
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
		if splitTargetContainer != nil {
			paned.SetStartChild(splitTargetContainer)
			paned.SetEndChild(newContainer)
			log.Printf("[workspace] added splitTarget=%p as start child, new=%p as end child", splitTargetContainer, newContainer)
		}
	} else {
		if splitTargetContainer != nil {
			paned.SetStartChild(newContainer)
			paned.SetEndChild(splitTargetContainer)
			log.Printf("[workspace] added new=%p as start child, splitTarget=%p as end child", newContainer, splitTargetContainer)
		}
	}

	// Attach the new paned to its parent
	if parent == nil {
		if wm.window != nil {
			wm.window.SetChild(paned)
			paned.QueueAllocate()
			log.Printf("[workspace] paned set as window child: paned=%p", paned)
		}
	} else {
		if parentPaned, ok := parent.container.(*gtk.Paned); ok && parentPaned != nil {
			if parent.left == split {
				parentPaned.SetStartChild(paned)
			} else if parent.right == split {
				parentPaned.SetEndChild(paned)
			}
			webkit.WidgetQueueAllocate(parent.container)
			paned.QueueAllocate()
			log.Printf("[workspace] paned inserted into parent=%p", parent.container)
		}
	}

	paned.Show()

	// CRITICAL: Set initial 50/50 split position after showing paned
	// GTK needs the widget to be realized before we can set position based on allocation
	// Schedule this to run after GTK has allocated space to the paned
	wm.scheduleIdleGuarded(func() bool {
		if paned == nil {
			return false
		}
		// Get paned allocation to calculate 50% position
		alloc := paned.Allocation()
		var splitPos int
		if orientation == gtk.OrientationHorizontal {
			splitPos = alloc.Width() / 2
		} else {
			splitPos = alloc.Height() / 2
		}
		if splitPos > 0 {
			paned.SetPosition(splitPos)
			log.Printf("[workspace] Set initial paned position to 50%%: %d (orientation=%d, size=%dx%d)", splitPos, orientation, alloc.Width(), alloc.Height())
		}
		return false
	}, split)

	// CRITICAL: Show the existing container and force GTK to recreate rendering surface after reparenting
	// This reconnects WebKitGTK's rendering pipeline and fixes compositor sync issues
	if target != nil && target.container != nil {
		wm.scheduleIdleGuarded(func() bool {
			if target == nil || !target.widgetValid || target.container == nil {
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
		if newContainer != nil {
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
	if sibling.container != nil {
		if webkit.WidgetHasCSSClass(sibling.container, activePaneClass) {
			webkit.WidgetRemoveCSSClass(sibling.container, activePaneClass)
		}
	}

	if grandPaned, ok := grand.container.(*gtk.Paned); ok && grandPaned != nil && sibling.container != nil {
		// GTK4 PanedSetStartChild/EndChild auto-unparent from current parent
		// Focus was already cleared before unparenting closed pane
		if parent := webkit.WidgetGetParent(sibling.container); parent != nil && parent != grand.container {
			log.Printf("[workspace] widget %p reparenting from %p to %p (GTK4 will auto-unparent)", sibling.container, parent, grand.container)
		}

		if grand.left == sibling {
			grandPaned.SetStartChild(sibling.container)
			log.Printf("[workspace] swapContainers: set widget %p as start child of paned %p", sibling.container, grand.container)
		} else {
			grandPaned.SetEndChild(sibling.container)
			log.Printf("[workspace] swapContainers: set widget %p as end child of paned %p", sibling.container, grand.container)
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
		if onlyChild.container != nil {
			wm.attachRoot(onlyChild)
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
		if greatGrandPaned, ok := greatGrandparent.container.(*gtk.Paned); ok && greatGrandPaned != nil && onlyChild.container != nil {
			// GTK4 PanedSet*Child automatically handles unparenting
			if greatGrandparent.left == onlyChild {
				greatGrandPaned.SetStartChild(onlyChild.container)
			} else {
				greatGrandPaned.SetEndChild(onlyChild.container)
			}

			webkit.WidgetSetHExpand(onlyChild.container, true)
			webkit.WidgetSetVExpand(onlyChild.container, true)
			webkit.WidgetQueueAllocate(onlyChild.container)
			webkit.WidgetQueueAllocate(greatGrandparent.container)
			wm.scheduleIdleGuarded(func() bool {
				if onlyChild == nil || !onlyChild.widgetValid {
					return false
				}
				if onlyChild.container != nil {
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
	if root == nil || root.container == nil || wm.window == nil {
		return
	}

	// Note: In gotk4, reference counting is handled automatically by Go's GC
	// No need for manual Ref()/Unref() calls

	// Detach from previous container (paned, stack, etc.) before replacing the window child.
	// While GTK4 auto-unparents for paned operations, window.SetChild requires manual unparent
	if parent := webkit.WidgetGetParent(root.container); parent != nil {
		log.Printf("[workspace] unparenting widget %p from parent %p before window attach", root.container, parent)
		webkit.WidgetUnparent(root.container)
		// Note: In gotk4, the Go object reference remains valid after unparent
		// Only the GTK parent relationship is cleared
	}

	// Clear any existing window child to avoid GTK warnings when swapping roots.
	wm.window.SetChild(nil)

	if root.container != nil {
		webkit.WidgetSetHExpand(root.container, true)
		webkit.WidgetSetVExpand(root.container, true)
	}

	// GTK4 will automatically unparent root.container from its previous parent when we attach it here
	wm.window.SetChild(root.container)
	if root.container != nil {
		webkit.WidgetQueueAllocate(root.container)
		webkit.WidgetShow(root.container)
		webkit.WidgetQueueResize(root.container)
		webkit.WidgetQueueDraw(root.container)
	}

	wm.scheduleIdleGuarded(func() bool {
		if root == nil || !root.widgetValid {
			return false
		}
		if root.container != nil {
			webkit.WidgetQueueResize(root.container)
			webkit.WidgetQueueDraw(root.container)
		}
		return false
	}, root)

	// Verify attachment succeeded
	if finalParent := webkit.WidgetGetParent(root.container); finalParent == nil {
		log.Printf("[workspace] WARNING: widget %p has no parent after SetChild", root.container)
	} else {
		log.Printf("[workspace] attachRoot successful: widget %p now child of %p", root.container, finalParent)
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

	if wm.focusStateMachine != nil {
		wm.focusStateMachine.InvalidateActivePane(node)
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

	if node.container != nil {
		// Container widget will be destroyed by GTK when unparented
		node.container = nil
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
	if !parent.isLeaf && parent.container != nil {
		// In gotk4, we don't need to manually destroy - GTK handles this when unparented
		// Just mark it for cleanup
		log.Printf("[workspace] marked paned widget for cleanup: %p", parent.container)
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
	// TODO: Implement proper application quit mechanism
	// For now, the window close will trigger GTK application quit
	if wm.window != nil && wm.window.AsWindow() != nil {
		wm.window.AsWindow().Close()
	}
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
	if grandparent != nil && grandparent.container != nil {
		webkit.WidgetSetFocusChild(grandparent.container, nil)
	}

	// Unparent the closed pane's container from the paned BEFORE promoting sibling
	if node.container != nil {
		if containerParent := webkit.WidgetGetParent(node.container); containerParent == parent.container {
			if parentPaned, ok := parent.container.(*gtk.Paned); ok && parentPaned != nil {
				if parent.left == node {
					parentPaned.SetStartChild(nil)
				} else if parent.right == node {
					parentPaned.SetEndChild(nil)
				}
				log.Printf("[workspace] unparented closed pane container: %p", node.container)
			}
		}
	}

	// STEP 5: Unparent sibling from parent paned BEFORE promotion
	// The sibling needs to be detached from the parent paned before we can attach it elsewhere
	if sibling.container != nil && parent.container != nil {
		if parentPaned, ok := parent.container.(*gtk.Paned); ok && parentPaned != nil {
			if parent.left == sibling {
				parentPaned.SetStartChild(nil)
			} else if parent.right == sibling {
				parentPaned.SetEndChild(nil)
			}
			log.Printf("[workspace] unparented sibling container %p from parent paned %p", sibling.container, parent.container)
		}
	}

	// STEP 6: Promote sibling in-place (tree structure)
	wm.promoteSibling(grandparent, parent, sibling)

	// STEP 7: GTK widget reparenting
	// For paned containers: GTK4 PanedSetStartChild/EndChild auto-unparent from current parent
	// For window: we already manually unparented above, so now we can safely attach
	wm.swapContainers(grandparent, sibling)

	// Reconnect the promoted sibling's rendering pipeline after GTK reparenting.
	if sibling != nil && sibling.container != nil {
		wm.scheduleIdleGuarded(func() bool {
			if sibling == nil || !sibling.widgetValid {
				return false
			}
			if sibling.container != nil {
				webkit.WidgetShow(sibling.container)
				webkit.WidgetQueueResize(sibling.container)
				webkit.WidgetQueueDraw(sibling.container)
			}
			return false
		}, sibling)
	}

	// STEP 8: Check if grandparent now has only one child (cascade promotion needed)
	if grandparent != nil && ((grandparent.left == nil) != (grandparent.right == nil)) {
		// Grandparent has exactly one child - it should be promoted too
		log.Printf("[workspace] Grandparent %p now has only one child, cascading promotion", grandparent)
		wm.cascadePromotion(grandparent)
	}

	// STEP 9: Cleanup & focus
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
			if parentPaned, ok := stack.parent.container.(*gtk.Paned); ok && parentPaned != nil && remaining.container != nil {
				if stack.parent.left == remaining {
					parentPaned.SetStartChild(remaining.container)
				} else {
					parentPaned.SetEndChild(remaining.container)
				}
			}
		} else {
			// Stack was root
			wm.root = remaining
			if remaining.container != nil {
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
	if stackBox, ok := stack.container.(*gtk.Box); ok && stackBox != nil && node.container != nil {
		stackBox.Remove(node.container)
	}

	// Show new active pane
	if stack.activeStackIndex >= 0 {
		active := stack.stackedPanes[stack.activeStackIndex]
		if active.container != nil {
			webkit.WidgetShow(active.container)
		}
	}

	// Cleanup closed pane
	wm.cleanupPane(node, wm.nextCleanupGeneration())

	return stack, nil
}
