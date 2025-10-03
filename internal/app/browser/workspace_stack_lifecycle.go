// workspace_stack_lifecycle.go - Stack pane lifecycle coordination without SafeWidget wrappers
package browser

import (
	"errors"
	"fmt"
	"log"
)

// StackLifecycleManager manages the lifecycle hooks around stacked pane operations.
type StackLifecycleManager struct {
	wm            *WorkspaceManager
	treeValidator *TreeValidator
}

// NewStackLifecycleManager creates a new stack lifecycle manager.
func NewStackLifecycleManager(wm *WorkspaceManager, treeValidator *TreeValidator) *StackLifecycleManager {
	return &StackLifecycleManager{
		wm:            wm,
		treeValidator: treeValidator,
	}
}

// CloseStackedPaneWithLifecycle validates the stack state and delegates the actual
// close operation to the stacked pane manager. This keeps widget handling simple
// (raw GTK pointers) while still giving us a single place for additional safety checks.
func (slm *StackLifecycleManager) CloseStackedPaneWithLifecycle(node *paneNode) error {
	if node == nil {
		return errors.New("node is nil")
	}

	stackNode := node.parent
	if stackNode == nil || !stackNode.isStacked {
		return errors.New("node is not part of a stacked pane")
	}

	log.Printf("[stack-lifecycle] Closing stacked pane: node=%p stack=%p stackSize=%d",
		node, stackNode, len(stackNode.stackedPanes))

	if err := slm.validateStackState(stackNode, "before close"); err != nil {
		return fmt.Errorf("invalid stack state before close: %w", err)
	}

	if slm.wm == nil || slm.wm.stackedPaneManager == nil {
		return errors.New("stacked pane manager not available")
	}

	if err := slm.wm.stackedPaneManager.CloseStackedPane(node); err != nil {
		return err
	}

	if slm.treeValidator != nil {
		if err := slm.treeValidator.ValidateTree(slm.wm.root, "after stack close"); err != nil {
			log.Printf("[stack-lifecycle] Tree validation failed after stack close: %v", err)
		}
	}

	return nil
}

// validateStackState validates the current state of a stack before we perform operations.
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
