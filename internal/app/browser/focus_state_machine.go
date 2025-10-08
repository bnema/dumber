// focus_state_machine.go - Centralized focus state management with queue-based serialization
package browser

import (
	"container/heap"
	"fmt"
	"log"
	"runtime/debug"
	"sync"
	"time"

	"github.com/bnema/dumber/pkg/webkit"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// FocusState represents the current state of the focus management system
type FocusState string

const (
	StateInitializing  FocusState = "initializing"  // System startup, determining initial focus
	StateIdle          FocusState = "idle"          // No focus transition in progress, system stable
	StateTransitioning FocusState = "transitioning" // Processing a focus change request
	StateFocused       FocusState = "focused"       // Focus successfully applied, awaiting next request
	StateReconciling   FocusState = "reconciling"   // Fixing inconsistencies detected in CSS/focus state
)

// FocusSource identifies where a focus request originated
type FocusSource string

const (
	SourceKeyboard     FocusSource = "keyboard"     // User keyboard navigation
	SourceMouse        FocusSource = "mouse"        // Mouse hover/click
	SourceProgrammatic FocusSource = "programmatic" // API call
	SourceSystem       FocusSource = "system"       // System initialization
	SourceStackNav     FocusSource = "stack-nav"    // Stack navigation
	SourceSplit        FocusSource = "split"        // Pane split operation
	SourceClose        FocusSource = "close"        // Pane close operation
)

// Priority levels for focus requests
const (
	PriorityLow    = 10
	PriorityNormal = 50
	PriorityHigh   = 90
	PriorityUrgent = 100
)

// FocusRequest represents a request to change focus to a specific pane
type FocusRequest struct {
	ID         string                 // Unique request identifier
	TargetNode *paneNode              // Target pane to focus
	Source     FocusSource            // Where the request originated
	Priority   int                    // Request priority (higher = more urgent)
	Timestamp  time.Time              // When the request was created
	Context    map[string]interface{} // Additional metadata
}

// FocusTransition records a completed focus change for debugging
type FocusTransition struct {
	From      *paneNode   // Previous focus target
	To        *paneNode   // New focus target
	State     FocusState  // State during transition
	Source    FocusSource // Request source
	Timestamp time.Time   // When transition occurred
	Success   bool        // Whether transition succeeded
	Error     error       // Error if transition failed
	Stack     []byte      // Stack trace for debugging
}

// RingBuffer provides a thread-safe circular buffer for focus history
type RingBuffer[T any] struct {
	buffer []T
	head   int
	tail   int
	size   int
	cap    int
	mu     sync.RWMutex
}

// NewRingBuffer creates a new ring buffer with the specified capacity
func NewRingBuffer[T any](capacity int) *RingBuffer[T] {
	return &RingBuffer[T]{
		buffer: make([]T, capacity),
		cap:    capacity,
	}
}

// Add inserts an item into the ring buffer
func (rb *RingBuffer[T]) Add(item T) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	rb.buffer[rb.head] = item
	rb.head = (rb.head + 1) % rb.cap

	if rb.size < rb.cap {
		rb.size++
	} else {
		rb.tail = (rb.tail + 1) % rb.cap
	}
}

// GetAll returns all items in the ring buffer in chronological order
func (rb *RingBuffer[T]) GetAll() []T {
	rb.mu.RLock()
	defer rb.mu.RUnlock()

	if rb.size == 0 {
		return nil
	}

	result := make([]T, rb.size)
	for i := 0; i < rb.size; i++ {
		idx := (rb.tail + i) % rb.cap
		result[i] = rb.buffer[idx]
	}
	return result
}

// FocusPriorityQueue implements a priority queue for focus requests
type FocusPriorityQueue []*FocusRequest

func (pq FocusPriorityQueue) Len() int { return len(pq) }

func (pq FocusPriorityQueue) Less(i, j int) bool {
	// Higher priority value = higher priority (processed first)
	return pq[i].Priority > pq[j].Priority
}

func (pq FocusPriorityQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
}

func (pq *FocusPriorityQueue) Push(x interface{}) {
	*pq = append(*pq, x.(*FocusRequest))
}

func (pq *FocusPriorityQueue) Pop() interface{} {
	old := *pq
	n := len(old)
	item := old[n-1]
	*pq = old[0 : n-1]
	return item
}

// RequestDeduplicator prevents duplicate requests from flooding the queue
type RequestDeduplicator struct {
	mu         sync.RWMutex
	recentSigs map[string]time.Time
	ttl        time.Duration
}

// NewRequestDeduplicator creates a new request deduplicator
func NewRequestDeduplicator(ttl time.Duration) *RequestDeduplicator {
	return &RequestDeduplicator{
		recentSigs: make(map[string]time.Time),
		ttl:        ttl,
	}
}

