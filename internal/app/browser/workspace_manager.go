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
	paneModeSource    *webkit.WebView // Which webview initiated pane mode
	lastPaneModeEntry time.Time       // When pane mode was last entered
	paneMutex         sync.Mutex      // Protects pane mode state

	// Focus throttling fields to prevent infinite loops
	lastFocusChange    time.Time  // When focus was last changed
	focusThrottleMutex sync.Mutex // Protects focus throttling state

	// Stack operation timing to prevent focus conflicts
	lastStackOperation time.Time // When a stack operation was last performed

	// NEW: Pane creation deduplicator
	paneDeduplicator *messaging.PaneRequestDeduplicator

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
		app:              app,
		window:           rootPane.webView.Window(),
		viewToNode:       make(map[*webkit.WebView]*paneNode),
		lastSplitMsg:     make(map[*webkit.WebView]time.Time),
		lastExitMsg:      make(map[*webkit.WebView]time.Time),
		paneDeduplicator: messaging.NewPaneRequestDeduplicator(),
		focusDebounce:    150 * time.Millisecond,
		debugLevel:       debugLevel,
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

	if manager.debugPaneClose {
		manager.dumpTreeState("workspace_init")
	}

	return manager
}

// OnWorkspaceMessage implements messaging.WorkspaceObserver.
func (wm *WorkspaceManager) OnWorkspaceMessage(source *webkit.WebView, msg messaging.Message) {
	node := wm.viewToNode[source]
	if node == nil {
		log.Printf("[workspace] message from unknown webview: event=%s", msg.Event)
		return
	}

	switch event := strings.ToLower(msg.Event); event {
	case "pane-mode-entered":
		// Use mutex to prevent race conditions when multiple webviews try to enter pane mode
		wm.paneMutex.Lock()
		defer wm.paneMutex.Unlock()

		// Check if pane mode was already entered recently (within 200ms debounce window)
		if time.Since(wm.lastPaneModeEntry) < 200*time.Millisecond {
			log.Printf("[workspace] pane-mode-entered rejected: debounce protection (%.0fms ago) source=%p",
				time.Since(wm.lastPaneModeEntry).Seconds()*1000, source)
			return
		}

		// Check if pane mode is already active from a different source
		if wm.paneModeActive && wm.paneModeSource != nil && wm.paneModeSource != source {
			log.Printf("[workspace] pane-mode-entered rejected: already active from different source (current=%p, new=%p)",
				wm.paneModeSource, source)
			return
		}

		log.Printf("[workspace] pane-mode-entered accepted: source=%p", source)
		wm.paneModeActive = true
		wm.paneModeSource = source
		wm.lastPaneModeEntry = time.Now()
		wm.SetActivePane(node, SourceProgrammatic)
	case "pane-confirmed", "pane-cancelled", "pane-mode-exited":
		// Debounce pane-mode-exited events to prevent duplicate focus calls
		if event == "pane-mode-exited" {
			if last, ok := wm.lastExitMsg[source]; ok {
				if time.Since(last) < 100*time.Millisecond {
					log.Printf("[workspace] pane-mode-exited ignored: debounce (%.0fms)", time.Since(last).Seconds()*1000)
					return
				}
			}
			wm.lastExitMsg[source] = time.Now()
		}

		wm.paneMutex.Lock()
		wm.paneModeActive = false
		wm.paneModeSource = nil
		wm.paneMutex.Unlock()

		wm.focusAfterPaneMode(node)
	case "pane-close":
		if !wm.paneModeActive {
			log.Printf("[workspace] close requested outside pane mode; ignoring")
			break
		}

		wm.paneMutex.Lock()
		wm.paneModeActive = false
		wm.paneModeSource = nil
		wm.paneMutex.Unlock()

		// Don't focus the node that's about to be closed - closeCurrentPane() will handle focus
		wm.closeCurrentPane()
	case "pane-split":
		wm.SetActivePane(node, SourceProgrammatic)
		direction := strings.ToLower(msg.Direction)
		if direction == "" {
			direction = "right"
		}
		if last, ok := wm.lastSplitMsg[source]; ok {
			if time.Since(last) < 200*time.Millisecond {
				log.Printf("[workspace] split ignored: debounce (%.0fms)", time.Since(last).Seconds()*1000)
				return
			}
		}

		// CRITICAL FIX: Use atomic operations for splitting flag to prevent race conditions
		if !atomic.CompareAndSwapInt32(&wm.splitting, 0, 1) {
			log.Printf("[workspace] split ignored: operation already in progress")
			return
		}

		// CRITICAL FIX: Always clear splitting flag on exit (panic recovery)
		defer func() {
			atomic.StoreInt32(&wm.splitting, 0)
			if r := recover(); r != nil {
				log.Printf("[workspace] split operation panicked, flag cleared: %v", r)
				panic(r) // Re-panic to maintain error handling
			}
		}()

		wm.lastSplitMsg[source] = time.Now()

		newNode, err := wm.SplitPane(node, direction)
		if err != nil {
			log.Printf("[workspace] split failed: %v", err)
			// CRITICAL FIX: Pump GTK events after validation failure to clear pending operations
			if glib.MainContextDefault().IsOwner() {
				glib.MainContextDefault().Iteration(false)
			}
			return
		}
		wm.clonePaneState(node, newNode)
	case "pane-stack":
		wm.SetActivePane(node, SourceProgrammatic)
		if last, ok := wm.lastSplitMsg[source]; ok {
			if time.Since(last) < 200*time.Millisecond {
				log.Printf("[workspace] stack ignored: debounce (%.0fms)", time.Since(last).Seconds()*1000)
				return
			}
		}

		// CRITICAL FIX: Use atomic operations for splitting flag
		if !atomic.CompareAndSwapInt32(&wm.splitting, 0, 1) {
			log.Printf("[workspace] stack ignored: operation already in progress")
			return
		}

		// CRITICAL FIX: Always clear splitting flag on exit (panic recovery)
		defer func() {
			atomic.StoreInt32(&wm.splitting, 0)
			if r := recover(); r != nil {
				log.Printf("[workspace] stack operation panicked, flag cleared: %v", r)
				panic(r) // Re-panic to maintain error handling
			}
		}()

		wm.lastSplitMsg[source] = time.Now()

		newNode, err := wm.stackedPaneManager.StackPane(node)
		if err != nil {
			log.Printf("[workspace] stack failed: %v", err)
			// CRITICAL FIX: Pump GTK events after stack failure
			if glib.MainContextDefault().IsOwner() {
				glib.MainContextDefault().Iteration(false)
			}
			return
		}
		wm.clonePaneState(node, newNode)
	case "create-pane":
		log.Printf("[workspace] create-pane requested: url=%s action=%s requestId=%s", msg.URL, msg.Action, msg.RequestID)

		if msg.URL == "" {
			log.Printf("[workspace] create-pane: empty URL, ignoring")
			break
		}

		// NEW: Get WebView ID for deduplication
		webViewID := "unknown"
		if source != nil {
			// Try to get a unique identifier for the WebView
			webViewID = fmt.Sprintf("%p", source)
		}

		// NEW: Create intent for deduplication check
		intent := &messaging.WindowIntent{
			URL:           msg.URL,
			WindowType:    msg.Action,
			Timestamp:     time.Now().UnixMilli(),
			RequestID:     msg.RequestID,
			UserTriggered: true,
		}

		// NEW: Check for duplicates
		if isDup, reason := wm.paneDeduplicator.IsDuplicate(intent, webViewID); isDup {
			log.Printf("[workspace] create-pane BLOCKED: %s", reason)
			break
		}

		// Use the existing methods to handle tab vs popup creation
		switch strings.ToLower(msg.Action) {
		case "tab":
			newView := wm.handleIntentAsTab(node, msg.URL, intent)
			if newView != nil {
				log.Printf("[workspace] create-pane: tab created successfully")
			} else {
				log.Printf("[workspace] create-pane: failed to create tab")
			}
		case "popup":
			newView := wm.handleIntentAsPopup(node, msg.URL, intent)
			if newView != nil {
				log.Printf("[workspace] create-pane: popup created successfully")
			} else {
				log.Printf("[workspace] create-pane: failed to create popup")
			}
		default:
			log.Printf("[workspace] create-pane: unknown action '%s', defaulting to tab", msg.Action)
			newView := wm.handleIntentAsTab(node, msg.URL, intent)
			if newView != nil {
				log.Printf("[workspace] create-pane: default tab created successfully")
			} else {
				log.Printf("[workspace] create-pane: failed to create default tab")
			}
		}
	case "close-popup":
		log.Printf("[workspace] close-popup requested: webviewId=%s reason=%s", msg.WebViewID, msg.Reason)

		if msg.WebViewID == "" {
			log.Printf("[workspace] close-popup: empty webviewId, ignoring")
			break
		}

		// Find the popup pane by webview ID
		var targetNode *paneNode
		for webView, node := range wm.viewToNode {
			if webView != nil && fmt.Sprintf("%d", webView.ID()) == msg.WebViewID {
				targetNode = node
				break
			}
		}

		if targetNode == nil {
			log.Printf("[workspace] close-popup: webview not found: %s", msg.WebViewID)
			break
		}

		if !targetNode.isPopup {
			log.Printf("[workspace] close-popup: target is not a popup: %s", msg.WebViewID)
			break
		}

		log.Printf("[workspace] Closing popup pane due to %s", msg.Reason)

		// Remove this popup from parent's activePopupChildren before closing
		if targetNode.parentPane != nil {
			parentNode := targetNode.parentPane
			for i, childID := range parentNode.activePopupChildren {
				if childID == msg.WebViewID {
					parentNode.activePopupChildren = append(parentNode.activePopupChildren[:i], parentNode.activePopupChildren[i+1:]...)
					log.Printf("[workspace] Removed popup %s from parent's activePopupChildren (remaining: %d)", msg.WebViewID, len(parentNode.activePopupChildren))
					break
				}
			}

			// Clean up ALL localStorage entries for this popup in parent WebView
			// This includes popup_<requestId>_parent_info, popup_<requestId>_parent_action, etc.
			if parentNode.pane != nil && parentNode.pane.WebView() != nil && targetNode.requestID != "" {
				parentWebViewID := parentNode.pane.WebView().ID()
				requestID := targetNode.requestID

				cleanupScript := fmt.Sprintf(`
					try {
						// Clean up popup_mapping_<parentId> (from Go code)
						localStorage.removeItem('popup_mapping_%s');

						// Clean up popup_<requestId>_* entries (from JavaScript)
						const requestId = '%s';
						const keysToRemove = [
							'popup_' + requestId + '_parent_info',
							'popup_' + requestId + '_parent_action',
							'popup_' + requestId + '_message_to_popup',
							'popup_' + requestId + '_message_to_parent',
							'oauth_callback_' + '%s'
						];

						keysToRemove.forEach(key => {
							try {
								localStorage.removeItem(key);
							} catch(e) {}
						});

						console.log('[workspace] Cleaned up popup localStorage for requestId:', requestId);
					} catch(e) {
						console.warn('[workspace] Failed to clean popup localStorage:', e);
					}
				`, parentWebViewID, requestID, msg.WebViewID)

				if err := parentNode.pane.WebView().InjectScript(cleanupScript); err != nil {
					log.Printf("[workspace] Failed to inject localStorage cleanup script: %v", err)
				} else {
					log.Printf("[workspace] Cleaned up localStorage for popup requestId=%s webviewId=%s", requestID, msg.WebViewID)
				}
			}
		}

		// Close the popup pane
		if err := wm.ClosePane(targetNode); err != nil {
			log.Printf("[workspace] Failed to close popup pane: %v", err)
		} else {
			log.Printf("[workspace] Successfully closed popup pane: %s", msg.WebViewID)

			// Fix GTK rendering for parent pane after popup closes
			// The parent pane's rendering can become corrupted after popup closes
			if targetNode.parentPane != nil {
				parentNode := targetNode.parentPane
				if parentNode.container != nil {
					log.Printf("[workspace] Fixing parent pane rendering after popup close")

					// CRITICAL: Hide parent container to disconnect WebKitGTK rendering pipeline
					webkit.WidgetHide(parentNode.container)

					// Schedule showing and forcing GTK to reconnect rendering
					wm.scheduleIdleGuarded(func() bool {
						if parentNode == nil || !parentNode.widgetValid || parentNode.container == nil {
							return false
						}
						// Show widget to reconnect WebKit rendering pipeline
						webkit.WidgetShow(parentNode.container)
						// Force GTK to recalculate size and recreate rendering surface
						webkit.WidgetQueueResize(parentNode.container)
						webkit.WidgetQueueDraw(parentNode.container)

						// Also queue resize+draw on the WebView widget itself
						if parentNode.pane != nil && parentNode.pane.WebView() != nil {
							webViewWidget := parentNode.pane.WebView().Widget()
							if webViewWidget != nil {
								webkit.WidgetQueueResize(webViewWidget)
								webkit.WidgetQueueDraw(webViewWidget)
								log.Printf("[workspace] Queued resize+draw for parent WebView widget")
							}
						}

						log.Printf("[workspace] Shown and queued resize+draw for parent pane after popup close")
						return false
					}, parentNode)
				}
			}
		}
	default:
		log.Printf("[workspace] unhandled workspace event: %s", msg.Event)
	}
}

// GetActiveNode returns the currently active pane node
func (wm *WorkspaceManager) GetActiveNode() *paneNode {
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

// RegisterNavigationHandler sets up navigation handling for a webview (simplified)
func (wm *WorkspaceManager) RegisterNavigationHandler(webView *webkit.WebView) {
	if webView == nil {
		return
	}

	log.Printf("[workspace] Registered navigation handler for webview: %s", webView.ID())
}
