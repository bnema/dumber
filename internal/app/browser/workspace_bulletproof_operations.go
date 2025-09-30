// workspace_bulletproof_operations.go - Bulletproof wrapper methods for all tree operations
package browser

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/bnema/dumber/pkg/webkit"
)

// BulletproofSplitNode performs a bulletproof split operation with all safety checks
func (wm *WorkspaceManager) BulletproofSplitNode(target *paneNode, direction string) (*paneNode, error) {
	if wm == nil {
		return nil, fmt.Errorf("workspace manager is nil")
	}

	log.Printf("[bulletproof] Starting bulletproof split operation: target=%p direction=%s", target, direction)

	// Step 1: Capture state tombstone for rollback
	tombstone, err := wm.stateTombstoneManager.CaptureState("split")
	if err != nil {
		log.Printf("[bulletproof] Failed to capture state tombstone: %v", err)
		// Continue anyway - tombstone is for rollback safety
	}

	// Step 2: Validate geometry constraints
	validation := wm.geometryValidator.ValidateSplit(target, direction)
	if !validation.IsValid {
		return nil, fmt.Errorf("split validation failed: %s", validation.Reason)
	}

	// Log if re-validation will be needed due to pending widget allocation
	if validation.RequiresRevalidation {
		log.Printf("[bulletproof] Split validation passed with pending allocation - operation will proceed")
	}

	// Step 3: Validate tree invariants before operation
	if err := wm.treeValidator.ValidateTree(wm.root, "before_split"); err != nil {
		return nil, fmt.Errorf("tree validation failed before split: %w", err)
	}

	// Step 4: Execute directly if we're already on the GTK main thread
	if webkit.IsMainThread() {
		log.Printf("[bulletproof] Already on main thread, executing split directly")
		newNode, err := wm.splitNode(target, direction)
		if err != nil {
			if tombstone != nil {
				if rollbackErr := wm.stateTombstoneManager.RestoreState(tombstone.ID); rollbackErr != nil {
					log.Printf("[bulletproof] Rollback failed after split failure: %v", rollbackErr)
				}
			}
			return nil, err
		}

		if err := wm.treeValidator.ValidateTree(wm.root, "after_split"); err != nil {
			log.Printf("[bulletproof] Tree validation failed after split: %v", err)
		}

		// Tree rebalancing is only needed after close promotions. Splits have correct allocation from GTK.

		log.Printf("[bulletproof] Split operation completed successfully (direct execution): newNode=%p", newNode)
		return newNode, nil
	}

	// Step 5: Not on main thread, marshal through concurrency controller
	opReq := &OperationRequest{
		ID:         fmt.Sprintf("split_%p_%d", target, time.Now().UnixNano()),
		Type:       OpTypeSplit,
		TargetNode: target,
		Direction:  direction,
		Parameters: map[string]interface{}{
			"direction": direction,
		},
		Context:    context.Background(),
		MaxRetries: 3,
	}

	resultChan := wm.concurrencyController.SubmitOperation(opReq)

	log.Printf("[bulletproof] Waiting for operation result while pumping GTK events")
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	timeout := time.NewTimer(30 * time.Second)
	defer timeout.Stop()

	for {
		select {
		case result := <-resultChan:
			if !result.Success {
				if tombstone != nil {
					if rollbackErr := wm.stateTombstoneManager.RestoreState(tombstone.ID); rollbackErr != nil {
						log.Printf("[bulletproof] Rollback failed after split failure: %v", rollbackErr)
					}
				}
				return nil, result.Error
			}

			if err := wm.treeValidator.ValidateTree(wm.root, "after_split"); err != nil {
				log.Printf("[bulletproof] Tree validation failed after split: %v", err)
			}

			// Tree rebalancing is only needed after close promotions. Splits have correct allocation from GTK.

			log.Printf("[bulletproof] Split operation completed successfully: newNode=%p", result.NewNode)
			return result.NewNode, nil

		case <-ticker.C:
			if webkit.IsMainThread() {
				webkit.IterateMainLoop()
			}

		case <-timeout.C:
			return nil, fmt.Errorf("split operation timed out")
		}
	}
}