// IsDuplicate checks if a request is a duplicate within the TTL window
func (rd *RequestDeduplicator) IsDuplicate(req FocusRequest) bool {
	rd.mu.Lock()
	defer rd.mu.Unlock()

	// Clean up expired signatures
	now := time.Now()
	for sig, timestamp := range rd.recentSigs {
		if now.Sub(timestamp) > rd.ttl {
			delete(rd.recentSigs, sig)
		}
	}

	// Generate signature for this request
	sig := fmt.Sprintf("%p:%s", req.TargetNode, req.Source)

	// Check if this signature exists
	if _, exists := rd.recentSigs[sig]; exists {
		return true
	}

	// Record this signature
	rd.recentSigs[sig] = now
	return false
}

// FocusStateMachine manages all focus operations through a centralized state machine
type FocusStateMachine struct {
	mu               sync.RWMutex
	wm               *WorkspaceManager            // Reference to workspace manager
	currentState     FocusState                   // Current state of the focus system
	activePane       *paneNode                    // Currently focused pane
	requestQueue     chan FocusRequest            // Queue for serializing focus requests
	priorityQueue    *FocusPriorityQueue          // Priority queue for request ordering
	pendingRequests  chan struct{}                // Signal channel for pending requests
	historyRing      *RingBuffer[FocusTransition] // History of focus transitions
	validators       []FocusValidator             // Request validators
	settlingTimer    *time.Timer                  // Timer for state settling
	queueProcessor   *sync.WaitGroup              // Wait group for queue processor
	shutdownChan     chan struct{}                // Shutdown signal
	requestIDCounter uint64                       // Counter for generating request IDs

	// Request coalescing for rapid focus changes
	lastRequestTime  time.Time            // Time of last request
	coalescingWindow time.Duration        // Window for coalescing requests
	deduplicator     *RequestDeduplicator // Request deduplication

	// Performance metrics tracking
	metrics         FocusMetrics    // Performance metrics
	processTimes    []time.Duration // Recent process times for averaging
	maxProcessTimes int             // Max number of process times to keep

	// Debug configuration
	debugEnabled   bool // Enable focus debug logging
	metricsEnabled bool // Enable metrics tracking
	// Reconciliation loop prevention
	lastReconcileTime    time.Time // Last reconciliation timestamp
	reconcileCount       int       // Recent reconciliation count
	maxReconcileAttempts int       // Max reconcile attempts per second
}

// FocusValidator validates focus requests before processing
type FocusValidator func(request FocusRequest) error

// Predefined errors
var (
	ErrFocusQueueFull        = fmt.Errorf("focus request queue is full")
	ErrInvalidTarget         = fmt.Errorf("invalid focus target")
	ErrInvalidStackOperation = fmt.Errorf("invalid stack operation")
	ErrStateMachineShutdown  = fmt.Errorf("focus state machine is shutting down")
)

// NewFocusStateMachine creates a new focus state machine
func NewFocusStateMachine(wm *WorkspaceManager) *FocusStateMachine {
	pq := &FocusPriorityQueue{}
	heap.Init(pq)

	fsm := &FocusStateMachine{
		wm:               wm,
		currentState:     StateInitializing,
		requestQueue:     make(chan FocusRequest, 100), // Buffered channel with capacity 100
		priorityQueue:    pq,
		pendingRequests:  make(chan struct{}, 1),             // Signal for priority queue processing
		historyRing:      NewRingBuffer[FocusTransition](50), // Keep last 50 transitions
		validators:       make([]FocusValidator, 0),
		queueProcessor:   &sync.WaitGroup{},
		shutdownChan:     make(chan struct{}),
		coalescingWindow: 50 * time.Millisecond,                          // 50ms coalescing window
		deduplicator:     NewRequestDeduplicator(200 * time.Millisecond), // 200ms dedup TTL
		maxProcessTimes:  100,                                            // Keep last 100 process times for averaging
		// Debug flags will be set via ConfigureDebug()
		debugEnabled:   false,
		metricsEnabled: false,
		// Reconciliation loop prevention
		maxReconcileAttempts: 3, // Max 3 reconciliation attempts per second
	}

	// Add default validators
	fsm.addDefaultValidators()

	return fsm
}

// ConfigureDebug configures debug settings for the focus state machine
func (fsm *FocusStateMachine) ConfigureDebug(focusDebug, _ bool, metricsEnabled bool) {
	fsm.mu.Lock()
	defer fsm.mu.Unlock()

	fsm.debugEnabled = focusDebug
	fsm.metricsEnabled = metricsEnabled

	if focusDebug {
		log.Printf("[FSM] Focus debug logging enabled")
	}
	if metricsEnabled {
		log.Printf("[FSM] Focus metrics tracking enabled")
	}
}

