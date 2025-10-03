// workspace_operations.go - Workspace operation methods with validation and safety
package browser

import (
	"fmt"
	"log"

	"github.com/bnema/dumber/pkg/webkit"
)

// SplitPane performs a split operation with validation and safety checks
func (wm *WorkspaceManager) SplitPane(target *paneNode, direction string) (*paneNode, error) {
	if wm == nil {
		return nil, fmt.Errorf("workspace manager is nil")
	}

	log.Printf("[workspace] Starting split operation: target=%p direction=%s", target, direction)

	// Step 1: Validate geometry constraints (only in DebugBasic+)
	if wm.debugLevel >= DebugBasic {
		validation := wm.geometryValidator.ValidateSplit(target, direction)
		if !validation.IsValid {
			if wm.debugLevel == DebugFull {
				return nil, fmt.Errorf("split validation failed: %s", validation.Reason)
			}
			log.Printf("[workspace] WARNING: Split validation failed but allowing operation: %s", validation.Reason)
		}

		// Log if re-validation will be needed due to pending widget allocation
		if validation.RequiresRevalidation {
			log.Printf("[workspace] Split validation passed with pending allocation - operation will proceed")
		}

		// Step 3: Validate tree invariants before operation
		if err := wm.treeValidator.ValidateTree(wm.root, "before_split"); err != nil {
			if wm.debugLevel == DebugFull {
				return nil, fmt.Errorf("tree validation failed before split: %w", err)
			}
			log.Printf("[workspace] WARNING: Tree validation failed before split but allowing operation: %v", err)
		}
	}

	// Step 2: Execute directly if we're already on the GTK main thread
	if webkit.IsMainThread() {
		log.Printf("[workspace] Already on main thread, executing split directly")
		newNode, err := wm.splitNode(target, direction)
		if err != nil {
			return nil, err
		}

		if wm.debugLevel >= DebugBasic {
			if err := wm.treeValidator.ValidateTree(wm.root, "after_split"); err != nil {
				log.Printf("[workspace] Tree validation failed after split: %v", err)
			}
		}

		// Tree rebalancing is only needed after close promotions. Splits have correct allocation from GTK.

		log.Printf("[workspace] Split operation completed successfully (direct execution): newNode=%p", newNode)
		return newNode, nil
	}

	// Step 3: Not on main thread, marshal via IdleAdd
	log.Printf("[workspace] Not on main thread, marshalling split via IdleAdd")
	var newNode *paneNode
	var splitErr error
	done := make(chan struct{})

	_ = webkit.IdleAdd(func() bool {
		newNode, splitErr = wm.splitNode(target, direction)
		if splitErr == nil {
			if wm.debugLevel >= DebugBasic {
				if verr := wm.treeValidator.ValidateTree(wm.root, "after_split"); verr != nil {
					log.Printf("[workspace] Tree validation failed after split: %v", verr)
				}
			}
		}
		close(done)
		return false
	})

	<-done

	if splitErr != nil {
		return nil, splitErr
	}

	log.Printf("[workspace] Split operation completed successfully: newNode=%p", newNode)
	return newNode, nil
}

