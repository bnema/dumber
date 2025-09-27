Detailed Implementation Plan: State Machine with Focus Queue

  Architecture Overview

  The focus management system will be refactored into a centralized state machine that serializes all
  focus operations through a queue, ensuring deterministic behavior and eliminating race conditions.

  Core Components

  1. Focus State Machine (focus_state_machine.go)

  States:
  - StateInitializing: System startup, determining initial focus
  - StateIdle: No focus transition in progress, system stable
  - StateTransitioning: Processing a focus change request
  - StateFocused: Focus successfully applied, awaiting next request
  - StateReconciling: Fixing inconsistencies detected in CSS/focus state

  State Transitions:
  Initializing → Idle (on workspace ready)
  Idle → Transitioning (on focus request)
  Transitioning → Focused (on success)
  Transitioning → StateReconciling (on conflict)
  Focused → Idle (after settling time)
  StateReconciling → Focused (after fix)

  Data Structures:
  type FocusState string

  type FocusRequest struct {
      ID          string
      TargetNode  *paneNode
      Source      FocusSource  // Keyboard, Mouse, Programmatic, System
      Priority    int          // Higher priority preempts queue
      Timestamp   time.Time
      Context     map[string]interface{} // Additional metadata
  }

  type FocusStateMachine struct {
      mu              sync.RWMutex
      currentState    FocusState
      activePane      *paneNode
      requestQueue    chan FocusRequest
      historyRing     *RingBuffer[FocusTransition]
      cssReconciler   *CSSReconciler
      gtkController   *GTKFocusController
      validators      []FocusValidator
      settlingTimer   *time.Timer
  }

  2. Focus Request Queue System

  Queue Management:
  - Buffered channel with capacity 100 for focus requests
  - Priority queue for urgent focus changes (e.g., user keyboard navigation)
  - Request deduplication to prevent redundant operations
  - Request coalescing for rapid focus changes

  Request Processing:
  func (fsm *FocusStateMachine) processQueue() {
      for request := range fsm.requestQueue {
          // Validate request
          if !fsm.validateRequest(request) {
              continue
          }

          // Check for higher priority requests
          if fsm.hasHigherPriorityRequest(request) {
              fsm.requeue(request)
              continue
          }

          // Execute state transition
          fsm.transitionTo(StateTransitioning)
          fsm.executeFocusChange(request)
          fsm.transitionTo(StateFocused)

          // Start settling timer
          fsm.startSettlingTimer()
      }
  }

  3. CSS Reconciliation Engine

  Reconciler Responsibilities:
  - Track expected CSS state for each pane
  - Detect CSS class inconsistencies
  - Apply corrective CSS changes atomically
  - Verify CSS state after GTK main loop iteration

  Implementation:
  type CSSReconciler struct {
      expectedState map[*paneNode]CSSClasses
      mu           sync.RWMutex
  }

  type CSSClasses struct {
      Base      []string  // workspace-pane, workspace-multi-pane
      Active    bool      // workspace-pane-active
      Stacked   []string  // stacked-pane-active, stacked-pane-collapsed
      Custom    []string  // Any additional classes
      Timestamp time.Time
  }

  func (cr *CSSReconciler) reconcile(node *paneNode, expected CSSClasses) error {
      actual := cr.getCurrentClasses(node)
      if !cr.matches(actual, expected) {
          return cr.applyClasses(node, expected)
      }
      return nil
  }

  4. GTK4 Focus Integration

  Native Focus Tracking:
  // In workspace_widgets_cgo.go
  static void focus_enter_cb(GtkEventControllerFocus* controller, gpointer user_data) {
      uintptr_t nodePtr = (uintptr_t)user_data;
      goFocusEnterCallback(nodePtr);
  }

  static void focus_leave_cb(GtkEventControllerFocus* controller, gpointer user_data) {
      uintptr_t nodePtr = (uintptr_t)user_data;
      goFocusLeaveCallback(nodePtr);
  }

  static GtkEventController* widget_add_focus_controller(GtkWidget* widget, uintptr_t nodePtr) {
      GtkEventController* focus = gtk_event_controller_focus_new();
      g_signal_connect(focus, "enter", G_CALLBACK(focus_enter_cb), (gpointer)nodePtr);
      g_signal_connect(focus, "leave", G_CALLBACK(focus_leave_cb), (gpointer)nodePtr);
      gtk_widget_add_controller(widget, focus);
      return focus;
  }

  5. Focus History & Debugging

  Ring Buffer for History:
  type FocusTransition struct {
      From      *paneNode
      To        *paneNode
      State     FocusState
      Source    FocusSource
      Timestamp time.Time
      Success   bool
      Error     error
      Stack     []byte // Stack trace for debugging
  }

  type RingBuffer[T any] struct {
      buffer []T
      head   int
      tail   int
      size   int
      mu     sync.RWMutex
  }

  Initialization Sequence

  The initialization sequence ensures proper focus state from startup:

  func (fsm *FocusStateMachine) Initialize(wm *WorkspaceManager) error {
      fsm.transitionTo(StateInitializing)

      // Phase 1: Setup GTK controllers for all panes
      leaves := wm.collectLeaves()
      for _, leaf := range leaves {
          fsm.attachGTKController(leaf)
      }

      // Phase 2: Determine initial focus
      initialPane := fsm.determineInitialFocus(leaves)

      // Phase 3: Apply initial focus and CSS
      fsm.applyInitialFocus(initialPane)

      // Phase 4: Start queue processor
      go fsm.processQueue()

      // Phase 5: Transition to idle
      fsm.transitionTo(StateIdle)

      return nil
  }

  func (fsm *FocusStateMachine) determineInitialFocus(leaves []*paneNode) *paneNode {
      // Priority order:
      // 1. First pane with user content
      // 2. Top-left pane (geometrically)
      // 3. First pane in tree order

      for _, leaf := range leaves {
          if leaf.pane != nil && leaf.pane.webView != nil {
              if leaf.pane.webView.GetURL() != "about:blank" {
                  return leaf
              }
          }
      }

      // Fall back to geometric selection
      return fsm.findTopLeftPane(leaves)
  }

  Focus Request Flow

  graph TD
      A[Focus Request] --> B{Validate Request}
      B -->|Valid| C[Add to Queue]
      B -->|Invalid| D[Log & Reject]
      C --> E{Check State}
      E -->|Idle| F[Process Immediately]
      E -->|Busy| G[Wait in Queue]
      F --> H[Transition to Processing]
      H --> I[Remove Old Focus]
      I --> J[Apply New Focus]
      J --> K[Update CSS Classes]
      K --> L[Notify Observers]
      L --> M[Start Settling Timer]
      M --> N[Transition to Focused]
      N --> O[After Settling: Idle]

  Key Methods

  Request Focus Change

  func (fsm *FocusStateMachine) RequestFocus(node *paneNode, source FocusSource) error {
      request := FocusRequest{
          ID:         generateRequestID(),
          TargetNode: node,
          Source:     source,
          Priority:   fsm.calculatePriority(source),
          Timestamp:  time.Now(),
      }

      select {
      case fsm.requestQueue <- request:
          return nil
      case <-time.After(100 * time.Millisecond):
          return ErrFocusQueueFull
      }
  }

  Execute Focus Change

  func (fsm *FocusStateMachine) executeFocusChange(request FocusRequest) error {
      fsm.mu.Lock()
      defer fsm.mu.Unlock()

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

      // Phase 1: Validate target is still valid
      if !fsm.isValidTarget(newPane) {
          transition.Success = false
          transition.Error = ErrInvalidTarget
          fsm.recordTransition(transition)
          return ErrInvalidTarget
      }

      // Phase 2: Remove focus from old pane
      if oldPane != nil {
          fsm.removeFocusFrom(oldPane)
      }

      // Phase 3: Apply focus to new pane
      if err := fsm.applyFocusTo(newPane); err != nil {
          // Rollback on failure
          if oldPane != nil {
              fsm.applyFocusTo(oldPane)
          }
          transition.Success = false
          transition.Error = err
          fsm.recordTransition(transition)
          return err
      }

      // Phase 4: Update CSS classes atomically
      fsm.cssReconciler.beginTransaction()
      if oldPane != nil {
          fsm.cssReconciler.removeClass(oldPane, "workspace-pane-active")
      }
      fsm.cssReconciler.addClass(newPane, "workspace-pane-active")
      fsm.cssReconciler.commit()

      // Phase 5: Update state
      fsm.activePane = newPane
      transition.Success = true
      fsm.recordTransition(transition)

      // Phase 6: Notify observers
      fsm.notifyFocusChange(oldPane, newPane)

      return nil
  }

  CSS Verification Loop

  func (fsm *FocusStateMachine) startCSSVerificationLoop() {
      ticker := time.NewTicker(100 * time.Millisecond)
      go func() {
          for range ticker.C {
              fsm.verifyCSSState()
          }
      }()
  }

  func (fsm *FocusStateMachine) verifyCSSState() {
      fsm.mu.RLock()
      activePane := fsm.activePane
      fsm.mu.RUnlock()

      if activePane == nil {
          return
      }

      leaves := fsm.wm.collectLeaves()
      activeCount := 0

      for _, leaf := range leaves {
          hasActive := fsm.hasActiveClass(leaf)
          shouldHaveActive := (leaf == activePane)

          if hasActive && shouldHaveActive {
              activeCount++
          } else if hasActive && !shouldHaveActive {
              // ERROR: Inactive pane has active class
              log.Printf("[FSM] CSS ERROR: Inactive pane has active class, fixing...")
              fsm.cssReconciler.removeClass(leaf, "workspace-pane-active")
          } else if !hasActive && shouldHaveActive {
              // ERROR: Active pane missing active class
              log.Printf("[FSM] CSS ERROR: Active pane missing active class, fixing...")
              fsm.cssReconciler.addClass(leaf, "workspace-pane-active")
          }
      }

      if activeCount > 1 {
          log.Fatalf("[FSM] CRITICAL: Multiple active panes detected: %d", activeCount)
      }
  }

  Stacked Pane Integration

  For stacked panes, the state machine handles special transitions:

  func (fsm *FocusStateMachine) handleStackedFocus(stack *paneNode, index int) error {
      if !stack.isStacked || index >= len(stack.stackedPanes) {
          return ErrInvalidStackOperation
      }

      request := FocusRequest{
          TargetNode: stack.stackedPanes[index],
          Source:     SourceStackNavigation,
          Priority:   PriorityHigh,
          Context: map[string]interface{}{
              "stack":     stack,
              "oldIndex":  stack.activeStackIndex,
              "newIndex":  index,
          },
      }

      return fsm.RequestFocus(stack.stackedPanes[index], SourceStackNavigation)
  }

  Debug Commands

  Add debug commands for testing:

  func (fsm *FocusStateMachine) DumpState() FocusDebugInfo {
      fsm.mu.RLock()
      defer fsm.mu.RUnlock()

      return FocusDebugInfo{
          CurrentState:    fsm.currentState,
          ActivePane:      fsm.activePane,
          QueueLength:     len(fsm.requestQueue),
          History:         fsm.historyRing.GetAll(),
          CSSState:        fsm.cssReconciler.GetState(),
          ValidationErrors: fsm.getValidationErrors(),
      }
  }

  func (fsm *FocusStateMachine) ForceReconcile() {
      fsm.transitionTo(StateReconciling)
      fsm.cssReconciler.forceFullReconciliation()
      fsm.transitionTo(StateIdle)
  }

  Implementation Timeline

  Phase 1: Core State Machine (3-4 hours)
  - Create focus_state_machine.go with basic state management
  - Implement queue system and request processing
  - Add state transition logic and validation

  Phase 2: CSS Reconciler (2-3 hours)
  - Implement CSS tracking and reconciliation
  - Add transaction support for atomic updates
  - Create verification loop

  Phase 3: GTK Integration (2-3 hours)
  - Add GtkEventControllerFocus bindings
  - Implement callbacks and signal handling
  - Sync with state machine

  Phase 4: Testing & Debugging (2-3 hours)
  - Add comprehensive logging
  - Implement debug commands
  - Test all navigation scenarios
  - Verify stacked pane behavior

  Benefits of This Approach

  1. Guaranteed Single Active Pane: Queue serialization ensures atomic focus changes
  2. Self-Healing: CSS verification loop detects and fixes inconsistencies
  3. Debuggable: Complete history and state inspection available
  4. Predictable: State machine ensures valid transitions only
  5. Performant: Async queue processing doesn't block UI
  6. Extensible: Easy to add new focus sources or validation rules

  This architecture will completely eliminate the focus initialization issues and provide a robust
  foundation for complex pane management scenarios.