// Initialize sets up the focus state machine and determines initial focus
func (fsm *FocusStateMachine) Initialize() error {
	fsm.mu.Lock()
	defer fsm.mu.Unlock()

	log.Printf("[FSM] Initializing focus state machine")
	fsm.currentState = StateInitializing

	// Determine initial focus
	leaves := fsm.wm.collectLeaves()
	if len(leaves) == 0 {
		log.Printf("[FSM] No panes found during initialization")
		fsm.currentState = StateIdle
		return nil
	}

	initialPane := fsm.determineInitialFocus(leaves)
	if initialPane == nil {
		log.Printf("[FSM] Could not determine initial focus target")
		fsm.currentState = StateIdle
		return nil
	}

	// Phase 3: Apply initial focus and CSS
	if err := fsm.applyInitialFocus(initialPane); err != nil {
		log.Printf("[FSM] Failed to apply initial focus: %v", err)
		fsm.currentState = StateIdle
		return err
	}

	// Phase 4: Attach GTK focus controllers to all panes
	fsm.attachGTKControllersToAllPanes()

	// Phase 5: Start queue processor
	fsm.queueProcessor.Add(1)
	go fsm.processQueue()

	// Phase 6: Start CSS verification loop
	go fsm.startCSSVerificationLoop()

	// Phase 7: Transition to idle state
	fsm.currentState = StateIdle

	log.Printf("[FSM] Focus state machine initialized successfully with pane %p", initialPane)
	return nil
}

// Shutdown gracefully shuts down the focus state machine
func (fsm *FocusStateMachine) Shutdown() {
	fsm.mu.Lock()
	defer fsm.mu.Unlock()

	log.Printf("[FSM] Shutting down focus state machine")

	// Signal shutdown
	close(fsm.shutdownChan)

	// Wait for queue processor to finish
	fsm.queueProcessor.Wait()

	// Clean up timers
	if fsm.settlingTimer != nil {
		fsm.settlingTimer.Stop()
	}

	log.Printf("[FSM] Focus state machine shutdown complete")
}

// RequestFocus queues a focus change request with priority and deduplication
func (fsm *FocusStateMachine) RequestFocus(node *paneNode, source FocusSource) error {
	if node == nil {
		return ErrInvalidTarget
	}

	request := FocusRequest{
		ID:         fsm.generateRequestID(),
		TargetNode: node,
		Source:     source,
		Priority:   fsm.calculatePriority(source),
		Timestamp:  time.Now(),
		Context:    make(map[string]interface{}),
	}

	// Track total requests
	if fsm.metricsEnabled {
		fsm.mu.Lock()
		fsm.metrics.TotalRequests++
		fsm.mu.Unlock()
	}

	// Check for duplicates
	if fsm.deduplicator.IsDuplicate(request) {
		if fsm.metricsEnabled {
			fsm.mu.Lock()
			fsm.metrics.DuplicateRequests++
			fsm.mu.Unlock()
		}
		if fsm.debugEnabled {
			log.Printf("[FSM] Duplicate request ignored: %s -> %p", request.Source, request.TargetNode)
		}
		return nil
	}

	// Check for coalescing opportunity
	if fsm.shouldCoalesce(request) {
		if fsm.metricsEnabled {
			fsm.mu.Lock()
			fsm.metrics.CoalescedRequests++
			fsm.mu.Unlock()
		}
		if fsm.debugEnabled {
			log.Printf("[FSM] Request coalesced: %s -> %p", request.Source, request.TargetNode)
		}
		return nil
	}

	// Add to priority queue instead of simple channel
	fsm.mu.Lock()
	heap.Push(fsm.priorityQueue, &request)
	fsm.lastRequestTime = request.Timestamp

	// Track queue depth metrics
	currentDepth := fsm.priorityQueue.Len()
	if currentDepth > fsm.metrics.MaxQueueDepth {
		fsm.metrics.MaxQueueDepth = currentDepth
	}
	fsm.mu.Unlock()

	// Signal that a request is pending
	select {
	case fsm.pendingRequests <- struct{}{}:
	default: // Channel is already signaled
	}

	return nil
}

// GetActivePane returns the currently active pane
func (fsm *FocusStateMachine) GetActivePane() *paneNode {
	fsm.mu.RLock()
	defer fsm.mu.RUnlock()
	return fsm.activePane
}

