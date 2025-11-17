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

// resolveWindow ensures the workspace has a reference to the GTK window.
func (wm *WorkspaceManager) resolveWindow() *webkit.Window {
	if wm == nil {
		return nil
	}

	if wm.window != nil {
		return wm.window
	}

	if wm.app != nil {
		if wm.app.tabManager != nil {
			if win := wm.app.tabManager.GetWindow(); win != nil {
				wm.window = win
				return win
			}
		}
		if wm.app.webView != nil {
			if win := wm.app.webView.Window(); win != nil {
				wm.window = win
				return win
			}
		}
	}

	return nil
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

	// CRITICAL: Register navigation handler (including pane mode) for the root pane
	// This must happen here because when creating the first tab, app.workspace is nil
	// in createPaneForView, so it doesn't register the handler
	manager.RegisterNavigationHandler(rootPane.webView)

	// NOTE: app.workspace is NOT set here - it's managed exclusively by TabManager.
	// TabManager sets app.workspace during tab switching (including initial tab creation).
	// This prevents workspaces from interfering with each other when multiple tabs exist.

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
			direction = DirectionRight
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

	// Register pane mode callbacks for Ctrl+P and pane mode action keys
	webView.RegisterPaneModeHandler(func(action string) bool {
		if wm == nil {
			return false
		}

		if action == "enter" {
			// CRITICAL FIX: Use app.workspace instead of closure's wm
			// This ensures Ctrl+P always targets the ACTIVE tab's workspace
			if wm.app != nil && wm.app.workspace != nil {
				wm.app.workspace.EnterPaneMode()
				return true
			}
			return false
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

		// For other actions, use active workspace (same pattern as "enter")
		// This ensures action keys work on the correct tab
		if wm.app != nil && wm.app.workspace != nil {
			// Check if pane mode is active on the active workspace
			activeWs := wm.app.workspace
			activeWs.paneMutex.Lock()
			isActive := activeWs.paneModeActive
			activeWs.paneMutex.Unlock()

			if !isActive {
				return false // Not in pane mode, allow normal behavior
			}

			// Pane mode is active, handle the action on active workspace
			activeWs.HandlePaneAction(action)
			return true
		}
		return false
	}, func() bool {
		// Check if pane mode is currently active
		wm.paneMutex.Lock()
		defer wm.paneMutex.Unlock()
		return wm.paneModeActive
	})

	// Register middle-click link handler to open links in new panes
	webView.RegisterMiddleClickLinkHandler(func(linkURL string) bool {
		if wm == nil {
			return false
		}

		log.Printf("[workspace] Middle-click/Ctrl+click on link, opening in new pane: %s", linkURL)

		// Get the node for this webview
		node := wm.GetNodeForWebView(webView)
		if node == nil {
			log.Printf("[workspace] Cannot find node for middle-clicked link")
			return false
		}

		// Get behavior from BlankTargetBehavior config
		behavior := "stacked" // Default
		if wm.app != nil && wm.app.config != nil && wm.app.config.Workspace.Popups.BlankTargetBehavior != "" {
			behavior = strings.ToLower(wm.app.config.Workspace.Popups.BlankTargetBehavior)
		}

		var newNode *paneNode
		var err error

		switch behavior {
		case "stacked":
			// Stack the current pane - this also creates a new pane in the stack
			log.Printf("[workspace] Using stacked behavior for gesture link")
			newNode, err = wm.stackedPaneManager.StackPane(node)
			if err != nil {
				log.Printf("[workspace] Failed to stack pane for gesture link: %v", err)
				return false
			}
			// newNode is the new pane that was added to the stack
			log.Printf("[workspace] StackPane created new pane in stack: %p", newNode)

		case "split":
			// Regular split behavior
			log.Printf("[workspace] Using split behavior for gesture link")
			direction := "right"
			if wm.app.config.Workspace.Popups.Placement != "" {
				direction = strings.ToLower(wm.app.config.Workspace.Popups.Placement)
			}
			newNode, err = wm.SplitPane(node, direction)

		case "tabbed":
			// Tabbed not yet implemented, fall back to split
			log.Printf("[workspace] WARNING: Tabbed behavior not yet implemented for gesture links, falling back to split")
			direction := "right"
			if wm.app.config.Workspace.Popups.Placement != "" {
				direction = strings.ToLower(wm.app.config.Workspace.Popups.Placement)
			}
			newNode, err = wm.SplitPane(node, direction)

		default:
			// Unknown behavior, fall back to split
			log.Printf("[workspace] WARNING: Unknown behavior '%s' for gesture links, falling back to split", behavior)
			direction := "right"
			if wm.app.config.Workspace.Popups.Placement != "" {
				direction = strings.ToLower(wm.app.config.Workspace.Popups.Placement)
			}
			newNode, err = wm.SplitPane(node, direction)
		}

		if err != nil {
			log.Printf("[workspace] Failed to handle gesture link: %v", err)
			return false
		}

		// Navigate the new pane to the URL
		if newNode != nil && newNode.pane != nil && newNode.pane.WebView() != nil {
			if err := newNode.pane.WebView().LoadURL(linkURL); err != nil {
				log.Printf("[workspace] Failed to load URL in new pane: %v", err)
				return false
			}

			// Set the new pane as active
			wm.SetActivePane(newNode, SourceProgrammatic)
		}

		return true // Indicate we handled the click
	})

	log.Printf("[workspace] Registered navigation handler for webview: %d", webView.ID())
}

// GetAllPanes returns all BrowserPanes in this workspace.
// This is used by TabManager to update app-level panes reference.
func (wm *WorkspaceManager) GetAllPanes() []*BrowserPane {
	panes := make([]*BrowserPane, 0, len(wm.viewToNode))
	for _, node := range wm.viewToNode {
		if node.pane != nil {
			panes = append(panes, node.pane)
		}
	}
	return panes
}

// GetActivePane returns the currently active BrowserPane in this workspace.
// This is used by TabManager to update app-level activePane reference.
func (wm *WorkspaceManager) GetActivePane() *BrowserPane {
	node := wm.GetActiveNode()
	if node != nil && node.pane != nil {
		return node.pane
	}
	return nil
}

// RestoreFocus restores focus to the active pane in this workspace.
// Called when switching to a tab to ensure the correct pane has focus.
func (wm *WorkspaceManager) RestoreFocus() {
	log.Printf("[workspace] RestoreFocus: workspace %p", wm)

	// Get the active pane and explicitly grab GTK focus
	activeNode := wm.GetActiveNode()
	if activeNode != nil && activeNode.pane != nil && activeNode.pane.webView != nil {
		// Explicitly grab GTK focus to this WebView
		// This ensures keyboard events go to the active tab's WebView
		// Note: RestoreFocus is called from switchToTabInternal which is already on the main thread
		webkit.WidgetGrabFocus(activeNode.pane.webView.AsWidget())
		log.Printf("[workspace] Grabbed GTK focus for WebView in workspace %p", wm)

		// Also request focus via FSM
		if wm.focusStateMachine != nil {
			wm.focusStateMachine.RequestFocus(activeNode, SourceProgrammatic)
		}
	}
}

// IsInActiveTab returns whether this workspace is the app's current active workspace.
// This is checked dynamically by comparing with app.workspace (updated during tab switching).
func (wm *WorkspaceManager) IsInActiveTab() bool {
	if wm.app == nil {
		return false
	}
	// The active workspace is the one currently assigned to app.workspace
	isActive := wm.app.workspace == wm
	log.Printf("[workspace] IsInActiveTab check: workspace %p, app.workspace %p, isActive=%v", wm, wm.app.workspace, isActive)
	return isActive
}

// ClearFocus removes GTK focus from all panes in this workspace.
// This prevents inactive tab's WebViews from catching keyboard events.
func (wm *WorkspaceManager) ClearFocus() {
	log.Printf("[workspace] ClearFocus: workspace %p", wm)
	// No state to clear - IsInActiveTab() checks dynamically
}

// GetRootWidget returns the root GTK widget for this workspace.
// This is the container that should be added to the tab's content area.
func (wm *WorkspaceManager) GetRootWidget() gtk.Widgetter {
	if wm.root != nil && wm.root.container != nil {
		return wm.root.container
	}
	return nil
}

// setRootContainer attaches a widget as the workspace root to TabManager.ContentArea.
// The tab system is always present (even with a single tab), so this always uses ContentArea.
func (wm *WorkspaceManager) setRootContainer(container gtk.Widgetter) {
	if container == nil {
		return
	}

	if wm.app == nil || wm.app.tabManager == nil || wm.app.tabManager.ContentArea == nil {
		log.Printf("[workspace] ERROR: Cannot set root - tab manager not initialized")
		return
	}

	contentBox, ok := wm.app.tabManager.ContentArea.(*gtk.Box)
	if !ok || contentBox == nil {
		log.Printf("[workspace] ERROR: ContentArea is not a Box")
		return
	}

	contentBox.Append(container)
	webkit.WidgetSetVisible(container, true)
	log.Printf("[workspace] Attached root container %p to ContentArea", container)
}

// clearRootContainer removes the current workspace root from TabManager.ContentArea.
// The tab system is always present (even with a single tab), so this always uses ContentArea.
func (wm *WorkspaceManager) clearRootContainer() {
	if wm.root == nil || wm.root.container == nil {
		return
	}

	if wm.app == nil || wm.app.tabManager == nil || wm.app.tabManager.ContentArea == nil {
		log.Printf("[workspace] ERROR: Cannot clear root - tab manager not initialized")
		return
	}

	contentBox, ok := wm.app.tabManager.ContentArea.(*gtk.Box)
	if !ok || contentBox == nil {
		log.Printf("[workspace] ERROR: ContentArea is not a Box")
		return
	}

	contentBox.Remove(wm.root.container)
	log.Printf("[workspace] Removed root container %p from ContentArea", wm.root.container)
}
