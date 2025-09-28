// workspace_concurrency.go - Sequence-based concurrency control for bulletproof tree operations
package browser

import (
	"context"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bnema/dumber/pkg/webkit"
)

// OperationType represents different types of tree operations
type OperationType int

const (
	OpTypeSplit OperationType = iota
	OpTypeClose
	OpTypeStack
	OpTypeFocus
	OpTypeResize
)

func (ot OperationType) String() string {
	switch ot {
	case OpTypeSplit:
		return "split"
	case OpTypeClose:
		return "close"
	case OpTypeStack:
		return "stack"
	case OpTypeFocus:
		return "focus"
	case OpTypeResize:
		return "resize"
	default:
		return "unknown"
	}
}

// OperationRequest represents a pending tree operation
type OperationRequest struct {
	ID          string
	Type        OperationType
	TargetNode  *paneNode
	Direction   string // For split operations
	Parameters  map[string]interface{}
	SequenceNum uint64
	SubmittedAt time.Time
	Context     context.Context
	ResultChan  chan OperationResult
	RetryCount  int
	MaxRetries  int
}

// OperationResult represents the result of an operation
type OperationResult struct {
	Success     bool
	Error       error
	NewNode     *paneNode
	Duration    time.Duration
	SequenceNum uint64
}

// ConcurrencyController manages concurrent access to the tree
type ConcurrencyController struct {
	// Sequence numbers for operation ordering
	globalSequence uint64
	nodeSequences  map[*paneNode]uint64
	sequenceMutex  sync.RWMutex

	// Operation queuing and processing
	operationQueue chan *OperationRequest
	workerCount    int
	workers        []*OperationWorker
	shutdown       chan struct{}
	shutdownOnce   sync.Once

	// Tree-level synchronization
	treeMutex       sync.RWMutex
	readOperations  map[string]context.CancelFunc // Read operations that can be cancelled
	writeOperations map[string]*OperationRequest  // Active write operations

	// Performance tracking
	operationHistory []OperationStats
	historyMutex     sync.RWMutex
	maxHistory       int

	// Configuration
	maxQueueSize     int
	operationTimeout time.Duration
	retryBackoff     time.Duration
	deadlockTimeout  time.Duration

	// Dependencies
	widgetTxManager  *WidgetTransactionManager
	treeValidator    *TreeValidator
	workspaceManager *WorkspaceManager
}

// OperationStats tracks performance metrics for operations
type OperationStats struct {
	OperationType OperationType
	Duration      time.Duration
	Success       bool
	RetryCount    int
	QueueTime     time.Duration
	SequenceNum   uint64
	Timestamp     time.Time
	ConflictCount int
}

// OperationWorker processes operations from the queue
type OperationWorker struct {
	id         int
	controller *ConcurrencyController
	shutdown   chan struct{}
	wg         *sync.WaitGroup
}

// NewConcurrencyController creates a new concurrency controller
func NewConcurrencyController(workerCount int, widgetTxManager *WidgetTransactionManager, treeValidator *TreeValidator) *ConcurrencyController {
	if workerCount <= 0 {
		workerCount = 2 // Default to 2 workers
	}

	cc := &ConcurrencyController{
		globalSequence:   0,
		nodeSequences:    make(map[*paneNode]uint64),
		operationQueue:   make(chan *OperationRequest, 1000), // Buffered channel
		workerCount:      workerCount,
		workers:          make([]*OperationWorker, workerCount),
		shutdown:         make(chan struct{}),
		readOperations:   make(map[string]context.CancelFunc),
		writeOperations:  make(map[string]*OperationRequest),
		operationHistory: make([]OperationStats, 0, 1000),
		maxHistory:       1000,
		maxQueueSize:     1000,
		operationTimeout: 10 * time.Second,
		retryBackoff:     100 * time.Millisecond,
		deadlockTimeout:  30 * time.Second,
		widgetTxManager:  widgetTxManager,
		treeValidator:    treeValidator,
	}

	// Start workers
	var wg sync.WaitGroup
	for i := 0; i < workerCount; i++ {
		worker := &OperationWorker{
			id:         i,
			controller: cc,
			shutdown:   make(chan struct{}),
			wg:         &wg,
		}
		cc.workers[i] = worker
		wg.Add(1)
		go worker.run()
	}

	log.Printf("[concurrency] Started concurrency controller with %d workers", workerCount)
	return cc
}