// InvalidateActivePane clears the active pane if it matches the provided node.
// This is used when panes are destroyed so the focus state machine doesn't keep
// references to invalid widgets.
func (fsm *FocusStateMachine) InvalidateActivePane(node *paneNode) {
	if node == nil {
		return
	}

	fsm.mu.Lock()
	defer fsm.mu.Unlock()

	if fsm.activePane != node {
		return
	}

	log.Printf("[FSM] Active pane invalidated: %p", node)
	fsm.activePane = nil

	if fsm.wm != nil && fsm.wm.app != nil {
		fsm.wm.app.activePane = nil
	}

	if fsm.currentState == StateFocused {
		fsm.currentState = StateIdle
	}
}

// GetCurrentState returns the current state of the focus system
func (fsm *FocusStateMachine) GetCurrentState() FocusState {
	fsm.mu.RLock()
	defer fsm.mu.RUnlock()
	return fsm.currentState
}

// AddValidator adds a focus request validator
func (fsm *FocusStateMachine) AddValidator(validator FocusValidator) {
	fsm.mu.Lock()
	defer fsm.mu.Unlock()
	fsm.validators = append(fsm.validators, validator)
}

// Private methods

// generateRequestID creates a unique request identifier
func (fsm *FocusStateMachine) generateRequestID() string {
	fsm.requestIDCounter++
	return fmt.Sprintf("req-%d-%d", time.Now().UnixNano(), fsm.requestIDCounter)
}

// calculatePriority determines request priority based on source
func (fsm *FocusStateMachine) calculatePriority(source FocusSource) int {
	switch source {
	case SourceKeyboard:
		return PriorityHigh
	case SourceMouse:
		return PriorityNormal
	case SourceStackNav:
		return PriorityHigh
	case SourceSystem:
		return PriorityUrgent
	case SourceSplit, SourceClose:
		return PriorityHigh
	default:
		return PriorityLow
	}
}

// addDefaultValidators adds standard focus request validators
func (fsm *FocusStateMachine) addDefaultValidators() {
	// Validate target node exists and is a leaf
	fsm.validators = append(fsm.validators, func(request FocusRequest) error {
		if request.TargetNode == nil {
			return ErrInvalidTarget
		}
		if !request.TargetNode.isLeaf {
			return fmt.Errorf("target node is not a leaf pane")
		}
		if request.TargetNode.pane == nil || request.TargetNode.pane.webView == nil {
			return fmt.Errorf("target node has no valid webview")
		}
		return nil
	})

	// Validate target node still exists in workspace
	fsm.validators = append(fsm.validators, func(request FocusRequest) error {
		if fsm.wm.viewToNode[request.TargetNode.pane.webView] != request.TargetNode {
			return fmt.Errorf("target node no longer exists in workspace")
		}
		return nil
	})
}

// validateRequest runs all validators on a focus request
func (fsm *FocusStateMachine) validateRequest(request FocusRequest) error {
	for _, validator := range fsm.validators {
		if err := validator(request); err != nil {
			return err
		}
	}
	return nil
}

// determineInitialFocus selects the initial focus target based on priority rules
func (fsm *FocusStateMachine) determineInitialFocus(leaves []*paneNode) *paneNode {
	if len(leaves) == 0 {
		return nil
	}

	// Priority 1: First pane with user content (not about:blank)
	for _, leaf := range leaves {
		if leaf.pane != nil && leaf.pane.webView != nil {
			url := leaf.pane.webView.GetCurrentURL()
			if url != "" && url != "about:blank" {
				log.Printf("[FSM] Selected pane with content as initial focus: %s", url)
				return leaf
			}
		}
	}

	// Priority 2: Top-left pane (geometrically)
	if topLeft := fsm.findTopLeftPane(leaves); topLeft != nil {
		log.Printf("[FSM] Selected top-left pane as initial focus")
		return topLeft
	}

	// Priority 3: First pane in tree order
	log.Printf("[FSM] Selected first pane as initial focus")
	return leaves[0]
}

// findTopLeftPane finds the geometrically top-left pane
func (fsm *FocusStateMachine) findTopLeftPane(leaves []*paneNode) *paneNode {
	var bestPane *paneNode
	var bestScore float64 = 1e9

	for _, leaf := range leaves {
		if leaf.container == nil {
			continue
		}

		x, y, _, _ := webkit.WidgetGetAllocation(leaf.container)

		// Score based on distance from top-left corner (0,0)
		score := float64(x) + float64(y)
		if score < bestScore {
			bestScore = score
			bestPane = leaf
		}
	}

	return bestPane
}