// ClosePane performs a close operation with validation and safety checks
func (wm *WorkspaceManager) ClosePane(node *paneNode) error {
	if wm == nil {
		return fmt.Errorf("workspace manager is nil")
	}

	log.Printf("[workspace] Starting close operation: node=%p", node)
	wm.paneCloseLogf("start bulletproof close node=%p", node)
	wm.dumpTreeState("before_close")

	// Step 1: Validate tree invariants before operation (only in DebugBasic+)
	if wm.debugLevel >= DebugBasic {
		if err := wm.treeValidator.ValidateTree(wm.root, "before_close"); err != nil {
			if wm.debugLevel == DebugFull {
				return fmt.Errorf("tree validation failed before close: %w", err)
			}
			log.Printf("[workspace] WARNING: Tree validation failed before close but allowing operation: %v", err)
		}
	}

	// Step 3: Check if this is a stacked pane and use enhanced lifecycle management
	if node.parent != nil && node.parent.isStacked {
		log.Printf("[workspace] Using enhanced stack lifecycle management for close")
		return wm.stackLifecycleManager.CloseStackedPaneWithLifecycle(node)
	}

	// Step 4: Execute directly when already on the GTK main thread
	if webkit.IsMainThread() {
		log.Printf("[workspace] Already on main thread, executing close directly")

		wm.paneCloseLogf("invoking closePane node=%p", node)
		promoted, err := wm.closePane(node)
		if err != nil {
			wm.paneCloseLogf("closePane failed node=%p err=%v", node, err)
			wm.dumpTreeState("after_close_error")
			return err
		}

		if wm.debugLevel >= DebugBasic {
			if err := wm.treeValidator.ValidateTree(wm.root, "after_close"); err != nil {
				log.Printf("[workspace] Tree validation failed after close: %v", err)
			}
		}
		wm.paneCloseLogf("closePane succeeded node=%p promoted=%p root=%p", node, promoted, wm.root)
		wm.dumpTreeState("after_close_success")

		if wm.treeRebalancer != nil {
			if err := wm.treeRebalancer.RebalanceAfterClose(node, promoted); err != nil {
				log.Printf("[workspace] Tree rebalancing failed after close: %v", err)
			}
		}

		log.Printf("[workspace] Close operation completed successfully (direct execution)")
		return nil
	}

	// Step 5: Not on main thread, marshal via IdleAdd
	log.Printf("[workspace] Not on main thread, marshalling close via IdleAdd")
	var promoted *paneNode
	var closeErr error
	done := make(chan struct{})

	_ = webkit.IdleAdd(func() bool {
		wm.paneCloseLogf("invoking closePane node=%p", node)
		promoted, closeErr = wm.closePane(node)
		if closeErr != nil {
			wm.paneCloseLogf("closePane failed node=%p err=%v", node, closeErr)
			wm.dumpTreeState("after_close_error")
		} else {
			if wm.debugLevel >= DebugBasic {
				if verr := wm.treeValidator.ValidateTree(wm.root, "after_close"); verr != nil {
					log.Printf("[workspace] Tree validation failed after close: %v", verr)
				}
			}
			wm.paneCloseLogf("closePane succeeded node=%p promoted=%p root=%p", node, promoted, wm.root)
			wm.dumpTreeState("after_close_success")

			if wm.treeRebalancer != nil {
				if rerr := wm.treeRebalancer.RebalanceAfterClose(node, promoted); rerr != nil {
					log.Printf("[workspace] Tree rebalancing failed after close: %v", rerr)
				}
			}
		}
		close(done)
		return false
	})

	<-done

	if closeErr != nil {
		return closeErr
	}

	log.Printf("[workspace] Close operation completed successfully")
	return nil
}

// StackPane performs a stack operation with validation and safety checks
func (wm *WorkspaceManager) StackPane(target *paneNode) (*paneNode, error) {
	if wm == nil {
		return nil, fmt.Errorf("workspace manager is nil")
	}

	log.Printf("[workspace] Starting stack operation: target=%p", target)

	// Step 1: Validate stack operation constraints (only in DebugBasic+)
	if wm.debugLevel >= DebugBasic {
		validation := wm.geometryValidator.ValidateStackOperation(target)
		if !validation.IsValid {
			if wm.debugLevel == DebugFull {
				return nil, fmt.Errorf("stack validation failed: %s", validation.Reason)
			}
			log.Printf("[workspace] WARNING: Stack validation failed but allowing operation: %s", validation.Reason)
		}

		// Log if re-validation will be needed due to pending widget allocation
		if validation.RequiresRevalidation {
			log.Printf("[workspace] Stack validation passed with pending allocation - operation will proceed")
		}
	}

	// Step 2: Validate tree invariants before operation (only in DebugBasic+)
	if wm.debugLevel >= DebugBasic {
		if err := wm.treeValidator.ValidateTree(wm.root, "before_stack"); err != nil {
			if wm.debugLevel == DebugFull {
				return nil, fmt.Errorf("tree validation failed before stack: %w", err)
			}
			log.Printf("[workspace] WARNING: Tree validation failed before stack but allowing operation: %v", err)
		}
	}

	// Step 3: Execute directly if already on the GTK main thread
	if webkit.IsMainThread() {
		log.Printf("[workspace] Already on main thread, executing stack directly")
		newNode, err := wm.stackedPaneManager.StackPane(target)
		if err != nil {
			return nil, err
		}

		if wm.debugLevel >= DebugBasic {
			if err := wm.treeValidator.ValidateTree(wm.root, "after_stack"); err != nil {
				log.Printf("[workspace] Tree validation failed after stack: %v", err)
			}
		}

		log.Printf("[workspace] Stack operation completed successfully (direct execution): newNode=%p", newNode)
		return newNode, nil
	}

	// Step 4: Not on main thread, marshal via IdleAdd
	log.Printf("[workspace] Not on main thread, marshalling stack via IdleAdd")
	var newNode *paneNode
	var stackErr error
	done := make(chan struct{})

	_ = webkit.IdleAdd(func() bool {
		newNode, stackErr = wm.stackedPaneManager.StackPane(target)
		if stackErr == nil {
			if wm.debugLevel >= DebugBasic {
				if verr := wm.treeValidator.ValidateTree(wm.root, "after_stack"); verr != nil {
					log.Printf("[workspace] Tree validation failed after stack: %v", verr)
				}
			}
		}
		close(done)
		return false
	})

	<-done

	if stackErr != nil {
		return nil, stackErr
	}

	log.Printf("[workspace] Stack operation completed successfully: newNode=%p", newNode)
	return newNode, nil
}