// BulletproofClosePane performs a bulletproof close operation with all safety checks
func (wm *WorkspaceManager) BulletproofClosePane(node *paneNode) error {
	if wm == nil {
		return fmt.Errorf("workspace manager is nil")
	}

	log.Printf("[bulletproof] Starting bulletproof close operation: node=%p", node)
	wm.paneCloseLogf("start bulletproof close node=%p", node)
	wm.dumpTreeState("before_close")

	// Step 1: Capture state tombstone for rollback
	tombstone, err := wm.stateTombstoneManager.CaptureState("close")
	if err != nil {
		log.Printf("[bulletproof] Failed to capture state tombstone: %v", err)
	}

	// Step 2: Validate tree invariants before operation
	if err := wm.treeValidator.ValidateTree(wm.root, "before_close"); err != nil {
		return fmt.Errorf("tree validation failed before close: %w", err)
	}

	// Step 3: Check if this is a stacked pane and use enhanced lifecycle management
	if node.parent != nil && node.parent.isStacked {
		log.Printf("[bulletproof] Using enhanced stack lifecycle management for close")
		return wm.stackLifecycleManager.CloseStackedPaneWithLifecycle(node)
	}

	// Step 4: Execute directly when already on the GTK main thread
	if webkit.IsMainThread() {
		log.Printf("[bulletproof] Already on main thread, executing close directly")

		wm.paneCloseLogf("invoking closePane node=%p", node)
		promoted, err := wm.closePane(node)
		if err != nil {
			wm.paneCloseLogf("closePane failed node=%p err=%v", node, err)
			wm.dumpTreeState("after_close_error")
			if tombstone != nil {
				if rollbackErr := wm.stateTombstoneManager.RestoreState(tombstone.ID); rollbackErr != nil {
					log.Printf("[bulletproof] Rollback failed after close failure: %v", rollbackErr)
				}
			}
			return err
		}

		if err := wm.treeValidator.ValidateTree(wm.root, "after_close"); err != nil {
			log.Printf("[bulletproof] Tree validation failed after close: %v", err)
		}
		wm.paneCloseLogf("closePane succeeded node=%p promoted=%p root=%p", node, promoted, wm.root)
		wm.dumpTreeState("after_close_success")

		if wm.treeRebalancer != nil {
			if err := wm.treeRebalancer.RebalanceAfterClose(node, promoted); err != nil {
				log.Printf("[bulletproof] Tree rebalancing failed after close: %v", err)
			}
		}

		log.Printf("[bulletproof] Close operation completed successfully (direct execution)")
		return nil
	}

	// Step 5: Not on main thread, marshal through concurrency controller
	log.Printf("[bulletproof] Not on main thread, using concurrency controller")
	opReq := &OperationRequest{
		ID:         fmt.Sprintf("close_%p_%d", node, time.Now().UnixNano()),
		Type:       OpTypeClose,
		TargetNode: node,
		Parameters: map[string]interface{}{},
		Context:    context.Background(),
		MaxRetries: 3,
	}

	resultChan := wm.concurrencyController.SubmitOperation(opReq)

	log.Printf("[bulletproof] Waiting for operation result while pumping GTK events")
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	timeout := time.NewTimer(30 * time.Second)
	defer timeout.Stop()

	for {
		select {
		case result := <-resultChan:
			if !result.Success {
				wm.paneCloseLogf("async close failed node=%p err=%v", node, result.Error)
				wm.dumpTreeState("after_close_error")
				if tombstone != nil {
					if rollbackErr := wm.stateTombstoneManager.RestoreState(tombstone.ID); rollbackErr != nil {
						log.Printf("[bulletproof] Rollback failed after close failure: %v", rollbackErr)
					}
				}
				return result.Error
			}

			if err := wm.treeValidator.ValidateTree(wm.root, "after_close"); err != nil {
				log.Printf("[bulletproof] Tree validation failed after close: %v", err)
			}
			wm.paneCloseLogf("async close succeeded node=%p promoted=%p root=%p", node, result.NewNode, wm.root)
			wm.dumpTreeState("after_close_success")

			if wm.treeRebalancer != nil {
				if err := wm.treeRebalancer.RebalanceAfterClose(node, result.NewNode); err != nil {
					log.Printf("[bulletproof] Tree rebalancing failed after close: %v", err)
				}
			}

			log.Printf("[bulletproof] Close operation completed successfully")
			return nil

		case <-ticker.C:
			if webkit.IsMainThread() {
				webkit.IterateMainLoop()
			}

		case <-timeout.C:
			return fmt.Errorf("close operation timed out")
		}
	}
}