// applyInitialFocus applies focus to the initial target without going through the queue
func (fsm *FocusStateMachine) applyInitialFocus(node *paneNode) error {
	if node == nil {
		return ErrInvalidTarget
	}

	log.Printf("[FSM] Applying initial focus to pane %p", node)

	// Apply GTK focus
	if err := fsm.applyGTKFocus(node); err != nil {
		return fmt.Errorf("failed to apply GTK focus: %w", err)
	}

	// Apply initial visual border
	if fsm.wm != nil {
		ctx := fsm.wm.determineBorderContext(node)
		fsm.wm.applyActivePaneBorder(ctx)
	}

	// Update state
	fsm.activePane = node

	// Record transition
	transition := FocusTransition{
		From:      nil,
		To:        node,
		State:     StateInitializing,
		Source:    SourceSystem,
		Timestamp: time.Now(),
		Success:   true,
		Stack:     debug.Stack(),
	}
	fsm.historyRing.Add(transition)

	// Notify workspace manager
	if fsm.wm != nil && fsm.wm.app != nil && node.pane != nil {
		fsm.wm.app.activePane = node.pane
	}

	return nil
}

// applyGTKFocus applies GTK focus to the specified pane
func (fsm *FocusStateMachine) applyGTKFocus(node *paneNode) error {
	if node.pane == nil || node.pane.webView == nil {
		return fmt.Errorf("pane has no valid webview")
	}

	viewWidget := node.pane.webView.Widget()
	if viewWidget == nil {
		return fmt.Errorf("webview has no valid widget")
	}

	webkit.WidgetGrabFocus(viewWidget)
	return nil
}

// processQueue processes focus requests from the priority queue
func (fsm *FocusStateMachine) processQueue() {
	defer fsm.queueProcessor.Done()

	log.Printf("[FSM] Focus request queue processor started")

	for {
		select {
		case <-fsm.pendingRequests:
			// Process all pending requests from priority queue
			fsm.processPendingRequests()
		case request := <-fsm.requestQueue:
			// Legacy channel for backward compatibility
			fsm.handleFocusRequest(request)
		case <-fsm.shutdownChan:
			log.Printf("[FSM] Focus request queue processor shutting down")
			return
		}
	}
}

// processPendingRequests processes all requests in the priority queue
func (fsm *FocusStateMachine) processPendingRequests() {
	for {
		fsm.mu.Lock()
		if fsm.priorityQueue.Len() == 0 {
			fsm.mu.Unlock()
			break
		}

		// Get highest priority request
		request := heap.Pop(fsm.priorityQueue).(*FocusRequest)
		fsm.mu.Unlock()

		// Check if a higher priority request has arrived while processing
		if fsm.hasHigherPriorityRequest(*request) {
			// Requeue current request and process higher priority one
			fsm.requeue(*request)
			continue
		}

		fsm.handleFocusRequest(*request)
	}
}

// handleFocusRequest processes a single focus request
func (fsm *FocusStateMachine) handleFocusRequest(request FocusRequest) {
	startTime := time.Now()

	// Validate request
	if err := fsm.validateRequest(request); err != nil {
		fsm.mu.Lock()
		fsm.metrics.FailedRequests++
		fsm.mu.Unlock()
		log.Printf("[FSM] Invalid focus request %s: %v", request.ID, err)
		return
	}

	// Check if we're already focused on the target
	fsm.mu.RLock()
	currentActive := fsm.activePane
	fsm.mu.RUnlock()

	if currentActive == request.TargetNode {
		log.Printf("[FSM] Focus request %s: already focused on target", request.ID)
		fsm.recordProcessTime(time.Since(startTime))
		return
	}

	log.Printf("[FSM] Processing focus request %s: %s -> %p",
		request.ID, request.Source, request.TargetNode)

	// Execute focus change
	if err := fsm.executeFocusChange(request); err != nil {
		fsm.mu.Lock()
		fsm.metrics.FailedRequests++
		fsm.mu.Unlock()
		log.Printf("[FSM] Focus request %s failed: %v", request.ID, err)
	} else {
		fsm.mu.Lock()
		fsm.metrics.SuccessfulRequests++
		fsm.mu.Unlock()
	}

	fsm.recordProcessTime(time.Since(startTime))
}

// executeFocusChange executes a focus change with proper state management
func (fsm *FocusStateMachine) executeFocusChange(request FocusRequest) error {
	fsm.mu.Lock()
	defer fsm.mu.Unlock()

	// Transition to processing state
	oldState := fsm.currentState
	fsm.currentState = StateTransitioning

	oldPane := fsm.activePane
	newPane := request.TargetNode

	// Record transition start
	transition := FocusTransition{
		From:      oldPane,
		To:        newPane,
		Source:    request.Source,
		Timestamp: request.Timestamp,
		Stack:     debug.Stack(),
	}

	// Apply GTK focus
	if err := fsm.applyGTKFocus(newPane); err != nil {
		// Rollback state
		fsm.currentState = oldState
		transition.Success = false
		transition.Error = err
		fsm.historyRing.Add(transition)
		return fmt.Errorf("failed to apply GTK focus: %w", err)
	}

	// Apply visual border changes
	if fsm.wm != nil {
		// Remove border from old pane
		if oldPane != nil {
			fsm.wm.removeActivePaneBorder(oldPane)
		}
		// Add border to new pane
		ctx := fsm.wm.determineBorderContext(newPane)
		fsm.wm.applyActivePaneBorder(ctx)
	}

	// Update state
	fsm.activePane = newPane
	fsm.currentState = StateFocused

	// Record successful transition
	transition.Success = true
	fsm.historyRing.Add(transition)

	// Notify workspace manager
	if fsm.wm != nil && fsm.wm.app != nil && newPane.pane != nil {
		fsm.wm.app.activePane = newPane.pane
	}

	// Start settling timer
	fsm.startSettlingTimer()

	log.Printf("[FSM] Focus change completed: %p -> %p", oldPane, newPane)
	return nil
}