// SetWorkspaceManager sets the workspace manager reference (called after construction to avoid circular dependency)
func (cc *ConcurrencyController) SetWorkspaceManager(wm *WorkspaceManager) {
	cc.workspaceManager = wm
}

// SubmitOperation submits an operation for execution
func (cc *ConcurrencyController) SubmitOperation(req *OperationRequest) <-chan OperationResult {
	// Assign sequence number
	req.SequenceNum = atomic.AddUint64(&cc.globalSequence, 1)
	req.SubmittedAt = time.Now()
	req.ResultChan = make(chan OperationResult, 1)

	// Set default retry count if not specified
	if req.MaxRetries == 0 {
		req.MaxRetries = 3
	}

	// Set default context if not provided
	if req.Context == nil {
		ctx, cancel := context.WithTimeout(context.Background(), cc.operationTimeout)
		req.Context = ctx
		// Schedule cleanup
		go func() {
			<-ctx.Done()
			cancel()
		}()
	}

	// Try to queue the operation
	select {
	case cc.operationQueue <- req:
		log.Printf("[concurrency] Queued operation %s (seq=%d, type=%s)", req.ID, req.SequenceNum, req.Type)
	case <-req.Context.Done():
		// Context cancelled before queuing
		req.ResultChan <- OperationResult{
			Success: false,
			Error:   fmt.Errorf("operation cancelled before queuing: %w", req.Context.Err()),
		}
	default:
		// Queue is full
		req.ResultChan <- OperationResult{
			Success: false,
			Error:   fmt.Errorf("operation queue is full"),
		}
	}

	return req.ResultChan
}

// worker run loop
func (worker *OperationWorker) run() {
	defer worker.wg.Done()
	log.Printf("[concurrency] Worker %d started", worker.id)

	for {
		select {
		case req := <-worker.controller.operationQueue:
			worker.processOperation(req)
		case <-worker.shutdown:
			log.Printf("[concurrency] Worker %d shutting down", worker.id)
			return
		case <-worker.controller.shutdown:
			log.Printf("[concurrency] Worker %d shutting down (global)", worker.id)
			return
		}
	}
}

// processOperation processes a single operation request
func (worker *OperationWorker) processOperation(req *OperationRequest) {
	startTime := time.Now()
	queueTime := startTime.Sub(req.SubmittedAt)

	log.Printf("[concurrency] Worker %d processing operation %s (seq=%d, queue_time=%v)",
		worker.id, req.ID, req.SequenceNum, queueTime)

	result := OperationResult{
		SequenceNum: req.SequenceNum,
	}

	// Check if context is already cancelled
	select {
	case <-req.Context.Done():
		result.Success = false
		result.Error = fmt.Errorf("operation context cancelled: %w", req.Context.Err())
		worker.sendResult(req, result, startTime, queueTime)
		return
	default:
	}

	// Check for conflicts and acquire locks
	conflicts := worker.checkConflicts(req)
	if conflicts > 0 && req.RetryCount < req.MaxRetries {
		// Retry with backoff
		req.RetryCount++
		backoff := time.Duration(req.RetryCount) * worker.controller.retryBackoff

		log.Printf("[concurrency] Operation %s has %d conflicts, retrying in %v (attempt %d/%d)",
			req.ID, conflicts, backoff, req.RetryCount, req.MaxRetries)

		go func() {
			time.Sleep(backoff)
			select {
			case worker.controller.operationQueue <- req:
			case <-req.Context.Done():
				result.Success = false
				result.Error = fmt.Errorf("operation cancelled during retry")
				req.ResultChan <- result
			}
		}()
		return
	}

	if conflicts > 0 {
		result.Success = false
		result.Error = fmt.Errorf("operation failed after %d retries due to conflicts", req.MaxRetries)
		worker.sendResult(req, result, startTime, queueTime)
		return
	}

	// Execute the operation
	err := worker.executeOperation(req, &result)
	if err != nil {
		result.Success = false
		result.Error = err
	} else {
		result.Success = true
	}

	// Record stats and send result
	worker.sendResult(req, result, startTime, queueTime)
}

