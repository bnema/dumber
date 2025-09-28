// workspace_stack_lifecycle.go - Enhanced stack container lifecycle management for bulletproof operations
package browser

import (
	"errors"
	"fmt"
	"log"

	"github.com/bnema/dumber/pkg/webkit"
)

// StackLifecycleManager manages the complete lifecycle of stack containers
type StackLifecycleManager struct {
	wm              *WorkspaceManager
	treeValidator   *TreeValidator
	widgetTxManager *WidgetTransactionManager
}

// NewStackLifecycleManager creates a new stack lifecycle manager
func NewStackLifecycleManager(wm *WorkspaceManager, treeValidator *TreeValidator, widgetTxManager *WidgetTransactionManager) *StackLifecycleManager {
	return &StackLifecycleManager{
		wm:              wm,
		treeValidator:   treeValidator,
		widgetTxManager: widgetTxManager,
	}
}

// CloseStackedPaneWithLifecycle handles closing a stacked pane with proper lifecycle management
func (slm *StackLifecycleManager) CloseStackedPaneWithLifecycle(node *paneNode) error {
	if node.parent == nil || !node.parent.isStacked {
		return errors.New("node is not part of a stacked pane")
	}

	stackNode := node.parent

	log.Printf("[stack-lifecycle] Closing stacked pane: node=%p stack=%p stackSize=%d",
		node, stackNode, len(stackNode.stackedPanes))

	// Validate initial state
	if err := slm.validateStackState(stackNode, "before close"); err != nil {
		return fmt.Errorf("invalid stack state before close: %w", err)
	}

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

	// Determine what to do based on remaining stack size
	remainingSize := len(stackNode.stackedPanes) - 1

	switch remainingSize {
	case 0:
		// Last pane in stack - remove entire stack container
		return slm.removeEmptyStackContainer(stackNode, node)
	case 1:
		// Two panes, closing one - convert remaining pane back to regular pane
		return slm.unstackLastPane(stackNode, node, nodeIndex)
	default:
		// Multiple panes remain - remove pane from stack
		return slm.removePaneFromStack(stackNode, node, nodeIndex)
	}
}

