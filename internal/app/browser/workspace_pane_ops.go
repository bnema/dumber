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
			webkit.IdleAdd(func() bool {
				if injErr := target.pane.webView.InjectScript("window.__dumber_omnibox?.open('omnibox');"); injErr != nil {
					log.Printf("[workspace] failed to open omnibox: %v", injErr)
				}
				return false // Remove idle callback
			})
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

	if target.container == nil {
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
	webkit.PanedSetResizeStart(paned, true)
	webkit.PanedSetResizeEnd(paned, true)

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

	// Prevent GTK from auto-removing controllers while the widget is temporarily unparented.
	wm.safelyDetachControllersBeforeReparent(target)

	// Detach existing container from its current GTK parent before inserting into new paned.
	existingContainer := target.getContainerPtr()
	if parent == nil {
		// Target is the root - remove it from the window
		log.Printf("[workspace] removing existing container=%#x from window", existingContainer)
		if wm.window != nil {
			wm.window.SetChild(0)
		}
	} else if parent.container != nil {
		// Target has a parent paned - unparent it from there
		log.Printf("[workspace] unparenting existing container from parent paned")
		err := parent.container.Execute(func(parentPtr uintptr) error {
			log.Printf("[workspace] executing unparent: parentPtr=%#x", parentPtr)
			if parent.left == target {
				webkit.PanedSetStartChild(parentPtr, 0)
			} else if parent.right == target {
				webkit.PanedSetEndChild(parentPtr, 0)
			}
			webkit.WidgetQueueAllocate(parentPtr)
			return nil
		})
		if err != nil {
			log.Printf("[workspace] error during parent unparent: %v", err)
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
		err := parent.container.Execute(func(parentPtr uintptr) error {
			log.Printf("[workspace] executing parent reparent: parentPtr=%#x, paned=%#x", parentPtr, paned)
			if parent.left == target {
				parent.left = split
				webkit.PanedSetStartChild(parentPtr, paned)
			} else if parent.right == target {
				parent.right = split
				webkit.PanedSetEndChild(parentPtr, paned)
			}
			webkit.WidgetQueueAllocate(parentPtr)
			webkit.WidgetQueueAllocate(paned)
			log.Printf("[workspace] paned inserted into parent successfully")
			return nil
		})
		if err != nil {
			log.Printf("[workspace] error during parent reparent: %v", err)
		}
	}

	webkit.WidgetShow(paned)

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

	webkit.IdleAdd(func() bool {
		if newContainer != 0 {
			webkit.WidgetShow(newContainer)
		}
		// Attach GTK focus controller to new pane
		if wm.focusStateMachine != nil {
			wm.focusStateMachine.attachGTKController(newLeaf)
		}

		for _, candidate := range reattachTargets {
			if candidate == nil {
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
	})

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
	var splitTargetContainer *SafeWidget

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
	if splitTargetContainer == nil {
		return nil, errors.New("split target missing container")
	}

	// Only apply GTK operations if this is a simple widget (leaf pane)
	// For stacked containers, we need to be more careful
	if splitTarget.isLeaf {
		splitTargetContainer.Execute(func(containerPtr uintptr) error {
			webkit.WidgetSetHExpand(containerPtr, true)
			webkit.WidgetSetVExpand(containerPtr, true)
			webkit.WidgetRealizeInContainer(containerPtr)
			return nil
		})
	} else if splitTarget.isStacked {
		// Stack containers need the same setup as regular panes for proper splitting
		splitTargetContainer.Execute(func(containerPtr uintptr) error {
			webkit.WidgetSetHExpand(containerPtr, true)
			webkit.WidgetSetVExpand(containerPtr, true)
			webkit.WidgetRealizeInContainer(containerPtr)
			log.Printf("[workspace] configured stack wrapper for split: %#x", containerPtr)
			return nil
		})
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

	// Detach split target container from its current GTK parent before inserting into new paned.
	if parent == nil {
		// Split target is the root - remove it from the window
		log.Printf("[workspace] removing split target container=%#x from window", splitTargetContainer)
		if wm.window != nil {
			wm.window.SetChild(0)
		}
		// For root widgets, only unparent if they actually have a GTK parent
		splitTargetContainer.Execute(func(containerPtr uintptr) error {
			if webkit.WidgetGetParent(containerPtr) != 0 {
				webkit.WidgetUnparent(containerPtr)
			}
			return nil
		})
	} else if parent.container != nil {
		// Split target has a parent paned - remove it (automatically unparents in GTK4)
		log.Printf("[workspace] unparenting split target container=%#x from parent paned=%#x", splitTargetContainer, parent.container)
		parent.container.Execute(func(panedPtr uintptr) error {
			if parent.left == splitTarget {
				webkit.PanedSetStartChild(panedPtr, 0)
			} else if parent.right == splitTarget {
				webkit.PanedSetEndChild(panedPtr, 0)
			}
			// GTK4 automatically unparents when we set paned child to 0 - no manual unparent needed
			webkit.WidgetQueueAllocate(panedPtr)
			return nil
		})
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
		splitTargetContainer.Execute(func(splitPtr uintptr) error {
			webkit.PanedSetStartChild(paned, splitPtr)
			webkit.PanedSetEndChild(paned, newContainer)
			log.Printf("[workspace] added splitTarget=%#x as start child, new=%#x as end child", splitPtr, newContainer)
			return nil
		})
	} else {
		splitTargetContainer.Execute(func(splitPtr uintptr) error {
			webkit.PanedSetStartChild(paned, newContainer)
			webkit.PanedSetEndChild(paned, splitPtr)
			log.Printf("[workspace] added new=%#x as start child, splitTarget=%#x as end child", newContainer, splitPtr)
			return nil
		})
	}

	// Attach the new paned to its parent
	if parent == nil {
		if wm.window != nil {
			wm.window.SetChild(paned)
			webkit.WidgetQueueAllocate(paned)
			log.Printf("[workspace] paned set as window child: paned=%#x", paned)
		}
	} else {
		parent.container.Execute(func(parentPtr uintptr) error {
			if parent.left == split {
				webkit.PanedSetStartChild(parentPtr, paned)
			} else if parent.right == split {
				webkit.PanedSetEndChild(parentPtr, paned)
			}
			webkit.WidgetQueueAllocate(parentPtr)
			webkit.WidgetQueueAllocate(paned)
			log.Printf("[workspace] paned inserted into parent=%#x", parentPtr)
			return nil
		})
	}

	webkit.WidgetShow(paned)

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

	webkit.IdleAdd(func() bool {
		if newContainer != 0 {
			webkit.WidgetShow(newContainer)
		}
		// Attach GTK focus controller to new pane
		if wm.focusStateMachine != nil {
			wm.focusStateMachine.attachGTKController(newLeaf)
		}

		for _, candidate := range reattachTargets {
			if candidate == nil {
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
	})

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

// ensureWidgets validates and recovers SafeWidget instances
func (wm *WorkspaceManager) ensureWidgets(nodes ...*paneNode) {
	for _, n := range nodes {
		if n == nil || n.container == nil {
			continue
		}
		if n.container.IsValid() {
			continue
		}
		ptr := n.container.Ptr()
		typeInfo := n.container.typeInfo
		n.container = wm.widgetRegistry.Register(ptr, typeInfo)
	}
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
	grand.container.Execute(func(gPtr uintptr) error {
		sibling.container.Execute(func(sPtr uintptr) error {
			// GTK4 auto-unparents when setting paned child
			// But we should verify the widget is ready for reparenting
			if parent := webkit.WidgetGetParent(sPtr); parent != 0 && parent != gPtr {
				log.Printf("[workspace] widget %#x has unexpected parent %#x, expected %#x or 0", sPtr, parent, gPtr)
			}

			if grand.left == sibling {
				webkit.PanedSetStartChild(gPtr, sPtr)
				log.Printf("[workspace] swapContainers: set widget %#x as start child of paned %#x", sPtr, gPtr)
			} else {
				webkit.PanedSetEndChild(gPtr, sPtr)
				log.Printf("[workspace] swapContainers: set widget %#x as end child of paned %#x", sPtr, gPtr)
			}
			webkit.WidgetQueueAllocate(gPtr)
			return nil
		})
		return nil
	})
}

// attachRoot attaches a node as the new window root
func (wm *WorkspaceManager) attachRoot(root *paneNode) {
	if root == nil || root.container == nil || wm.window == nil {
		return
	}
	root.container.Execute(func(ptr uintptr) error {
		// Check if widget has a parent and unparent it first
		if parent := webkit.WidgetGetParent(ptr); parent != 0 {
			log.Printf("[workspace] unparenting widget %#x from parent %#x before window attach", ptr, parent)
			webkit.WidgetUnparent(ptr)
		}

		// Now safe to set as window child
		wm.window.SetChild(ptr)
		webkit.WidgetQueueAllocate(ptr)
		webkit.WidgetShow(ptr)

		// Verify attachment succeeded
		if finalParent := webkit.WidgetGetParent(ptr); finalParent == 0 {
			log.Printf("[workspace] WARNING: widget %#x has no parent after SetChild", ptr)
		} else {
			log.Printf("[workspace] attachRoot successful: widget %#x now child of %#x", ptr, finalParent)
		}
		return nil
	})
}

// cleanupPane safely cleans up a pane node with generation tracking
func (wm *WorkspaceManager) cleanupPane(node *paneNode, generation uint) {
	if node == nil {
		return
	}
	if !node.widgetValid {
		return
	}

	node.widgetValid = false
	node.cleanupGeneration = generation

	if node.pane != nil {
		node.pane.CleanupFromWorkspace(wm)
		if node.pane.webView != nil {
			node.pane.webView.Destroy()
		}
	}

	if node.container != nil {
		node.container.Invalidate()
		node.container = nil
	}

	node.parent = nil
	node.left = nil
	node.right = nil
}

// decommissionParent cleans up a parent node after promotion
func (wm *WorkspaceManager) decommissionParent(parent *paneNode, generation uint) {
	if parent == nil {
		return
	}

	// For branch nodes (paneds), ensure the GTK widget is destroyed properly
	if !parent.isLeaf && parent.container != nil {
		parent.container.Execute(func(ptr uintptr) error {
			// Clear any remaining children first to ensure clean destruction
			// Note: Children may already be reparented during promotion, so verify parent relationship
			if webkit.IsPaned(ptr) {
				startChild := webkit.PanedGetStartChild(ptr)
				endChild := webkit.PanedGetEndChild(ptr)

				// Only clear children that are still valid AND still parented to this paned
				// If the child was reparented during swapContainers, its parent will be different
				if startChild != 0 && webkit.WidgetIsValid(startChild) && webkit.WidgetGetParent(startChild) == ptr {
					webkit.PanedSetStartChild(ptr, 0)
				}
				if endChild != 0 && webkit.WidgetIsValid(endChild) && webkit.WidgetGetParent(endChild) == ptr {
					webkit.PanedSetEndChild(ptr, 0)
				}
				log.Printf("[workspace] cleared paned children before destruction: %#x", ptr)
			}
			// Hook widget destruction
			webkit.WidgetHookDestroy(ptr)
			log.Printf("[workspace] destroyed paned widget: %#x", ptr)
			return nil
		})
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
	if err := node.pane.webView.Destroy(); err != nil {
		log.Printf("[workspace] failed to destroy webview: %v", err)
	}
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

	// STEP 3: Handle trivial exit cases
	if ctx.remaining == 1 {
		return wm.cleanupAndExit(node)
	}
	if node == wm.root {
		return wm.promoteNewRoot(ctx, node)
	}

	// STEP 4: Promote sibling in-place
	parent := node.parent
	sibling := wm.getSibling(node)
	grandparent := parent.parent
	wm.ensureWidgets(grandparent, parent, sibling)

	wm.promoteSibling(grandparent, parent, sibling)

	// STEP 5: GTK updates leverage auto-unparenting
	wm.swapContainers(grandparent, sibling)

	// STEP 6: Cleanup & focus
	wm.cleanupPane(node, ctx.Generation())
	wm.decommissionParent(parent, ctx.Generation())
	wm.setFocusToLeaf(sibling)

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
			stack.parent.container.Execute(func(pPtr uintptr) error {
				remaining.container.Execute(func(rPtr uintptr) error {
					if stack.parent.left == remaining {
						webkit.PanedSetStartChild(pPtr, rPtr)
					} else {
						webkit.PanedSetEndChild(pPtr, rPtr)
					}
					return nil
				})
				return nil
			})
		} else {
			// Stack was root
			wm.root = remaining
			remaining.container.Execute(func(ptr uintptr) error {
				wm.window.SetChild(ptr)
				return nil
			})
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
	stack.container.Execute(func(boxPtr uintptr) error {
		node.container.Execute(func(nodePtr uintptr) error {
			webkit.BoxRemove(boxPtr, nodePtr)
			return nil
		})
		return nil
	})

	// Show new active pane
	if stack.activeStackIndex >= 0 {
		active := stack.stackedPanes[stack.activeStackIndex]
		active.container.Execute(func(ptr uintptr) error {
			webkit.WidgetShow(ptr)
			return nil
		})
	}

	// Cleanup closed pane
	wm.cleanupPane(node, wm.nextCleanupGeneration())

	return stack, nil
}