// checkConflicts checks for operation conflicts
func (worker *OperationWorker) checkConflicts(req *OperationRequest) int {
	cc := worker.controller
	conflicts := 0

	cc.treeMutex.RLock()
	defer cc.treeMutex.RUnlock()

	// Check for conflicting write operations on the same node
	for _, activeOp := range cc.writeOperations {
		if activeOp.TargetNode == req.TargetNode {
			conflicts++
		}
		// Check for parent-child conflicts
		if req.TargetNode != nil && activeOp.TargetNode != nil {
			if isAncestor(activeOp.TargetNode, req.TargetNode) || isAncestor(req.TargetNode, activeOp.TargetNode) {
				conflicts++
			}
		}
	}

	return conflicts
}

// executeOperation executes the actual operation
func (worker *OperationWorker) executeOperation(req *OperationRequest, result *OperationResult) error {
	cc := worker.controller

	// Acquire tree write lock
	cc.treeMutex.Lock()
	defer cc.treeMutex.Unlock()

	// Register this operation as active
	cc.writeOperations[req.ID] = req
	defer delete(cc.writeOperations, req.ID)

	// Update node sequence
	if req.TargetNode != nil {
		cc.sequenceMutex.Lock()
		cc.nodeSequences[req.TargetNode] = req.SequenceNum
		cc.sequenceMutex.Unlock()
	}

	// Create widget transaction for this operation
	txID := fmt.Sprintf("op_%s_%d", req.ID, req.SequenceNum)
	tx := cc.widgetTxManager.BeginTransaction(txID)

	// Execute based on operation type
	var err error
	switch req.Type {
	case OpTypeSplit:
		result.NewNode, err = worker.executeSplitOperation(req, tx)
	case OpTypeClose:
		result.NewNode, err = worker.executeCloseOperation(req, tx)
	case OpTypeStack:
		result.NewNode, err = worker.executeStackOperation(req, tx)
	case OpTypeFocus:
		err = worker.executeFocusOperation(req, tx)
	case OpTypeResize:
		err = worker.executeResizeOperation(req, tx)
	default:
		err = fmt.Errorf("unknown operation type: %s", req.Type)
	}

	if err != nil {
		// Rollback transaction on error
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			log.Printf("[concurrency] Failed to rollback transaction %s: %v", txID, rollbackErr)
		}
		cc.widgetTxManager.FinishTransaction(txID, false, err.Error())
		return err
	}

	// Execute widget transaction
	if err = tx.Execute(); err != nil {
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			log.Printf("[concurrency] Failed to rollback transaction %s: %v", txID, rollbackErr)
		}
		cc.widgetTxManager.FinishTransaction(txID, false, err.Error())
		return fmt.Errorf("widget transaction failed: %w", err)
	}

	// Commit transaction
	if err = tx.Commit(); err != nil {
		cc.widgetTxManager.FinishTransaction(txID, false, err.Error())
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	cc.widgetTxManager.FinishTransaction(txID, true, "")

	// Validate tree if validator is available
	if cc.treeValidator != nil {
		if validateErr := cc.treeValidator.ValidateTree(req.TargetNode, req.Type.String()); validateErr != nil {
			log.Printf("[concurrency] Tree validation failed after operation %s: %v", req.ID, validateErr)
			// Don't return error for validation failures in production, just log
		}
	}

	return nil
}

// Placeholder implementations for operation execution
func (worker *OperationWorker) executeSplitOperation(req *OperationRequest, tx *WidgetTransaction) (*paneNode, error) {
	log.Printf("[concurrency] Executing split operation on node %p direction=%s", req.TargetNode, req.Direction)

	// Get the workspace manager from the controller
	wm := worker.controller.workspaceManager
	if wm == nil {
		return nil, fmt.Errorf("workspace manager not available")
	}

	// Fast-path when we are already on the GTK main thread.
	if webkit.IsMainThread() {
		log.Printf("[concurrency] Already on main thread, executing split directly")
		newNode, err := wm.splitNode(req.TargetNode, req.Direction)
		if err != nil {
			log.Printf("[concurrency] Split operation failed: %v", err)
			return nil, err
		}
		log.Printf("[concurrency] Split operation completed successfully (direct): newNode=%p", newNode)
		return newNode, nil
	}

	// GTK operations must be executed on the main thread
	// Use IdleAdd to marshal the GTK work to the main thread
	gtkComplete := make(chan struct {
		newNode *paneNode
		err     error
	}, 1)

	log.Printf("[concurrency] Marshaling split operation to main thread")

	// Schedule GTK work on main thread using IdleAdd
	webkit.IdleAdd(func() bool {
		// Perform actual GTK split operations on main thread
		newNode, err := wm.splitNode(req.TargetNode, req.Direction)
		gtkComplete <- struct {
			newNode *paneNode
			err     error
		}{newNode: newNode, err: err}
		return false // Remove idle callback (G_SOURCE_REMOVE)
	})

	// Wait for GTK completion
	result := <-gtkComplete

	if result.err != nil {
		log.Printf("[concurrency] Split operation failed on main thread: %v", result.err)
		return nil, result.err
	}

	log.Printf("[concurrency] Split operation completed successfully on main thread: newNode=%p", result.newNode)
	return result.newNode, nil
}