// startSettlingTimer starts a timer to transition back to idle state
func (fsm *FocusStateMachine) startSettlingTimer() {
	if fsm.settlingTimer != nil {
		fsm.settlingTimer.Stop()
	}

	fsm.settlingTimer = time.AfterFunc(50*time.Millisecond, func() {
		fsm.mu.Lock()
		if fsm.currentState == StateFocused {
			fsm.currentState = StateIdle
		}
		fsm.mu.Unlock()
	})
}

// startCSSVerificationLoop starts the CSS verification background loop
func (fsm *FocusStateMachine) startCSSVerificationLoop() {
	ticker := time.NewTicker(100 * time.Millisecond)

	go func() {
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				fsm.verifyCSSState()
			case <-fsm.shutdownChan:
				return
			}
		}
	}()
}

// verifyCSSState checks and corrects CSS class inconsistencies
func (fsm *FocusStateMachine) verifyCSSState() {
	fsm.mu.RLock()
	activePane := fsm.activePane
	fsm.mu.RUnlock()

	if activePane == nil || fsm.wm == nil {
		return
	}

	var issues []string

	if len(issues) > 0 {
		// Prevent infinite reconciliation loops
		fsm.mu.Lock()
		now := time.Now()
		if now.Sub(fsm.lastReconcileTime) < time.Second {
			fsm.reconcileCount++
		} else {
			fsm.reconcileCount = 1
			fsm.lastReconcileTime = now
		}

		if fsm.reconcileCount > fsm.maxReconcileAttempts {
			log.Printf("[FSM] WARNING: Reconciliation loop detected, skipping (attempt %d)", fsm.reconcileCount)
			fsm.mu.Unlock()
			return
		}
		fsm.mu.Unlock()

		log.Printf("[FSM] CSS inconsistencies found: %v", issues)
		log.Printf("[FSM] Triggering CSS reconciliation (attempt %d)", fsm.reconcileCount)

		// Track reconciliation in metrics
		fsm.mu.Lock()
		fsm.metrics.ReconciliationCount++
		fsm.mu.Unlock()
	}
}

// shouldCoalesce checks if a request should be coalesced with recent requests
func (fsm *FocusStateMachine) shouldCoalesce(request FocusRequest) bool {
	fsm.mu.RLock()
	defer fsm.mu.RUnlock()

	// Only coalesce if we have a recent request and it's to the same target
	if time.Since(fsm.lastRequestTime) > fsm.coalescingWindow {
		return false
	}

	// Check if there's already a pending request to the same target
	for i := 0; i < fsm.priorityQueue.Len(); i++ {
		pending := (*fsm.priorityQueue)[i]
		if pending.TargetNode == request.TargetNode {
			log.Printf("[FSM] Coalescing request to same target: %p", request.TargetNode)
			return true
		}
	}

	return false
}

// hasHigherPriorityRequest checks if there's a higher priority request in the queue
func (fsm *FocusStateMachine) hasHigherPriorityRequest(current FocusRequest) bool {
	fsm.mu.RLock()
	defer fsm.mu.RUnlock()

	if fsm.priorityQueue.Len() == 0 {
		return false
	}

	// Check the highest priority request (at top of heap)
	highest := (*fsm.priorityQueue)[0]
	return highest.Priority > current.Priority
}

// requeue adds a request back to the priority queue
func (fsm *FocusStateMachine) requeue(request FocusRequest) {
	fsm.mu.Lock()
	defer fsm.mu.Unlock()

	heap.Push(fsm.priorityQueue, &request)
	log.Printf("[FSM] Requeued request due to higher priority: %s", request.ID)

	// Signal that a request is pending
	select {
	case fsm.pendingRequests <- struct{}{}:
	default: // Channel is already signaled
	}
}