// BulletproofStackPane performs a bulletproof stack operation
func (wm *WorkspaceManager) BulletproofStackPane(target *paneNode) (*paneNode, error) {
	if wm == nil {
		return nil, fmt.Errorf("workspace manager is nil")
	}

	log.Printf("[bulletproof] Starting bulletproof stack operation: target=%p", target)

	// Step 1: Validate stack operation constraints
	validation := wm.geometryValidator.ValidateStackOperation(target)
	if !validation.IsValid {
		return nil, fmt.Errorf("stack validation failed: %s", validation.Reason)
	}

	// Log if re-validation will be needed due to pending widget allocation
	if validation.RequiresRevalidation {
		log.Printf("[bulletproof] Stack validation passed with pending allocation - operation will proceed")
	}

	// Step 2: Capture state tombstone for rollback
	tombstone, err := wm.stateTombstoneManager.CaptureState("stack")
	if err != nil {
		log.Printf("[bulletproof] Failed to capture state tombstone: %v", err)
	}

	// Step 3: Validate tree invariants before operation
	if err := wm.treeValidator.ValidateTree(wm.root, "before_stack"); err != nil {
		return nil, fmt.Errorf("tree validation failed before stack: %w", err)
	}

	// Step 4: Execute directly if already on the GTK main thread
	if webkit.IsMainThread() {
		log.Printf("[bulletproof] Already on main thread, executing stack directly")
		newNode, err := wm.stackedPaneManager.StackPane(target)
		if err != nil {
			if tombstone != nil {
				if rollbackErr := wm.stateTombstoneManager.RestoreState(tombstone.ID); rollbackErr != nil {
					log.Printf("[bulletproof] Rollback failed after stack failure: %v", rollbackErr)
				}
			}
			return nil, err
		}

		if err := wm.treeValidator.ValidateTree(wm.root, "after_stack"); err != nil {
			log.Printf("[bulletproof] Tree validation failed after stack: %v", err)
		}

		log.Printf("[bulletproof] Stack operation completed successfully (direct execution): newNode=%p", newNode)
		return newNode, nil
	}

	// Step 5: Not on main thread, marshal through concurrency controller
	opReq := &OperationRequest{
		ID:         fmt.Sprintf("stack_%p_%d", target, time.Now().UnixNano()),
		Type:       OpTypeStack,
		TargetNode: target,
		Parameters: map[string]interface{}{},
		Context:    context.Background(),
		MaxRetries: 3,
	}

	resultChan := wm.concurrencyController.SubmitOperation(opReq)

	log.Printf("[bulletproof] Waiting for operation result while pumping GTK events")
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	timeout := time.NewTimer(30 * time.Second)
	defer timeout.Stop()

	for {
		select {
		case result := <-resultChan:
			if !result.Success {
				if tombstone != nil {
					if rollbackErr := wm.stateTombstoneManager.RestoreState(tombstone.ID); rollbackErr != nil {
						log.Printf("[bulletproof] Rollback failed after stack failure: %v", rollbackErr)
					}
				}
				return nil, result.Error
			}

			if err := wm.treeValidator.ValidateTree(wm.root, "after_stack"); err != nil {
				log.Printf("[bulletproof] Tree validation failed after stack: %v", err)
			}

			log.Printf("[bulletproof] Stack operation completed successfully: newNode=%p", result.NewNode)
			return result.NewNode, nil

		case <-ticker.C:
			if webkit.IsMainThread() {
				webkit.IterateMainLoop()
			}

		case <-timeout.C:
			return nil, fmt.Errorf("stack operation timed out")
		}
	}
}