func (worker *OperationWorker) executeCloseOperation(req *OperationRequest, tx *WidgetTransaction) (*paneNode, error) {
	log.Printf("[concurrency] Executing close operation on node %p", req.TargetNode)

	// Get the workspace manager from the controller
	wm := worker.controller.workspaceManager
	if wm == nil {
		return nil, fmt.Errorf("workspace manager not available")
	}

	// Fast-path when we are already on the GTK main thread.
	if webkit.IsMainThread() {
		log.Printf("[concurrency] Already on main thread, executing close directly")
		promoted, err := wm.closePane(req.TargetNode)
		if err != nil {
			log.Printf("[concurrency] Close operation failed: %v", err)
			return nil, err
		}
		log.Printf("[concurrency] Close operation completed successfully (direct)")
		return promoted, nil
	}

	// GTK operations must be executed on the main thread
	// Use IdleAdd to marshal the GTK work to the main thread
	gtkComplete := make(chan struct {
		promoted *paneNode
		err      error
	}, 1)

	log.Printf("[concurrency] Marshaling close operation to main thread")

	// Schedule GTK work on main thread using IdleAdd
	webkit.IdleAdd(func() bool {
		// Perform actual GTK close operations on main thread
		promoted, err := wm.closePane(req.TargetNode)
		gtkComplete <- struct {
			promoted *paneNode
			err      error
		}{promoted: promoted, err: err}
		return false // Remove idle callback (G_SOURCE_REMOVE)
	})

	// Wait for GTK completion
	result := <-gtkComplete

	if result.err != nil {
		log.Printf("[concurrency] Close operation failed on main thread: %v", result.err)
		return nil, result.err
	}

	log.Printf("[concurrency] Close operation completed successfully on main thread")
	return result.promoted, nil
}

func (worker *OperationWorker) executeStackOperation(req *OperationRequest, tx *WidgetTransaction) (*paneNode, error) {
	log.Printf("[concurrency] Executing stack operation on node %p", req.TargetNode)

	// Get the workspace manager from the controller
	wm := worker.controller.workspaceManager
	if wm == nil {
		return nil, fmt.Errorf("workspace manager not available")
	}

	// Fast-path when we are already on the GTK main thread.
	if webkit.IsMainThread() {
		log.Printf("[concurrency] Already on main thread, executing stack directly")
		newNode, err := wm.stackedPaneManager.StackPane(req.TargetNode)
		if err != nil {
			log.Printf("[concurrency] Stack operation failed: %v", err)
			return nil, err
		}
		log.Printf("[concurrency] Stack operation completed successfully (direct): newNode=%p", newNode)
		return newNode, nil
	}

	// GTK operations must be executed on the main thread
	// Use IdleAdd to marshal the GTK work to the main thread
	gtkComplete := make(chan struct {
		newNode *paneNode
		err     error
	}, 1)

	log.Printf("[concurrency] Marshaling stack operation to main thread")

	// Schedule GTK work on main thread using IdleAdd
	webkit.IdleAdd(func() bool {
		// Perform actual GTK stack operations on main thread
		newNode, err := wm.stackedPaneManager.StackPane(req.TargetNode)
		gtkComplete <- struct {
			newNode *paneNode
			err     error
		}{newNode: newNode, err: err}
		return false // Remove idle callback (G_SOURCE_REMOVE)
	})

	// Wait for GTK completion
	result := <-gtkComplete

	if result.err != nil {
		log.Printf("[concurrency] Stack operation failed on main thread: %v", result.err)
		return nil, result.err
	}

	log.Printf("[concurrency] Stack operation completed successfully on main thread: newNode=%p", result.newNode)
	return result.newNode, nil
}

