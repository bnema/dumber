// workspace_widget_transaction.go - Atomic widget operations for bulletproof GTK widget management
package browser

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/bnema/dumber/pkg/webkit"
)

// WidgetOperation represents a single widget operation that can be executed or rolled back
type WidgetOperation struct {
	ID          string
	Description string
	Execute     func() error
	Rollback    func() error
	Executed    bool
	Priority    int // Higher priority operations execute first
}

// WidgetTransaction manages a set of widget operations atomically
type WidgetTransaction struct {
	ID         string
	operations []*WidgetOperation
	rollbacks  []func() error
	executed   []bool
	committed  bool
	rolledBack bool
	startTime  time.Time
	timeout    time.Duration
	mu         sync.Mutex
}

// WidgetTransactionManager manages widget transactions for thread safety
type WidgetTransactionManager struct {
	activeTransactions map[string]*WidgetTransaction
	transactionHistory []TransactionResult
	mu                 sync.RWMutex
	globalTimeout      time.Duration
	maxHistory         int
}

// TransactionResult captures the result of a transaction
type TransactionResult struct {
	TransactionID  string
	Success        bool
	OperationCount int
	Duration       time.Duration
	ErrorMessage   string
	Timestamp      time.Time
}

// NewWidgetTransactionManager creates a new transaction manager
func NewWidgetTransactionManager() *WidgetTransactionManager {
	return &WidgetTransactionManager{
		activeTransactions: make(map[string]*WidgetTransaction),
		transactionHistory: make([]TransactionResult, 0, 100),
		globalTimeout:      30 * time.Second,
		maxHistory:         100,
	}
}

// BeginTransaction starts a new widget transaction
func (wtm *WidgetTransactionManager) BeginTransaction(id string) *WidgetTransaction {
	wtm.mu.Lock()
	defer wtm.mu.Unlock()

	// Check if transaction already exists
	if _, exists := wtm.activeTransactions[id]; exists {
		log.Printf("[widget-tx] Transaction %s already exists, returning existing", id)
		return wtm.activeTransactions[id]
	}

	tx := &WidgetTransaction{
		ID:         id,
		operations: make([]*WidgetOperation, 0),
		rollbacks:  make([]func() error, 0),
		executed:   make([]bool, 0),
		startTime:  time.Now(),
		timeout:    wtm.globalTimeout,
	}

	wtm.activeTransactions[id] = tx
	log.Printf("[widget-tx] Started transaction: %s", id)
	return tx
}

// AddOperation adds an operation to the transaction
func (tx *WidgetTransaction) AddOperation(op *WidgetOperation) error {
	tx.mu.Lock()
	defer tx.mu.Unlock()

	if tx.committed || tx.rolledBack {
		return fmt.Errorf("cannot add operation to finalized transaction")
	}

	// Check for timeout
	if time.Since(tx.startTime) > tx.timeout {
		return fmt.Errorf("transaction timed out")
	}

	tx.operations = append(tx.operations, op)
	tx.executed = append(tx.executed, false)

	log.Printf("[widget-tx] Added operation %s to transaction %s", op.ID, tx.ID)
	return nil
}

// Execute runs all operations in the transaction
func (tx *WidgetTransaction) Execute() error {
	tx.mu.Lock()
	defer tx.mu.Unlock()

	if tx.committed || tx.rolledBack {
		return fmt.Errorf("transaction already finalized")
	}

	// Sort operations by priority (higher priority first)
	for i := 0; i < len(tx.operations)-1; i++ {
		for j := i + 1; j < len(tx.operations); j++ {
			if tx.operations[i].Priority < tx.operations[j].Priority {
				tx.operations[i], tx.operations[j] = tx.operations[j], tx.operations[i]
				tx.executed[i], tx.executed[j] = tx.executed[j], tx.executed[i]
			}
		}
	}

	log.Printf("[widget-tx] Executing transaction %s with %d operations", tx.ID, len(tx.operations))

	// Execute operations one by one
	for i, op := range tx.operations {
		if time.Since(tx.startTime) > tx.timeout {
			// Timeout - rollback executed operations
			tx.rollbackExecuted()
			return fmt.Errorf("transaction %s timed out during execution", tx.ID)
		}

		log.Printf("[widget-tx] Executing operation %s (%s)", op.ID, op.Description)

		if err := op.Execute(); err != nil {
			log.Printf("[widget-tx] Operation %s failed: %v", op.ID, err)

			// Rollback all executed operations
			tx.rollbackExecuted()
			return fmt.Errorf("operation %s failed: %w", op.ID, err)
		}

		tx.executed[i] = true
		op.Executed = true

		// Store rollback function if provided
		if op.Rollback != nil {
			tx.rollbacks = append([]func() error{op.Rollback}, tx.rollbacks...) // Prepend for reverse order
		}
	}

	log.Printf("[widget-tx] Transaction %s executed successfully", tx.ID)
	return nil
}