// removeEmptyStackContainer removes a stack container when the last pane is closed
func (slm *StackLifecycleManager) removeEmptyStackContainer(stackNode *paneNode, closingNode *paneNode) error {
	log.Printf("[stack-lifecycle] Removing empty stack container: stack=%p", stackNode)

	// Create transaction for this operation
	txID := fmt.Sprintf("remove_empty_stack_%p", stackNode)
	tx := slm.widgetTxManager.BeginTransaction(txID)

	// Clean up the closing pane first
	slm.cleanupPaneResources(closingNode)

	// Remove stack from tree structure
	if err := slm.removeStackFromTree(stackNode, tx); err != nil {
		tx.Rollback()
		slm.widgetTxManager.FinishTransaction(txID, false, err.Error())
		return fmt.Errorf("failed to remove stack from tree: %w", err)
	}

	// Execute widget operations
	if err := tx.Execute(); err != nil {
		tx.Rollback()
		slm.widgetTxManager.FinishTransaction(txID, false, err.Error())
		return fmt.Errorf("failed to execute widget operations: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		slm.widgetTxManager.FinishTransaction(txID, false, err.Error())
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	slm.widgetTxManager.FinishTransaction(txID, true, "")

	// Validate final state
	if slm.treeValidator != nil {
		if err := slm.treeValidator.ValidateTree(slm.wm.root, "remove_empty_stack"); err != nil {
			log.Printf("[stack-lifecycle] Tree validation failed after removing empty stack: %v", err)
		}
	}

	log.Printf("[stack-lifecycle] Successfully removed empty stack container")
	return nil
}

// unstackLastPane converts a two-pane stack back to a regular pane when one is closed
func (slm *StackLifecycleManager) unstackLastPane(stackNode *paneNode, closingNode *paneNode, closingIndex int) error {
	log.Printf("[stack-lifecycle] Unstacking last pane: stack=%p closingIndex=%d", stackNode, closingIndex)

	// Find the remaining pane
	var remainingPane *paneNode
	for i, pane := range stackNode.stackedPanes {
		if i != closingIndex && pane != closingNode {
			remainingPane = pane
			break
		}
	}

	if remainingPane == nil {
		return errors.New("could not find remaining pane in stack")
	}

	// Create transaction for this operation
	txID := fmt.Sprintf("unstack_last_%p", stackNode)
	tx := slm.widgetTxManager.BeginTransaction(txID)

	// Clean up the closing pane
	slm.cleanupPaneResources(closingNode)

	// Convert remaining pane back to regular pane
	if err := slm.convertStackToRegularPane(stackNode, remainingPane, tx); err != nil {
		tx.Rollback()
		slm.widgetTxManager.FinishTransaction(txID, false, err.Error())
		return fmt.Errorf("failed to convert stack to regular pane: %w", err)
	}

	// Execute widget operations
	if err := tx.Execute(); err != nil {
		tx.Rollback()
		slm.widgetTxManager.FinishTransaction(txID, false, err.Error())
		return fmt.Errorf("failed to execute widget operations: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		slm.widgetTxManager.FinishTransaction(txID, false, err.Error())
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	slm.widgetTxManager.FinishTransaction(txID, true, "")

	// Validate final state
	if slm.treeValidator != nil {
		if err := slm.treeValidator.ValidateTree(slm.wm.root, "unstack_last"); err != nil {
			log.Printf("[stack-lifecycle] Tree validation failed after unstacking: %v", err)
		}
	}

	log.Printf("[stack-lifecycle] Successfully unstacked last pane")
	return nil
}

// removePaneFromStack removes a pane from a multi-pane stack
func (slm *StackLifecycleManager) removePaneFromStack(stackNode *paneNode, closingNode *paneNode, closingIndex int) error {
	log.Printf("[stack-lifecycle] Removing pane from stack: pane=%p index=%d", closingNode, closingIndex)

	// Create transaction for this operation
	txID := fmt.Sprintf("remove_from_stack_%p_%d", stackNode, closingIndex)
	tx := slm.widgetTxManager.BeginTransaction(txID)

	// Clean up the closing pane
	slm.cleanupPaneResources(closingNode)

	// Remove pane widgets from stack container
	if err := slm.removePaneWidgetsFromStack(stackNode, closingNode, tx); err != nil {
		tx.Rollback()
		slm.widgetTxManager.FinishTransaction(txID, false, err.Error())
		return fmt.Errorf("failed to remove pane widgets: %w", err)
	}

	// Update stack data structures
	slm.updateStackAfterRemoval(stackNode, closingIndex)

	// Execute widget operations
	if err := tx.Execute(); err != nil {
		tx.Rollback()
		slm.widgetTxManager.FinishTransaction(txID, false, err.Error())
		return fmt.Errorf("failed to execute widget operations: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		slm.widgetTxManager.FinishTransaction(txID, false, err.Error())
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	slm.widgetTxManager.FinishTransaction(txID, true, "")

	// Update stack visibility after removal
	if slm.wm.stackedPaneManager != nil {
		slm.wm.stackedPaneManager.UpdateStackVisibility(stackNode)
	}

	// Validate final state
	if slm.treeValidator != nil {
		if err := slm.treeValidator.ValidateTree(slm.wm.root, "remove_from_stack"); err != nil {
			log.Printf("[stack-lifecycle] Tree validation failed after removing from stack: %v", err)
		}
	}

	log.Printf("[stack-lifecycle] Successfully removed pane from stack")
	return nil
}

// removeStackFromTree removes the stack container from the tree structure
func (slm *StackLifecycleManager) removeStackFromTree(stackNode *paneNode, tx *WidgetTransaction) error {
	if stackNode == nil {
		return errors.New("stack node is nil")
	}

	parent := stackNode.parent

	// Add operation to unparent stack container when still valid
	if stackNode.container != nil {
		if !stackNode.container.IsValid() {
			log.Printf("[stack-lifecycle] stack container already invalid, skipping unparent: node=%p", stackNode)
			slm.invalidateSafeWidget(&stackNode.container)
			slm.invalidateSafeWidget(&stackNode.stackWrapper)
		} else {
			op := CreateWidgetUnparentOperation(
				fmt.Sprintf("unparent_stack_%p", stackNode),
				stackNode.container,
			)
			if err := tx.AddOperation(op); err != nil {
				return fmt.Errorf("failed to add unparent operation: %w", err)
			}

			// Schedule cleanup of stack container and wrapper after unparent
			cleanupOp := &WidgetOperation{
				ID:          fmt.Sprintf("cleanup_stack_widgets_%p", stackNode),
				Description: "Invalidate stack container and wrapper",
				Priority:    10,
				Execute: func() error {
					slm.invalidateSafeWidget(&stackNode.container)
					slm.invalidateSafeWidget(&stackNode.stackWrapper)
					return nil
				},
			}
			if err := tx.AddOperation(cleanupOp); err != nil {
				return fmt.Errorf("failed to add stack cleanup operation: %w", err)
			}
		}
	}

	// Update tree structure
	if parent == nil {
		// Stack was root - clear root
		slm.wm.root = nil
		slm.wm.mainPane = nil
	} else {
		// Remove stack from parent
		if parent.left == stackNode {
			parent.left = nil
		} else if parent.right == stackNode {
			parent.right = nil
		}

		// If parent now has only one child, it needs restructuring
		if (parent.left == nil) != (parent.right == nil) {
			// Parent has exactly one child - promote the child
			var remainingChild *paneNode
			if parent.left != nil {
				remainingChild = parent.left
			} else {
				remainingChild = parent.right
			}

			if err := slm.promoteChildNode(parent, remainingChild, tx); err != nil {
				return fmt.Errorf("failed to promote child node: %w", err)
			}
		}
	}

	return nil
}

// convertStackToRegularPane converts a stack container to a regular pane
func (slm *StackLifecycleManager) convertStackToRegularPane(stackNode *paneNode, remainingPane *paneNode, tx *WidgetTransaction) error {
	parent := stackNode.parent

	// Remove remaining pane from stack container
	if remainingPane.container != nil {
		op := CreateWidgetUnparentOperation(
			fmt.Sprintf("unparent_remaining_%p", remainingPane),
			remainingPane.container,
		)
		if err := tx.AddOperation(op); err != nil {
			return fmt.Errorf("failed to add unparent operation: %w", err)
		}
	}

	// Update remaining pane properties
	remainingPane.parent = parent
	remainingPane.isStacked = false

	// Clear stack-related fields
	remainingPane.titleBar = nil

	// Update tree structure
	if parent == nil {
		// Stack was root - make remaining pane the new root
		slm.wm.root = remainingPane
		slm.wm.mainPane = remainingPane

		// Reparent to window
		if remainingPane.container != nil && slm.wm.window != nil {
			op := &WidgetOperation{
				ID:          fmt.Sprintf("reparent_to_window_%p", remainingPane),
				Description: "Reparent remaining pane to window",
				Priority:    200,
				Execute: func() error {
					return remainingPane.container.Execute(func(ptr uintptr) error {
						slm.wm.window.SetChild(ptr)
						webkit.WidgetQueueAllocate(ptr)
						webkit.WidgetShow(ptr)
						return nil
					})
				},
			}
			if err := tx.AddOperation(op); err != nil {
				return fmt.Errorf("failed to add reparent operation: %w", err)
			}
		}
	} else {
		// Replace stack with remaining pane in parent
		if parent.left == stackNode {
			parent.left = remainingPane
		} else if parent.right == stackNode {
			parent.right = remainingPane
		}

		// Reparent to parent container
		if remainingPane.container != nil && parent.container != nil {
			isStart := parent.left == remainingPane
			op := CreateWidgetReparentOperation(
				fmt.Sprintf("reparent_remaining_%p", remainingPane),
				remainingPane.container,
				parent.container.Ptr(),
				isStart,
			)
			if err := tx.AddOperation(op); err != nil {
				return fmt.Errorf("failed to add reparent operation: %w", err)
			}
		}
	}

	return nil
}

// removePaneWidgetsFromStack removes a pane's widgets from the stack container
func (slm *StackLifecycleManager) removePaneWidgetsFromStack(stackNode *paneNode, closingNode *paneNode, tx *WidgetTransaction) error {
	// Remove title bar if it exists
	if closingNode.titleBar != nil {
		if !closingNode.titleBar.IsValid() {
			log.Printf("[stack-lifecycle] title bar already invalid, skipping removal: pane=%p", closingNode)
			slm.invalidateSafeWidget(&closingNode.titleBar)
		} else {
			op := CreateWidgetUnparentOperation(
				fmt.Sprintf("remove_titlebar_%p", closingNode),
				closingNode.titleBar,
			)
			if err := tx.AddOperation(op); err != nil {
				return fmt.Errorf("failed to add titlebar removal operation: %w", err)
			}
		}
	}

	// Remove container
	if closingNode.container != nil {
		if !closingNode.container.IsValid() {
			log.Printf("[stack-lifecycle] pane container already invalid, skipping removal: pane=%p", closingNode)
			slm.invalidateSafeWidget(&closingNode.container)
		} else {
			op := CreateWidgetUnparentOperation(
				fmt.Sprintf("remove_container_%p", closingNode),
				closingNode.container,
			)
			if err := tx.AddOperation(op); err != nil {
				return fmt.Errorf("failed to add container removal operation: %w", err)
			}
		}
	}

	cleanupOp := &WidgetOperation{
		ID:          fmt.Sprintf("cleanup_removed_pane_%p", closingNode),
		Description: "Invalidate removed pane widgets",
		Priority:    10,
		Execute: func() error {
			slm.invalidateSafeWidget(&closingNode.titleBar)
			slm.invalidateSafeWidget(&closingNode.container)
			return nil
		},
	}
	if err := tx.AddOperation(cleanupOp); err != nil {
		return fmt.Errorf("failed to add pane widget cleanup operation: %w", err)
	}

	return nil
}

// updateStackAfterRemoval updates stack data structures after removing a pane
func (slm *StackLifecycleManager) updateStackAfterRemoval(stackNode *paneNode, removedIndex int) {
	// Remove from stackedPanes slice
	stackNode.stackedPanes = append(
		stackNode.stackedPanes[:removedIndex],
		stackNode.stackedPanes[removedIndex+1:]...,
	)

	// Update active index if necessary
	if stackNode.activeStackIndex >= removedIndex {
		if stackNode.activeStackIndex > 0 {
			stackNode.activeStackIndex--
		}
	}

	// Ensure active index is valid
	if stackNode.activeStackIndex >= len(stackNode.stackedPanes) {
		stackNode.activeStackIndex = len(stackNode.stackedPanes) - 1
	}
	if stackNode.activeStackIndex < 0 && len(stackNode.stackedPanes) > 0 {
		stackNode.activeStackIndex = 0
	}

	log.Printf("[stack-lifecycle] Updated stack: remaining=%d activeIndex=%d",
		len(stackNode.stackedPanes), stackNode.activeStackIndex)
}

// promoteChildNode promotes a child node to replace its parent
func (slm *StackLifecycleManager) promoteChildNode(parent *paneNode, child *paneNode, tx *WidgetTransaction) error {
	grandparent := parent.parent

	// Update child's parent pointer
	child.parent = grandparent

	// Update grandparent or root reference
	if grandparent == nil {
		// Child becomes new root
		slm.wm.root = child
		if child.isLeaf {
			slm.wm.mainPane = child
		}

		// Reparent to window
		if child.container != nil && slm.wm.window != nil {
			op := &WidgetOperation{
				ID:          fmt.Sprintf("promote_to_root_%p", child),
				Description: "Promote child to root",
				Priority:    200,
				Execute: func() error {
					return child.container.Execute(func(ptr uintptr) error {
						slm.wm.window.SetChild(ptr)
						webkit.WidgetQueueAllocate(ptr)
						webkit.WidgetShow(ptr)
						return nil
					})
				},
			}
			if err := tx.AddOperation(op); err != nil {
				return fmt.Errorf("failed to add promote operation: %w", err)
			}
		}
	} else {
		// Update grandparent's child pointer
		if grandparent.left == parent {
			grandparent.left = child
		} else if grandparent.right == parent {
			grandparent.right = child
		}

		// Reparent widget to grandparent
		if child.container != nil && grandparent.container != nil {
			isStart := grandparent.left == child
			op := CreateWidgetReparentOperation(
				fmt.Sprintf("promote_child_%p", child),
				child.container,
				grandparent.container.Ptr(),
				isStart,
			)
			if err := tx.AddOperation(op); err != nil {
				return fmt.Errorf("failed to add promote reparent operation: %w", err)
			}
		}
	}

	return nil
}

// cleanupPaneResources cleans up resources associated with a pane
func (slm *StackLifecycleManager) cleanupPaneResources(node *paneNode) {
	// Detach hover
	if slm.wm != nil {
		slm.wm.detachHover(node)
	}

	// Clean up from workspace
	if node.pane != nil {
		node.pane.CleanupFromWorkspace(slm.wm)
	}

	// Clear references
	node.parent = nil
	node.titleBar = nil
	node.stackWrapper = nil
}

// validateStackState validates the current state of a stack
func (slm *StackLifecycleManager) validateStackState(stackNode *paneNode, operation string) error {
	if stackNode == nil {
		return errors.New("stack node is nil")
	}

	if !stackNode.isStacked {
		return errors.New("node is not marked as stacked")
	}

	if len(stackNode.stackedPanes) == 0 {
		return errors.New("stack has no panes")
	}

	if stackNode.activeStackIndex < 0 || stackNode.activeStackIndex >= len(stackNode.stackedPanes) {
		return fmt.Errorf("invalid active stack index: %d (size: %d)",
			stackNode.activeStackIndex, len(stackNode.stackedPanes))
	}

	// Check that all stacked panes have this node as parent
	for i, pane := range stackNode.stackedPanes {
		if pane == nil {
			return fmt.Errorf("stacked pane at index %d is nil", i)
		}
		if pane.parent != stackNode {
			return fmt.Errorf("stacked pane at index %d has wrong parent", i)
		}
	}

	log.Printf("[stack-lifecycle] Stack state valid for operation '%s': size=%d activeIndex=%d",
		operation, len(stackNode.stackedPanes), stackNode.activeStackIndex)
	return nil
}

// invalidateSafeWidget invalidates and unregisters a widget safely
func (slm *StackLifecycleManager) invalidateSafeWidget(widget **SafeWidget) {
	if widget == nil || *widget == nil {
		return
	}

	ptr := (*widget).Ptr()
	(*widget).Invalidate()

	if slm.wm != nil && slm.wm.widgetRegistry != nil && ptr != 0 {
		slm.wm.widgetRegistry.Unregister(ptr)
	}

	*widget = nil
}