func (worker *OperationWorker) executeFocusOperation(req *OperationRequest, tx *WidgetTransaction) error {
	// This would integrate with your existing focus logic
	log.Printf("[concurrency] Executing focus operation on node %p", req.TargetNode)
	// TODO: Implement actual focus logic with transaction support
	return fmt.Errorf("focus operation not yet implemented")
}

func (worker *OperationWorker) executeResizeOperation(req *OperationRequest, tx *WidgetTransaction) error {
	// This would integrate with your existing resize logic
	log.Printf("[concurrency] Executing resize operation on node %p", req.TargetNode)
	// TODO: Implement actual resize logic with transaction support
	return fmt.Errorf("resize operation not yet implemented")
}

// sendResult sends the operation result and records stats
func (worker *OperationWorker) sendResult(req *OperationRequest, result OperationResult, startTime time.Time, queueTime time.Duration) {
	result.Duration = time.Since(startTime)

	// Record stats
	stats := OperationStats{
		OperationType: req.Type,
		Duration:      result.Duration,
		Success:       result.Success,
		RetryCount:    req.RetryCount,
		QueueTime:     queueTime,
		SequenceNum:   req.SequenceNum,
		Timestamp:     time.Now(),
	}

	worker.controller.recordStats(stats)

	// Send result
	select {
	case req.ResultChan <- result:
	case <-req.Context.Done():
		// Context cancelled, result channel may be closed
	}

	log.Printf("[concurrency] Worker %d completed operation %s: success=%v, duration=%v",
		worker.id, req.ID, result.Success, result.Duration)
}

// recordStats records operation statistics
func (cc *ConcurrencyController) recordStats(stats OperationStats) {
	cc.historyMutex.Lock()
	defer cc.historyMutex.Unlock()

	if len(cc.operationHistory) >= cc.maxHistory {
		cc.operationHistory = cc.operationHistory[1:]
	}
	cc.operationHistory = append(cc.operationHistory, stats)
}

// GetConcurrencyStats returns performance statistics
func (cc *ConcurrencyController) GetConcurrencyStats() map[string]interface{} {
	cc.historyMutex.RLock()
	defer cc.historyMutex.RUnlock()

	totalOps := len(cc.operationHistory)
	if totalOps == 0 {
		return map[string]interface{}{
			"total_operations": 0,
		}
	}

	successCount := 0
	totalDuration := time.Duration(0)
	totalQueueTime := time.Duration(0)
	totalRetries := 0

	opTypeCounts := make(map[OperationType]int)

	for _, stats := range cc.operationHistory {
		if stats.Success {
			successCount++
		}
		totalDuration += stats.Duration
		totalQueueTime += stats.QueueTime
		totalRetries += stats.RetryCount
		opTypeCounts[stats.OperationType]++
	}

	return map[string]interface{}{
		"total_operations":      totalOps,
		"successful_operations": successCount,
		"success_rate":          float64(successCount) / float64(totalOps),
		"avg_duration_ms":       (totalDuration / time.Duration(totalOps)).Milliseconds(),
		"avg_queue_time_ms":     (totalQueueTime / time.Duration(totalOps)).Milliseconds(),
		"total_retries":         totalRetries,
		"operation_counts":      opTypeCounts,
		"queue_size":            len(cc.operationQueue),
		"worker_count":          cc.workerCount,
		"global_sequence":       atomic.LoadUint64(&cc.globalSequence),
	}
}

// Shutdown stops the concurrency controller
func (cc *ConcurrencyController) Shutdown() {
	cc.shutdownOnce.Do(func() {
		log.Printf("[concurrency] Shutting down concurrency controller")
		close(cc.shutdown)

		// Stop all workers
		for _, worker := range cc.workers {
			close(worker.shutdown)
		}

		// Wait for workers to finish with timeout
		done := make(chan struct{})
		go func() {
			for _, worker := range cc.workers {
				worker.wg.Wait()
			}
			close(done)
		}()

		select {
		case <-done:
			log.Printf("[concurrency] All workers stopped gracefully")
		case <-time.After(5 * time.Second):
			log.Printf("[concurrency] Shutdown timeout, some workers may not have stopped gracefully")
		}
	})
}

// Helper functions

// isAncestor checks if node1 is an ancestor of node2
func isAncestor(node1, node2 *paneNode) bool {
	if node1 == nil || node2 == nil {
		return false
	}

	current := node2.parent
	for current != nil {
		if current == node1 {
			return true
		}
		current = current.parent
	}
	return false
}