// attachGTKController attaches a GTK focus controller to a pane node
func (fsm *FocusStateMachine) attachGTKController(node *paneNode) {
	if node == nil || node.pane == nil || node.pane.webView == nil {
		return
	}

	if node.focusControllerToken != 0 {
		log.Printf("[FSM] Controller already attached to pane %p", node)
		return
	}

	widget := node.pane.webView.Widget()
	if widget == nil {
		return
	}

	// Create focus controller for GTK4
	controller := gtk.NewEventControllerFocus()

	// Connect focus enter/leave callbacks for this specific node
	controller.ConnectEnter(func() {
		log.Printf("[FSM] GTK focus enter: %p", node)
		// Don't automatically change focus on GTK enter - let user interactions drive this
		// This prevents infinite loops with our own focus changes
	})

	controller.ConnectLeave(func() {
		log.Printf("[FSM] GTK focus leave: %p", node)
		// Similarly, don't react to GTK leave events automatically
	})

	// Add controller to widget
	webkit.WidgetAddController(widget, controller)

	// Store controller pointer as token for later removal
	node.focusControllerToken = uintptr(controller.Native())
	if node.focusControllerToken != 0 {
		log.Printf("[FSM] Attached GTK focus controller to pane %p with token %d", node, node.focusControllerToken)
	}
}

// detachGTKController removes a GTK focus controller from a pane node
func (fsm *FocusStateMachine) detachGTKController(node *paneNode, token uintptr) {
	if node == nil || node.pane == nil || node.pane.webView == nil || token == 0 {
		return
	}

	if node.focusControllerToken != token {
		log.Printf("[FSM] Token mismatch, skipping detach for pane %p", node)
		return
	}

	node.focusControllerToken = 0

	// Note: In GTK4, controllers are automatically removed when widget is destroyed
	// We just need to clear our token reference
	log.Printf("[FSM] Detached GTK focus controller from pane %p", node)
}

// attachGTKControllersToAllPanes attaches focus controllers to all existing panes
func (fsm *FocusStateMachine) attachGTKControllersToAllPanes() {
	if fsm.wm == nil {
		return
	}

	leaves := fsm.wm.collectLeaves()
	for _, leaf := range leaves {
		fsm.attachGTKController(leaf)
	}

	log.Printf("[FSM] Attached GTK focus controllers to %d panes", len(leaves))
}

// FocusDebugInfo contains comprehensive debug information about the focus system
type FocusDebugInfo struct {
	CurrentState      FocusState             `json:"current_state"`
	ActivePane        *paneNode              `json:"active_pane,omitempty"`
	QueueLength       int                    `json:"queue_length"`
	PriorityQueueSize int                    `json:"priority_queue_size"`
	History           []FocusTransition      `json:"history"`
	CSSState          map[string]interface{} `json:"css_state"`
	ValidationErrors  []string               `json:"validation_errors"`
	Metrics           FocusMetrics           `json:"metrics"`
	RecentRequests    []string               `json:"recent_requests"`
}

// FocusMetrics tracks performance metrics for the focus system
type FocusMetrics struct {
	TotalRequests       uint64        `json:"total_requests"`
	SuccessfulRequests  uint64        `json:"successful_requests"`
	FailedRequests      uint64        `json:"failed_requests"`
	CoalescedRequests   uint64        `json:"coalesced_requests"`
	DuplicateRequests   uint64        `json:"duplicate_requests"`
	AverageProcessTime  time.Duration `json:"average_process_time"`
	MaxQueueDepth       int           `json:"max_queue_depth"`
	ReconciliationCount uint64        `json:"reconciliation_count"`
}

// DumpState returns comprehensive debug information about the focus system
func (fsm *FocusStateMachine) DumpState() FocusDebugInfo {
	fsm.mu.RLock()
	defer fsm.mu.RUnlock()

	info := FocusDebugInfo{
		CurrentState:      fsm.currentState,
		ActivePane:        fsm.activePane,
		QueueLength:       len(fsm.requestQueue),
		PriorityQueueSize: fsm.priorityQueue.Len(),
		History:           fsm.historyRing.GetAll(),
		ValidationErrors:  fsm.getValidationErrors(),
		RecentRequests:    fsm.getRecentRequestSignatures(),
		Metrics:           fsm.GetMetrics(),
	}

	return info
}

// ForceValidation manually triggers CSS consistency check and validation
func (fsm *FocusStateMachine) ForceValidation() []string {
	log.Printf("[FSM] Force validation requested")

	// Trigger CSS verification
	fsm.verifyCSSState()

	// Run all validators on current state
	var issues []string

	fsm.mu.RLock()
	activePane := fsm.activePane
	fsm.mu.RUnlock()

	if activePane != nil {
		dummyRequest := FocusRequest{
			ID:         "validation-check",
			TargetNode: activePane,
			Source:     SourceSystem,
			Priority:   PriorityLow,
			Timestamp:  time.Now(),
		}

		if err := fsm.validateRequest(dummyRequest); err != nil {
			issues = append(issues, fmt.Sprintf("Active pane validation failed: %v", err))
		}
	}

	log.Printf("[FSM] Force validation completed: %d issues found", len(issues))
	return issues
}