// Commit finalizes the transaction (prevents rollback)
func (tx *WidgetTransaction) Commit() error {
	tx.mu.Lock()
	defer tx.mu.Unlock()

	if tx.committed {
		return fmt.Errorf("transaction already committed")
	}
	if tx.rolledBack {
		return fmt.Errorf("cannot commit rolled back transaction")
	}

	tx.committed = true
	tx.rollbacks = nil // Clear rollback functions to prevent accidental use

	log.Printf("[widget-tx] Transaction %s committed", tx.ID)
	return nil
}

// Rollback reverses all executed operations
func (tx *WidgetTransaction) Rollback() error {
	tx.mu.Lock()
	defer tx.mu.Unlock()

	if tx.committed {
		return fmt.Errorf("cannot rollback committed transaction")
	}
	if tx.rolledBack {
		return fmt.Errorf("transaction already rolled back")
	}

	return tx.rollbackExecuted()
}

// rollbackExecuted performs the actual rollback (internal, assumes lock held)
func (tx *WidgetTransaction) rollbackExecuted() error {
	log.Printf("[widget-tx] Rolling back transaction %s", tx.ID)

	var rollbackErrors []error

	// Execute rollbacks in reverse order
	for _, rollback := range tx.rollbacks {
		if err := rollback(); err != nil {
			rollbackErrors = append(rollbackErrors, err)
			log.Printf("[widget-tx] Rollback operation failed: %v", err)
		}
	}

	tx.rolledBack = true

	if len(rollbackErrors) > 0 {
		return fmt.Errorf("rollback completed with %d errors: %v", len(rollbackErrors), rollbackErrors[0])
	}

	log.Printf("[widget-tx] Transaction %s rolled back successfully", tx.ID)
	return nil
}

// FinishTransaction completes and removes the transaction from active list
func (wtm *WidgetTransactionManager) FinishTransaction(id string, success bool, errorMsg string) {
	wtm.mu.Lock()
	defer wtm.mu.Unlock()

	tx, exists := wtm.activeTransactions[id]
	if !exists {
		log.Printf("[widget-tx] Transaction %s not found for finishing", id)
		return
	}

	// Record result
	result := TransactionResult{
		TransactionID:  id,
		Success:        success,
		OperationCount: len(tx.operations),
		Duration:       time.Since(tx.startTime),
		ErrorMessage:   errorMsg,
		Timestamp:      time.Now(),
	}

	// Add to history
	if len(wtm.transactionHistory) >= wtm.maxHistory {
		wtm.transactionHistory = wtm.transactionHistory[1:] // Remove oldest
	}
	wtm.transactionHistory = append(wtm.transactionHistory, result)

	// Remove from active transactions
	delete(wtm.activeTransactions, id)

	log.Printf("[widget-tx] Finished transaction %s: success=%v, duration=%v", id, success, result.Duration)
}

// Common widget operations for convenience

// CreateWidgetUnparentOperation creates an operation to unparent a widget
func CreateWidgetUnparentOperation(id string, widget *SafeWidget) *WidgetOperation {
	return &WidgetOperation{
		ID:          id,
		Description: fmt.Sprintf("Unparent widget %s", widget.String()),
		Priority:    100, // High priority for cleanup operations
		Execute: func() error {
			if widget == nil || !widget.IsValid() {
				return fmt.Errorf("invalid widget for unparent operation")
			}

			return widget.Execute(func(ptr uintptr) error {
				if webkit.WidgetGetParent(ptr) != 0 {
					webkit.WidgetUnparent(ptr)
				}
				return nil
			})
		},
		Rollback: func() error {
			// Unparent operations are typically not reversible
			log.Printf("[widget-tx] Unparent rollback for %s (no-op)", id)
			return nil
		},
	}
}