// EnableBulletproofMode enables all bulletproof features
func (wm *WorkspaceManager) EnableBulletproofMode() {
	if wm.treeValidator != nil {
		wm.treeValidator.Enable()
	}
	if wm.treeRebalancer != nil {
		wm.treeRebalancer.Enable()
	}
	log.Printf("[bulletproof] Bulletproof mode enabled")
}

// DisableBulletproofMode disables bulletproof features for performance
func (wm *WorkspaceManager) DisableBulletproofMode() {
	if wm.treeValidator != nil {
		wm.treeValidator.Disable()
	}
	if wm.treeRebalancer != nil {
		wm.treeRebalancer.Disable()
	}
	log.Printf("[bulletproof] Bulletproof mode disabled")
}

// SetBulletproofDebugMode enables/disables debug mode for all bulletproof components
func (wm *WorkspaceManager) SetBulletproofDebugMode(debug bool) {
	if wm.treeValidator != nil {
		wm.treeValidator.SetDebugMode(debug)
	}
	if wm.geometryValidator != nil {
		wm.geometryValidator.SetDebugMode(debug)
	}
	log.Printf("[bulletproof] Debug mode set to: %v", debug)
}

// GetBulletproofStats returns comprehensive statistics about all bulletproof components
func (wm *WorkspaceManager) GetBulletproofStats() map[string]interface{} {
	stats := make(map[string]interface{})

	if wm.treeValidator != nil {
		stats["tree_validation"] = wm.treeValidator.GetValidationStats()
	}

	if wm.widgetTxManager != nil {
		stats["widget_transactions"] = wm.widgetTxManager.GetTransactionStats()
	}

	if wm.concurrencyController != nil {
		stats["concurrency"] = wm.concurrencyController.GetConcurrencyStats()
	}

	if wm.treeRebalancer != nil {
		stats["tree_rebalancing"] = wm.treeRebalancer.GetRebalancingStats()
	}

	if wm.geometryValidator != nil {
		stats["geometry"] = wm.geometryValidator.GetGeometryStats(wm.root)
	}

	if wm.stateTombstoneManager != nil {
		stats["tombstones"] = wm.stateTombstoneManager.GetTombstoneStats()
	}

	return stats
}

// ValidateWorkspaceIntegrity performs a comprehensive validation of the workspace
func (wm *WorkspaceManager) ValidateWorkspaceIntegrity() error {
	log.Printf("[bulletproof] Performing comprehensive workspace integrity check")

	// Tree structure validation
	if wm.treeValidator != nil {
		if err := wm.treeValidator.ValidateTree(wm.root, "integrity_check"); err != nil {
			return fmt.Errorf("tree validation failed: %w", err)
		}
	}

	// Geometry validation
	if wm.geometryValidator != nil {
		results := wm.geometryValidator.ValidateWorkspaceLayout(wm.root)
		for i, result := range results {
			if !result.IsValid {
				log.Printf("[bulletproof] Geometry validation failed for pane %d: %s", i, result.Reason)
				// Don't fail the entire check for geometry issues
			}
		}
	}

	log.Printf("[bulletproof] Workspace integrity check completed successfully")
	return nil
}

// ShutdownBulletproofComponents gracefully shuts down all bulletproof components
func (wm *WorkspaceManager) ShutdownBulletproofComponents() {
	log.Printf("[bulletproof] Shutting down bulletproof components")

	if wm.concurrencyController != nil {
		wm.concurrencyController.Shutdown()
	}

	// Other components don't require explicit shutdown currently
	log.Printf("[bulletproof] Bulletproof components shutdown complete")
}