// EnableEnhancedMode enables all enhanced validation features
func (wm *WorkspaceManager) EnableEnhancedMode() {
	if wm.treeValidator != nil {
		wm.treeValidator.Enable()
	}
	if wm.treeRebalancer != nil {
		wm.treeRebalancer.Enable()
	}
	log.Printf("[workspace] Enhanced validation mode enabled")
}

// DisableEnhancedMode disables enhanced features for performance
func (wm *WorkspaceManager) DisableEnhancedMode() {
	if wm.treeValidator != nil {
		wm.treeValidator.Disable()
	}
	if wm.treeRebalancer != nil {
		wm.treeRebalancer.Disable()
	}
	log.Printf("[workspace] Enhanced validation mode disabled")
}

// SetEnhancedDebugMode enables/disables debug mode for all enhanced validation components
func (wm *WorkspaceManager) SetEnhancedDebugMode(debug bool) {
	if wm.treeValidator != nil {
		wm.treeValidator.SetDebugMode(debug)
	}
	if wm.geometryValidator != nil {
		wm.geometryValidator.SetDebugMode(debug)
	}
	log.Printf("[workspace] Enhanced debug mode set to: %v", debug)
}

// GetEnhancedStats returns comprehensive statistics about all enhanced validation components
func (wm *WorkspaceManager) GetEnhancedStats() map[string]interface{} {
	stats := make(map[string]interface{})

	if wm.treeValidator != nil {
		stats["tree_validation"] = wm.treeValidator.GetValidationStats()
	}

	if wm.treeRebalancer != nil {
		stats["tree_rebalancing"] = wm.treeRebalancer.GetRebalancingStats()
	}

	// Geometry validator no longer tracks stats - simplified

	return stats
}

// ValidateWorkspaceIntegrity performs a comprehensive validation of the workspace
func (wm *WorkspaceManager) ValidateWorkspaceIntegrity() error {
	log.Printf("[workspace] Performing comprehensive workspace integrity check")

	// Tree structure validation
	if wm.treeValidator != nil {
		if err := wm.treeValidator.ValidateTree(wm.root, "integrity_check"); err != nil {
			return fmt.Errorf("tree validation failed: %w", err)
		}
	}

	// Geometry validation simplified - ValidateWorkspaceLayout removed

	log.Printf("[workspace] Workspace integrity check completed successfully")
	return nil
}

// ShutdownEnhancedComponents gracefully shuts down all enhanced validation components
func (wm *WorkspaceManager) ShutdownEnhancedComponents() {
	log.Printf("[workspace] Shutting down enhanced validation components")

	// Other components don't require explicit shutdown currently
	log.Printf("[workspace] Enhanced validation components shutdown complete")
}