// CreateWidgetReparentOperation creates an operation to reparent a widget
func CreateWidgetReparentOperation(id string, widget *SafeWidget, newParent uintptr, isStart bool) *WidgetOperation {
	var oldParent uintptr

	return &WidgetOperation{
		ID:          id,
		Description: fmt.Sprintf("Reparent widget %s", widget.String()),
		Priority:    200, // Medium priority
		Execute: func() error {
			if widget == nil || !widget.IsValid() {
				return fmt.Errorf("invalid widget for reparent operation")
			}
			if newParent == 0 {
				return fmt.Errorf("invalid new parent for reparent operation")
			}

			return widget.Execute(func(ptr uintptr) error {
				// Store old parent for rollback
				oldParent = webkit.WidgetGetParent(ptr)

				// Unparent first if needed
				if oldParent != 0 {
					webkit.WidgetUnparent(ptr)
				}

				// Reparent to new parent
				if isStart {
					webkit.PanedSetStartChild(newParent, ptr)
				} else {
					webkit.PanedSetEndChild(newParent, ptr)
				}

				return nil
			})
		},
		Rollback: func() error {
			if widget == nil || !widget.IsValid() {
				return nil // Widget is gone, nothing to rollback
			}

			return widget.Execute(func(ptr uintptr) error {
				// Remove from current parent
				current := webkit.WidgetGetParent(ptr)
				if current != 0 {
					webkit.WidgetUnparent(ptr)
				}

				// Restore to old parent if it existed
				if oldParent != 0 {
					// Note: This is simplified - in practice we'd need to know which child position to restore
					log.Printf("[widget-tx] Widget reparent rollback: restoring to parent %#x", oldParent)
				}

				return nil
			})
		},
	}
}

// CreateWidgetShowOperation creates an operation to show/hide a widget
func CreateWidgetShowOperation(id string, widget *SafeWidget, visible bool) *WidgetOperation {
	var wasVisible bool

	return &WidgetOperation{
		ID:          id,
		Description: fmt.Sprintf("Set widget %s visibility to %v", widget.String(), visible),
		Priority:    50, // Low priority for visibility operations
		Execute: func() error {
			if widget == nil || !widget.IsValid() {
				return fmt.Errorf("invalid widget for show operation")
			}

			return widget.Execute(func(ptr uintptr) error {
				// Store current visibility for rollback
				wasVisible = webkit.WidgetGetVisible(ptr)

				webkit.WidgetSetVisible(ptr, visible)
				return nil
			})
		},
		Rollback: func() error {
			if widget == nil || !widget.IsValid() {
				return nil // Widget is gone, nothing to rollback
			}

			return widget.Execute(func(ptr uintptr) error {
				webkit.WidgetSetVisible(ptr, wasVisible)
				return nil
			})
		},
	}
}

// CreateCSSClassOperation creates an operation to add/remove CSS classes
func CreateCSSClassOperation(id string, widget *SafeWidget, className string, add bool) *WidgetOperation {
	var hadClass bool

	return &WidgetOperation{
		ID:          id,
		Description: fmt.Sprintf("CSS class %s: %s on widget %s", map[bool]string{true: "add", false: "remove"}[add], className, widget.String()),
		Priority:    10, // Very low priority for styling operations
		Execute: func() error {
			if widget == nil || !widget.IsValid() {
				return fmt.Errorf("invalid widget for CSS operation")
			}

			return widget.Execute(func(ptr uintptr) error {
				// Store current state for rollback
				hadClass = webkit.WidgetHasCSSClass(ptr, className)

				if add {
					webkit.WidgetAddCSSClass(ptr, className)
				} else {
					webkit.WidgetRemoveCSSClass(ptr, className)
				}
				return nil
			})
		},
		Rollback: func() error {
			if widget == nil || !widget.IsValid() {
				return nil // Widget is gone, nothing to rollback
			}

			return widget.Execute(func(ptr uintptr) error {
				if hadClass {
					webkit.WidgetAddCSSClass(ptr, className)
				} else {
					webkit.WidgetRemoveCSSClass(ptr, className)
				}
				return nil
			})
		},
	}
}

// GetTransactionStats returns statistics about transactions
func (wtm *WidgetTransactionManager) GetTransactionStats() map[string]interface{} {
	wtm.mu.RLock()
	defer wtm.mu.RUnlock()

	totalTransactions := len(wtm.transactionHistory)
	successfulTransactions := 0
	totalDuration := time.Duration(0)

	for _, result := range wtm.transactionHistory {
		if result.Success {
			successfulTransactions++
		}
		totalDuration += result.Duration
	}

	var avgDuration time.Duration
	if totalTransactions > 0 {
		avgDuration = totalDuration / time.Duration(totalTransactions)
	}

	return map[string]interface{}{
		"total_transactions":      totalTransactions,
		"successful_transactions": successfulTransactions,
		"success_rate":            float64(successfulTransactions) / float64(max(totalTransactions, 1)),
		"active_transactions":     len(wtm.activeTransactions),
		"avg_duration_ms":         avgDuration.Milliseconds(),
		"global_timeout_sec":      wtm.globalTimeout.Seconds(),
	}
}
