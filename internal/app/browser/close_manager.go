package browser

import (
	"errors"
	"log"

	"github.com/bnema/dumber/pkg/webkit"
)

// CloseManager handles pane closing operations
type CloseManager struct {
	wm *WorkspaceManager
}

// NewCloseManager creates a new close manager
func NewCloseManager(wm *WorkspaceManager) *CloseManager {
	return &CloseManager{wm: wm}
}

// CloseCurrentPane closes the currently focused pane
func (cm *CloseManager) CloseCurrentPane() error {
	currentNode := cm.wm.currentlyFocused
	if currentNode == nil {
		return errors.New("no currently focused pane")
	}

	// Check if this is a stacked pane
	if currentNode.parent != nil && currentNode.parent.isStacked {
		return cm.wm.stackedPaneManager.CloseStackedPane(currentNode)
	}

	// Regular pane close
	return cm.ClosePane(currentNode)
}

// ClosePane closes a specific pane node
func (cm *CloseManager) ClosePane(node *paneNode) error {
	if node == nil {
		return errors.New("cannot close nil pane")
	}

	// Update metadata state
	if node.metadata != nil {
		node.metadata.SetState(PaneStateClosing)
		node.metadata.SetCloseReason("user_requested")
	}

	log.Printf("[close] Closing pane: %p", node)

	// Clean up hover tracking
	cm.wm.detachHover(node)

	// Clean up the pane itself
	if node.pane != nil {
		node.pane.CleanupFromWorkspace(cm.wm)
		node.pane.Cleanup()
	}

	// Handle tree restructuring
	if err := cm.restructureAfterClose(node); err != nil {
		return err
	}

	// Update metadata state
	if node.metadata != nil {
		node.metadata.SetState(PaneStateClosed)
	}

	log.Printf("[close] Pane closed successfully: %p", node)
	return nil
}

// restructureAfterClose handles tree restructuring after a pane is closed
func (cm *CloseManager) restructureAfterClose(node *paneNode) error {
	parent := node.parent

	if parent == nil {
		// This is the root node
		if cm.wm.root == node {
			cm.wm.root = nil
			cm.wm.currentlyFocused = nil
			log.Printf("[close] Closed root pane")
		}
		return nil
	}

	// Find sibling to promote
	var sibling *paneNode
	if parent.left == node {
		sibling = parent.right
	} else if parent.right == node {
		sibling = parent.left
	}

	if sibling == nil {
		return errors.New("no sibling found for closed pane")
	}

	// Use safe widget operation for reparenting
	return cm.wm.widgetManager.SafeWidgetOperation(func() error {
		return cm.promoteSibling(parent, sibling)
	})
}

// promoteSibling promotes a sibling node to replace its parent
func (cm *CloseManager) promoteSibling(parent, sibling *paneNode) error {
	grandparent := parent.parent
	siblingContainer := sibling.container

	if siblingContainer == nil {
		return errors.New("sibling container is nil")
	}

	// Validate widgets before reparenting
	if err := cm.wm.widgetManager.ValidateWidgetsForReparenting(siblingContainer); err != nil {
		return err
	}

	var siblingContainerPtr uintptr
	if err := siblingContainer.Execute(func(containerPtr uintptr) error {
		siblingContainerPtr = containerPtr
		return nil
	}); err != nil {
		return err
	}

	// Update tree structure first
	sibling.parent = grandparent

	if grandparent == nil {
		// Sibling becomes new root
		cm.wm.root = sibling
		if cm.wm.window != nil {
			cm.wm.window.SetChild(siblingContainerPtr)
		}
	} else {
		// Attach sibling to grandparent
		if grandparent.left == parent {
			grandparent.left = sibling
			if grandparent.container != nil {
				if err := grandparent.container.Execute(func(panedPtr uintptr) error {
					webkit.PanedSetStartChild(panedPtr, siblingContainerPtr)
					return nil
				}); err != nil {
					return err
				}
			}
		} else if grandparent.right == parent {
			grandparent.right = sibling
			if grandparent.container != nil {
				if err := grandparent.container.Execute(func(panedPtr uintptr) error {
					webkit.PanedSetEndChild(panedPtr, siblingContainerPtr)
					return nil
				}); err != nil {
					return err
				}
			}
		}
	}

	// Update main pane if necessary
	cm.wm.layoutManager.UpdateMainPane()

	// Focus the promoted sibling if it's a leaf
	if sibling.isLeaf {
		cm.wm.focusManager.SetActivePane(sibling)
		cm.wm.currentlyFocused = sibling
	} else {
		// Find a leaf descendant to focus
		leaves := cm.wm.layoutManager.CollectLeaves()
		if len(leaves) > 0 {
			cm.wm.focusManager.SetActivePane(leaves[0])
			cm.wm.currentlyFocused = leaves[0]
		}
	}

	// Update CSS classes
	cm.wm.cssManager.EnsurePaneBaseClasses()

	return nil
}

// RemoveFromMaps removes a WebView from tracking maps
func (cm *CloseManager) RemoveFromMaps(view *webkit.WebView) {
	if view == nil {
		return
	}
	delete(cm.wm.viewToNode, view)
	delete(cm.wm.lastSplitMsg, view)
	delete(cm.wm.lastExitMsg, view)
}

// RemoveFromAppPanes removes a BrowserPane from the app's panes slice
func (cm *CloseManager) RemoveFromAppPanes(pane *BrowserPane) {
	if pane == nil || cm.wm.app == nil {
		return
	}

	for i, p := range cm.wm.app.panes {
		if p == pane {
			cm.wm.app.panes = append(cm.wm.app.panes[:i], cm.wm.app.panes[i+1:]...)
			break
		}
	}
}