// workspace_manager.go - Core workspace management and coordination
package browser

import (
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bnema/dumber/internal/app/messaging"
	"github.com/bnema/dumber/pkg/webkit"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// WorkspaceManager coordinates Zellij-style pane operations.
type WorkspaceManager struct {
	app             *BrowserApp
	window          *webkit.Window
	root            *paneNode
	mainPane        *paneNode
	viewToNode      map[*webkit.WebView]*paneNode
	lastSplitMsg    map[*webkit.WebView]time.Time
	lastExitMsg     map[*webkit.WebView]time.Time
	paneModeActive  bool
	splitting       int32 // atomic: 0=false, 1=true
	cssInitialized  bool
	createWebViewFn func() (*webkit.WebView, error)
	createPaneFn    func(*webkit.WebView) (*BrowserPane, error)

	// Coordination fields for preventing duplicate events
	paneModeSource     *webkit.WebView // Which webview initiated pane mode
	paneModeActivePane *paneNode       // Which pane node has pane mode active
	paneModeContainer  gtk.Widgetter   // The container widget that has margin applied
	paneMutex          sync.Mutex      // Protects pane mode state

	// Focus throttling fields to prevent infinite loops
	lastFocusChange    time.Time  // When focus was last changed
	focusThrottleMutex sync.Mutex // Protects focus throttling state

	// Stack operation timing to prevent focus conflicts
	lastStackOperation time.Time // When a stack operation was last performed

	// NEW: Pane creation deduplicator
	// Popups waiting for ready-to-show lifecycle callback
	pendingPopups map[uint64]*pendingPopup

	// Specialized managers for different pane operations
	stackedPaneManager *StackedPaneManager
	focusStateMachine  *FocusStateMachine

	// Focus debouncing to prevent rapid oscillation from any source
	lastFocusTime   time.Time
	lastFocusTarget *paneNode
	focusDebounce   time.Duration

	// Validation and safety systems (debug-only, controlled by DUMBER_DEBUG_WORKSPACE env)
	treeValidator         *TreeValidator         // Debug-only tree validation
	treeRebalancer        *TreeRebalancer        // Rebalance after close operations
	geometryValidator     *GeometryValidator     // Validate split constraints
	stackLifecycleManager *StackLifecycleManager // Stack pane lifecycle

	// Debug instrumentation helpers
	debugLevel     DebugLevel
	debugPaneClose bool
	diagnostics    *WorkspaceDiagnostics

	// Cleanup tracking
	cleanupCounter uint
	pendingIdle    map[uintptr][]*paneNode
}

// Workspace navigation shortcuts are now handled globally by WindowShortcutHandler

// NewWorkspaceManager builds a workspace manager rooted at the provided pane.
func NewWorkspaceManager(app *BrowserApp, rootPane *BrowserPane) *WorkspaceManager {
	debugLevel := getDebugLevel()

	manager := &WorkspaceManager{
		app:           app,
		window:        rootPane.webView.Window(),
		viewToNode:    make(map[*webkit.WebView]*paneNode),
		lastSplitMsg:  make(map[*webkit.WebView]time.Time),
		lastExitMsg:   make(map[*webkit.WebView]time.Time),
		pendingPopups: make(map[uint64]*pendingPopup),
		focusDebounce: 150 * time.Millisecond,
		debugLevel:    debugLevel,
	}

	// Initialize validation components (opt-in based on debug level)
	if debugLevel >= DebugBasic {
		manager.treeValidator = NewTreeValidator(true, debugLevel == DebugFull)
		manager.geometryValidator = NewGeometryValidator()
		manager.geometryValidator.SetDebugMode(debugLevel == DebugFull)
	} else {
		// Production mode: disable validators
		manager.treeValidator = NewTreeValidator(false, false)
		manager.geometryValidator = NewGeometryValidator()
	}

	// Initialize tree rebalancer (always needed for proper close operations)
	manager.treeRebalancer = NewTreeRebalancer(manager, manager.treeValidator)

	// Initialize stack lifecycle manager
	manager.stackLifecycleManager = NewStackLifecycleManager(manager, manager.treeValidator)

	// Initialize existing specialized managers (now enhanced with bulletproof components)
	manager.stackedPaneManager = NewStackedPaneManager(manager)
	manager.focusStateMachine = NewFocusStateMachine(manager)
	manager.createWebViewFn = func() (*webkit.WebView, error) {
		if manager.app == nil {
			return nil, errors.New("workspace manager missing app reference")
		}
		cfg, err := manager.app.buildWebkitConfig()
		if err != nil {
			return nil, err
		}
		// CRITICAL: Pane WebViews are embedded inside the workspace paned containers,
		// so they do not need their own toplevel GtkWindow. Setting CreateWindow=false
		// prevents GTK "widget already has parent" errors when adding to paned containers.
		cfg.CreateWindow = false
		return webkit.NewWebView(cfg)
	}
	manager.createPaneFn = func(view *webkit.WebView) (*BrowserPane, error) {
		if manager.app == nil {
			return nil, errors.New("workspace manager missing app reference")
		}
		return manager.app.createPaneForView(view)
	}
	manager.ensureWorkspaceStyles()

	rootContainer := rootPane.webView.RootWidget()
	root := &paneNode{
		pane:   rootPane,
		isLeaf: true,
	}
	// Initialize widgets properly
	manager.initializePaneWidgets(root, rootContainer)

	manager.root = root
	manager.mainPane = root
	manager.viewToNode[rootPane.webView] = root
	manager.ensureHover(root)

	if handler := rootPane.MessageHandler(); handler != nil {
		handler.SetWorkspaceObserver(manager)
	}

	app.workspace = manager

	// Configure debug settings from app config
	if app.config != nil {
		manager.focusStateMachine.ConfigureDebug(
			app.config.Debug.EnableFocusDebug,
			false, // CSS debug removed
			app.config.Debug.EnableFocusMetrics,
		)

		manager.debugPaneClose = app.config.Debug.EnablePaneCloseDebug
	}

	manager.diagnostics = NewWorkspaceDiagnostics(manager.debugPaneClose)

	// Initialize focus state machine after all setup is complete
	if err := manager.focusStateMachine.Initialize(); err != nil {
		log.Printf("[workspace] Failed to initialize focus state machine: %v", err)
	}

	// Ensure hover controllers are attached to all panes for mouse-driven focus changes
	manager.attachHoverHandlersToAllPanes()

	if manager.debugPaneClose {
		manager.dumpTreeState("workspace_init")
	}

	return manager
}

// shouldDebounce checks if an operation should be debounced based on timing
func (wm *WorkspaceManager) shouldDebounce(source *webkit.WebView, threshold time.Duration) bool {
	if last, ok := wm.lastSplitMsg[source]; ok {
		if elapsed := time.Since(last); elapsed < threshold {
			log.Printf("[workspace] operation debounced: %.0fms", elapsed.Seconds()*1000)
			return true
		}
	}
	wm.lastSplitMsg[source] = time.Now()
	return false
}

// withSplitLock executes an operation with atomic splitting lock protection
func (wm *WorkspaceManager) withSplitLock(operation string, fn func() error) error {
	if !atomic.CompareAndSwapInt32(&wm.splitting, 0, 1) {
		log.Printf("[workspace] %s ignored: operation in progress", operation)
		return fmt.Errorf("operation in progress")
	}

	defer func() {
		atomic.StoreInt32(&wm.splitting, 0)
		if r := recover(); r != nil {
			log.Printf("[workspace] %s panicked: %v", operation, r)
			panic(r)
		}
	}()

	return fn()
}

// cleanupPopupStorage removes localStorage entries for a closed popup
func (wm *WorkspaceManager) cleanupPopupStorage(parentNode, targetNode *paneNode, webviewID string) {
	if parentNode.pane == nil || parentNode.pane.WebView() == nil || targetNode.requestID == "" {
		return
	}

	requestID := targetNode.requestID
	parentWebViewID := parentNode.pane.WebView().ID()

	script := fmt.Sprintf(`
		try {
			localStorage.removeItem('popup_mapping_%d');
			const keys = ['popup_%s_parent_info', 'popup_%s_parent_action',
			             'popup_%s_message_to_popup', 'popup_%s_message_to_parent',
			             'oauth_callback_%s'];
			keys.forEach(k => { try { localStorage.removeItem(k); } catch(e) {} });
			console.log('[workspace] Cleaned popup localStorage');
		} catch(e) { console.warn('[workspace] localStorage cleanup failed:', e); }
	`, parentWebViewID, requestID, requestID, requestID, requestID, webviewID)

	if err := parentNode.pane.WebView().InjectScript(script); err != nil {
		log.Printf("[workspace] localStorage cleanup failed: %v", err)
	} else {
		log.Printf("[workspace] Cleaned localStorage for popup requestId=%s webviewId=%s", requestID, webviewID)
	}
}

// OnWorkspaceMessage implements messaging.WorkspaceObserver.
func (wm *WorkspaceManager) OnWorkspaceMessage(source *webkit.WebView, msg messaging.Message) {
	node := wm.viewToNode[source]

	if msg.WebViewID != "" {
		if explicitNode := wm.findNodeByWebViewID(msg.WebViewID); explicitNode != nil {
			if explicitNode.pane != nil && explicitNode.pane.webView != nil {
				node = explicitNode
				source = explicitNode.pane.webView
			}
		}
	}

	if node == nil {
		log.Printf("[workspace] message from unknown webview: event=%s webviewId=%s", msg.Event, msg.WebViewID)
		return
	}

	switch event := strings.ToLower(msg.Event); event {
	case "pane-split":
		wm.SetActivePane(node, SourceProgrammatic)
		direction := strings.ToLower(msg.Direction)
		if direction == "" {
			direction = "right"
		}

		if wm.shouldDebounce(source, 200*time.Millisecond) {
			return
		}

		err := wm.withSplitLock("split", func() error {
			newNode, err := wm.SplitPane(node, direction)
			if err != nil {
				if glib.MainContextDefault().IsOwner() {
					glib.MainContextDefault().Iteration(false)
				}
				return err
			}
			wm.clonePaneState(node, newNode)
			return nil
		})

		if err != nil {
			log.Printf("[workspace] split failed: %v", err)
		}

	case "pane-stack":
		wm.SetActivePane(node, SourceProgrammatic)

		if wm.shouldDebounce(source, 200*time.Millisecond) {
			return
		}

		err := wm.withSplitLock("stack", func() error {
			newNode, err := wm.stackedPaneManager.StackPane(node)
			if err != nil {
				if glib.MainContextDefault().IsOwner() {
					glib.MainContextDefault().Iteration(false)
				}
				return err
			}
			wm.clonePaneState(node, newNode)
			return nil
		})

		if err != nil {
			log.Printf("[workspace] stack failed: %v", err)
		}

	case "close-popup":
		if msg.WebViewID == "" {
			log.Printf("[workspace] close-popup: empty webviewId, ignoring")
			return
		}

		// Find popup pane by webview ID
		var targetNode *paneNode
		for webView, node := range wm.viewToNode {
			if webView != nil && fmt.Sprintf("%d", webView.ID()) == msg.WebViewID {
				targetNode = node
				break
			}
		}

		if targetNode == nil {
			log.Printf("[workspace] close-popup: webview not found: %s", msg.WebViewID)
			return
		}

		if !targetNode.isPopup {
			log.Printf("[workspace] close-popup: target is not a popup: %s", msg.WebViewID)
			return
		}

		log.Printf("[workspace] Closing popup due to %s", msg.Reason)

		// Remove from parent's active popup list
		if targetNode.parentPane != nil {
			parentNode := targetNode.parentPane
			for i, childID := range parentNode.activePopupChildren {
				if childID == msg.WebViewID {
					parentNode.activePopupChildren = append(parentNode.activePopupChildren[:i], parentNode.activePopupChildren[i+1:]...)
					log.Printf("[workspace] Removed popup from parent (remaining: %d)", len(parentNode.activePopupChildren))
					break
				}
			}

			wm.cleanupPopupStorage(parentNode, targetNode, msg.WebViewID)
		}

		if err := wm.ClosePane(targetNode); err != nil {
			log.Printf("[workspace] Failed to close popup: %v", err)
		}

	default:
		log.Printf("[workspace] unhandled workspace event: %s", msg.Event)
	}
}

func (wm *WorkspaceManager) findNodeByWebViewID(id string) *paneNode {
	if wm == nil || id == "" {
		return nil
	}

	for webView, node := range wm.viewToNode {
		if webView != nil && webView.IDString() == id {
			return node
		}
	}
	return nil
}

// GetActiveNode returns the currently active pane node
func (wm *WorkspaceManager) GetActiveNode() *paneNode {
	if wm == nil || wm.focusStateMachine == nil {
		return nil
	}
	return wm.focusStateMachine.GetActivePane()
}

// SetActivePane requests focus change through the focus state machine with debouncing
func (wm *WorkspaceManager) SetActivePane(node *paneNode, source FocusSource) {
	now := time.Now()

	// Check if we should debounce this focus request (except for urgent system events)
	if source != SourceSystem && now.Sub(wm.lastFocusTime) < wm.focusDebounce && wm.lastFocusTarget == node {
		// Too soon and same target - ignore this request to prevent rapid oscillation
		return
	}

	// Update tracking and proceed with focus change
	wm.lastFocusTime = now
	wm.lastFocusTarget = node

	if err := wm.focusStateMachine.RequestFocus(node, source); err != nil {
		log.Printf("[workspace] Focus request failed: %v", err)
	}
}

// GetNodeForWebView returns the pane node associated with a WebView
func (wm *WorkspaceManager) GetNodeForWebView(webView *webkit.WebView) *paneNode {
	return wm.viewToNode[webView]
}

// RegisterNavigationHandler sets up navigation handling for a webview
func (wm *WorkspaceManager) RegisterNavigationHandler(webView *webkit.WebView) {
	if webView == nil {
		return
	}

	// Register workspace navigation callback for alt+arrow pane navigation
	webView.RegisterWorkspaceNavigationHandler(func(direction string) bool {
		if wm == nil {
			return false
		}
		return wm.FocusNeighbor(direction)
	})

	// Register pane mode callbacks for Ctrl+P and pane mode actions
	webView.RegisterPaneModeHandler(func(action string) bool {
		if wm == nil {
			return false
		}

		if action == "enter" {
			wm.EnterPaneMode()
			return true
		}

		if action == "exit" {
			// Check if pane mode is active
			wm.paneMutex.Lock()
			wasActive := wm.paneModeActive
			wm.paneMutex.Unlock()

			if wasActive {
				wm.ExitPaneMode("escape")
				return true // Mode was active, we handled it
			}
			return false // No mode active, pass through to DOM
		}

		// For other actions, only handle if pane mode is active
		wm.paneMutex.Lock()
		isActive := wm.paneModeActive
		wm.paneMutex.Unlock()

		if !isActive {
			return false // Not in pane mode, allow normal behavior
		}

		// Pane mode is active, handle the action
		wm.HandlePaneAction(action)
		return true
	}, func() bool {
		// Check if pane mode is currently active
		wm.paneMutex.Lock()
		defer wm.paneMutex.Unlock()
		return wm.paneModeActive
	})

	log.Printf("[workspace] Registered navigation handler for webview: %d", webView.ID())
}