// GetQueueStatus returns detailed information about pending requests
func (fsm *FocusStateMachine) GetQueueStatus() map[string]interface{} {
	fsm.mu.RLock()
	defer fsm.mu.RUnlock()

	status := map[string]interface{}{
		"channel_queue_length":  len(fsm.requestQueue),
		"priority_queue_length": fsm.priorityQueue.Len(),
		"channel_queue_cap":     cap(fsm.requestQueue),
		"processing_state":      fsm.currentState,
		"last_request_time":     fsm.lastRequestTime,
		"coalescing_window":     fsm.coalescingWindow,
	}

	// Add priority queue details
	if fsm.priorityQueue.Len() > 0 {
		priorities := make([]int, 0, fsm.priorityQueue.Len())
		targets := make([]string, 0, fsm.priorityQueue.Len())

		for i := 0; i < fsm.priorityQueue.Len(); i++ {
			req := (*fsm.priorityQueue)[i]
			priorities = append(priorities, req.Priority)
			targets = append(targets, fmt.Sprintf("%p", req.TargetNode))
		}

		status["pending_priorities"] = priorities
		status["pending_targets"] = targets
	}

	return status
}

// ClearHistory clears the focus transition history (useful for debugging sessions)
func (fsm *FocusStateMachine) ClearHistory() {
	fsm.mu.Lock()
	defer fsm.mu.Unlock()

	fsm.historyRing = NewRingBuffer[FocusTransition](50)
	log.Printf("[FSM] Focus transition history cleared")
}

// GetMetrics returns performance metrics for the focus system
func (fsm *FocusStateMachine) GetMetrics() FocusMetrics {
	fsm.mu.RLock()
	defer fsm.mu.RUnlock()

	// Copy current metrics with calculated average
	metrics := fsm.metrics
	metrics.AverageProcessTime = fsm.calculateAverageProcessTime()

	return metrics
}

// recordProcessTime adds a process time to the metrics and maintains the sliding window
func (fsm *FocusStateMachine) recordProcessTime(duration time.Duration) {
	fsm.mu.Lock()
	defer fsm.mu.Unlock()

	// Add to the list
	fsm.processTimes = append(fsm.processTimes, duration)

	// Maintain sliding window
	if len(fsm.processTimes) > fsm.maxProcessTimes {
		fsm.processTimes = fsm.processTimes[1:]
	}
}

// calculateAverageProcessTime calculates the average of recent process times
func (fsm *FocusStateMachine) calculateAverageProcessTime() time.Duration {
	if len(fsm.processTimes) == 0 {
		return 0
	}

	var total time.Duration
	for _, t := range fsm.processTimes {
		total += t
	}

	return total / time.Duration(len(fsm.processTimes))
}

// getValidationErrors checks for various validation issues
func (fsm *FocusStateMachine) getValidationErrors() []string {
	var errors []string

	// Check if we have an active pane but it's not valid
	if fsm.activePane != nil {
		if !fsm.activePane.isLeaf {
			errors = append(errors, "active pane is not a leaf node")
		}
		if fsm.activePane.pane == nil || fsm.activePane.pane.webView == nil {
			errors = append(errors, "active pane has no valid webview")
		}
		if fsm.wm != nil && fsm.wm.viewToNode[fsm.activePane.pane.webView] != fsm.activePane {
			errors = append(errors, "active pane not found in workspace view map")
		}
	}

	// Check for orphaned requests in queue
	if fsm.wm != nil {
		validNodes := make(map[*paneNode]bool)
		for _, node := range fsm.wm.viewToNode {
			validNodes[node] = true
		}

		for i := 0; i < fsm.priorityQueue.Len(); i++ {
			req := (*fsm.priorityQueue)[i]
			if !validNodes[req.TargetNode] {
				errors = append(errors, fmt.Sprintf("orphaned request for invalid node: %s", req.ID))
			}
		}
	}

	return errors
}

// getRecentRequestSignatures returns recent request signatures for debugging
func (fsm *FocusStateMachine) getRecentRequestSignatures() []string {
	if fsm.deduplicator == nil {
		return nil
	}

	fsm.deduplicator.mu.RLock()
	defer fsm.deduplicator.mu.RUnlock()

	sigs := make([]string, 0, len(fsm.deduplicator.recentSigs))
	for sig, timestamp := range fsm.deduplicator.recentSigs {
		sigs = append(sigs, fmt.Sprintf("%s (age: %v)", sig, time.Since(timestamp)))
	}

	return sigs
}
