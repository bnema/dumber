// workspace_pane_ops.go - Pane creation, splitting, closing and tree operations
package browser

import (
	"errors"
	"log"

	"github.com/bnema/dumber/pkg/webkit"
)

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

// closeCurrentPane closes the currently focused pane
func (wm *WorkspaceManager) closeCurrentPane() {
	if wm == nil || wm.GetActiveNode() == nil {
		return
	}
	if err := wm.closePane(wm.GetActiveNode()); err != nil {
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

	webkit.IdleAdd(func() bool {
		if newContainer != 0 {
			webkit.WidgetShow(newContainer)
		}
		// Attach GTK focus controller to new pane
		if wm.focusStateMachine != nil {
			wm.focusStateMachine.attachGTKController(newLeaf)
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
		wm.ensureHover(target)
	}

	wm.app.panes = append(wm.app.panes, newPane)
	if newPane.zoomController != nil {
		newPane.zoomController.ApplyInitialZoom()
	}

	// Update CSS classes for all panes now that we have multiple panes

	webkit.IdleAdd(func() bool {
		if newContainer != 0 {
			webkit.WidgetShow(newContainer)
		}
		// Attach GTK focus controller to new pane
		if wm.focusStateMachine != nil {
			wm.focusStateMachine.attachGTKController(newLeaf)
		}
		wm.SetActivePane(newLeaf, SourceSplit)
		return false
	})

	return newLeaf, nil
}

// closePane closes a specific pane and handles tree restructuring
func (wm *WorkspaceManager) closePane(node *paneNode) error {
	if wm == nil || node == nil || !node.isLeaf {
		return errors.New("close target must be a leaf pane")
	}
	if node.pane == nil || node.pane.webView == nil {
		return errors.New("close target missing webview")
	}

	// Handle closing panes in a stack
	if node.parent != nil && node.parent.isStacked {
		return wm.stackedPaneManager.CloseStackedPane(node)
	}

	if node.parent != nil && node.parent.container != nil {
		node.parent.container.Execute(func(panedPtr uintptr) error {
			if node.parent.left == node {
				webkit.PanedSetStartChild(panedPtr, 0)
				// GTK4 automatically unparents when setting child to 0
			} else if node.parent.right == node {
				webkit.PanedSetEndChild(panedPtr, 0)
				// GTK4 automatically unparents when setting child to 0
			}
			return nil
		})
	} else if node.container != nil {
		// Only manually unparent if widget wasn't auto-unparented above
		node.container.Execute(func(containerPtr uintptr) error {
			if webkit.WidgetGetParent(containerPtr) != 0 {
				webkit.WidgetUnparent(containerPtr)
			}
			return nil
		})
	}

	remaining := len(wm.app.panes)
	willBeLastPane := remaining <= 1
	paneCleaned := false
	ensureCleanup := func() {
		if paneCleaned {
			return
		}
		node.pane.Cleanup()
		paneCleaned = true
	}

	if remaining == 2 {
		var sibling *paneNode
		if node.parent != nil {
			if node.parent.left == node {
				sibling = node.parent.right
			} else if node.parent.right == node {
				sibling = node.parent.left
			}
		}
		if sibling != nil && sibling.isLeaf && sibling.pane != nil && sibling.pane.webView != nil {
			if sibling.isPopup && !node.isPopup {
				parentNode := node.parent
				var parentContainer *SafeWidget
				if parentNode != nil {
					parentContainer = parentNode.container
				}

				// Detach remaining GTK widgets before destroying the WebViews so GTK
				// does not attempt to dispose still-parented children.
				if parentContainer != nil {
					parentContainer.Execute(func(panedPtr uintptr) error {
						if parentNode.left == sibling {
							webkit.PanedSetStartChild(panedPtr, 0)
						} else if parentNode.right == sibling {
							webkit.PanedSetEndChild(panedPtr, 0)
						}
						return nil
					})
				}
				if sibling.container != nil {
					sibling.container.Execute(func(containerPtr uintptr) error {
						webkit.WidgetUnparent(containerPtr)
						return nil
					})
				}
				if parentContainer != nil && wm.window != nil {
					wm.window.SetChild(0)
				}

				log.Printf("[workspace] closing primary pane with related popup present; exiting")
				ensureCleanup()
				sibling.pane.Cleanup()

				wm.detachHover(node)
				wm.detachHover(sibling)

				// Clean up workspace tracking for both panes
				node.pane.CleanupFromWorkspace(wm)
				sibling.pane.CleanupFromWorkspace(wm)

				if node.pane.webView != nil {
					if err := node.pane.webView.Destroy(); err != nil {
						log.Printf("[workspace] failed to destroy primary webview: %v", err)
					}
				}
				if sibling.pane.webView != nil {
					if err := sibling.pane.webView.Destroy(); err != nil {
						log.Printf("[workspace] failed to destroy popup webview: %v", err)
					}
				}

				wm.root = nil
				wm.mainPane = nil

				webkit.QuitMainLoop()
				return nil
			}
		}
	}
	if willBeLastPane && wm.root == node {
		log.Printf("[workspace] closing final pane; exiting browser")
		ensureCleanup()
		wm.detachHover(node)
		if err := node.pane.webView.Destroy(); err != nil {
			log.Printf("[workspace] failed to destroy webview: %v", err)
		}
		webkit.QuitMainLoop()
		return nil
	}

	parent := node.parent
	if parent == nil {
		// This is the root pane (no parent in tree structure)
		if node != wm.root {
			return errors.New("inconsistent state: node has no parent but is not root")
		}

		// Check if we can find a replacement root
		replacement := wm.findReplacementRoot(node)
		if replacement == nil {
			// No other panes exist, this is the final pane
			log.Printf("[workspace] closing final pane; exiting browser")
			ensureCleanup()
			wm.detachHover(node)
			if err := node.pane.webView.Destroy(); err != nil {
				log.Printf("[workspace] failed to destroy webview: %v", err)
			}
			webkit.QuitMainLoop()
			return nil
		}

		// We have a replacement - delegate root status and close this pane
		log.Printf("[workspace] delegating root status from node=%#x to replacement=%#x", node.container, replacement.container)

		// Clean up workspace tracking
		node.pane.CleanupFromWorkspace(wm)

		if wm.mainPane == node {
			wm.mainPane = nil
		}

		// Clear current active if it's the node being closed
		if wm.GetActiveNode() == node {
		}

		// Set replacement as new root
		wm.root = replacement
		replacement.parent = nil

		// Replace window child directly (GTK handles reparenting automatically)
		if wm.window != nil && replacement.container != nil {
			replacement.container.Execute(func(containerPtr uintptr) error {
				wm.window.SetChild(containerPtr)
				webkit.WidgetQueueAllocate(containerPtr)
				webkit.WidgetShow(containerPtr)
				return nil
			})
		}

		// Focus a suitable pane
		focusTarget := wm.leftmostLeaf(replacement)
		if focusTarget != nil {
			wm.SetActivePane(focusTarget, SourceClose)
		}

		// Destroy the webview and detach hover AFTER rearranging hierarchy
		// Only destroy the webview if this is the final pane, otherwise just clean up
		ensureCleanup()
		wm.detachHover(node)
		switch {
		case node.isPopup:
			if err := node.pane.webView.Destroy(); err != nil {
				log.Printf("[workspace] failed to destroy popup webview: %v", err)
			}
		case willBeLastPane:
			// This is the last pane, safe to destroy completely
			if err := node.pane.webView.Destroy(); err != nil {
				log.Printf("[workspace] failed to destroy webview: %v", err)
			}
		default:
			// Multiple panes remain, don't destroy the window - just clean up the webview
			log.Printf("[workspace] skipping webview destruction to preserve window (panes remaining: %d)", remaining-1)
			// TODO: Add a method to destroy just the webview without the window
		}

		wm.updateMainPane()
		// Update CSS classes after pane count changes
		log.Printf("[workspace] root pane closed and delegated; panes remaining=%d", len(wm.app.panes))
		return nil
	}

	var sibling *paneNode
	if parent.left == node {
		sibling = parent.right
	} else {
		sibling = parent.left
	}
	if sibling == nil {
		return errors.New("pane close failed: missing sibling")
	}

	log.Printf("[workspace] closing pane: target=%#x parent=%#x sibling=%#x remaining=%d", node.container, parent.container, sibling.container, remaining)

	// Clean up workspace tracking
	node.pane.CleanupFromWorkspace(wm)

	if wm.mainPane == node {
		wm.mainPane = nil
	}

	// Clear current active if it's the node being closed
	if wm.GetActiveNode() == node {
	}

	grand := parent.parent
	if grand == nil {
		// Parent is the root node. Promote sibling to become the new root.
		log.Printf("[workspace] promoting sibling to root: container=%#x, isLeaf=%v", sibling.container, sibling.isLeaf)

		// Update tree structure first
		wm.root = sibling
		sibling.parent = nil

		// For root promotion, GTK4 handles unparenting automatically when we set the new window child
		if wm.window != nil && sibling.container != nil {
			sibling.container.Execute(func(containerPtr uintptr) error {
				wm.window.SetChild(containerPtr)
				webkit.WidgetQueueAllocate(containerPtr)
				webkit.WidgetShow(containerPtr)
				log.Printf("[workspace] successfully promoted sibling to root: %#x", containerPtr)
				return nil
			})
		}
	} else {
		// Parent has a grandparent, so promote sibling to take parent's place
		log.Printf("[workspace] promoting sibling to parent's position: sibling=%#x grand=%#x", sibling.container, grand.container)

		// Update tree structure
		sibling.parent = grand
		if grand.left == parent {
			grand.left = sibling
		} else {
			grand.right = sibling
		}

		// Move sibling to grandparent's paned widget
		// GTK4 handles unparenting automatically when we set the new paned child
		grand.container.Execute(func(grandPtr uintptr) error {
			sibling.container.Execute(func(siblingPtr uintptr) error {
				if grand.left == sibling {
					webkit.PanedSetStartChild(grandPtr, siblingPtr)
				} else {
					webkit.PanedSetEndChild(grandPtr, siblingPtr)
				}
				webkit.WidgetQueueAllocate(siblingPtr)
				webkit.WidgetShow(siblingPtr)
				log.Printf("[workspace] successfully promoted sibling to parent position: %#x", siblingPtr)
				return nil
			})
			return nil
		})
	}

	// Find a suitable focus target
	focusTarget := wm.leftmostLeaf(sibling)
	if focusTarget != nil {
		wm.SetActivePane(focusTarget, SourceClose)
	}

	// Clean up the node being closed
	ensureCleanup()
	wm.detachHover(node)

	switch {
	case node.isPopup:
		if err := node.pane.webView.Destroy(); err != nil {
			log.Printf("[workspace] failed to destroy popup webview: %v", err)
		}
	case willBeLastPane:
		// This is the last pane, safe to destroy completely
		if err := node.pane.webView.Destroy(); err != nil {
			log.Printf("[workspace] failed to destroy webview: %v", err)
		}
	default:
		// Multiple panes remain, don't destroy the window - just clean up the webview
		log.Printf("[workspace] skipping webview destruction to preserve window (panes remaining: %d)", remaining-1)
		// TODO: Add a method to destroy just the webview without the window
	}

	wm.updateMainPane()
	// Update CSS classes after pane count changes
	log.Printf("[workspace] pane closed; panes remaining=%d", len(wm.app.panes))
	return nil
}
