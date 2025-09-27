// workspace_manager.go - Core workspace management and coordination
package browser

import (
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/bnema/dumber/internal/app/messaging"
	"github.com/bnema/dumber/pkg/webkit"
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
	splitting       bool
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

	// Widget operation synchronization to prevent GTK race conditions
	widgetMutex    sync.Mutex
	widgetRegistry *WidgetRegistry
}

// Workspace navigation shortcuts are now handled globally by WindowShortcutHandler

// NewWorkspaceManager builds a workspace manager rooted at the provided pane.
func NewWorkspaceManager(app *BrowserApp, rootPane *BrowserPane) *WorkspaceManager {
	manager := &WorkspaceManager{
		app:              app,
		window:           rootPane.webView.Window(),
		viewToNode:       make(map[*webkit.WebView]*paneNode),
		lastSplitMsg:     make(map[*webkit.WebView]time.Time),
		lastExitMsg:      make(map[*webkit.WebView]time.Time),
		paneDeduplicator: messaging.NewPaneRequestDeduplicator(), // NEW: Initialize deduplicator
		widgetRegistry:   NewWidgetRegistry(),                    // Initialize widget registry
	}

	// Initialize specialized managers
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
		// Keep CreateWindow = true for popup WebViews to ensure proper window features initialization
		// This prevents WindowFeatures optional crashes when WebKit accesses popup properties
		cfg.CreateWindow = true
		return webkit.NewWebView(cfg)
	}
	manager.createPaneFn = func(view *webkit.WebView) (*BrowserPane, error) {
		if manager.app == nil {
			return nil, errors.New("workspace manager missing app reference")
		}
		return manager.app.createPaneForView(view)
	}
	manager.ensureStackedPaneStyles()

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
	}

	// Initialize focus state machine after all setup is complete
	if err := manager.focusStateMachine.Initialize(); err != nil {
		log.Printf("[workspace] Failed to initialize focus state machine: %v", err)
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
		if wm.splitting {
			log.Printf("[workspace] split ignored: operation already in progress")
			return
		}
		wm.splitting = true
		wm.lastSplitMsg[source] = time.Now()
		newNode, err := wm.splitNode(node, direction)
		if err != nil {
			log.Printf("[workspace] split failed: %v", err)
			wm.splitting = false
			return
		}
		wm.clonePaneState(node, newNode)
		wm.splitting = false
	case "pane-stack":
		wm.SetActivePane(node, SourceProgrammatic)
		if last, ok := wm.lastSplitMsg[source]; ok {
			if time.Since(last) < 200*time.Millisecond {
				log.Printf("[workspace] stack ignored: debounce (%.0fms)", time.Since(last).Seconds()*1000)
				return
			}
		}
		if wm.splitting {
			log.Printf("[workspace] stack ignored: operation already in progress")
			return
		}
		wm.splitting = true
		wm.lastSplitMsg[source] = time.Now()
		newNode, err := wm.stackedPaneManager.StackPane(node)
		if err != nil {
			log.Printf("[workspace] stack failed: %v", err)
			wm.splitting = false
			return
		}
		wm.clonePaneState(node, newNode)
		wm.splitting = false
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
			if webView != nil && webView.ID() == msg.WebViewID {
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

		// Close the popup pane
		if err := wm.closePane(targetNode); err != nil {
			log.Printf("[workspace] Failed to close popup pane: %v", err)
		} else {
			log.Printf("[workspace] Successfully closed popup pane: %s", msg.WebViewID)
		}
	default:
		log.Printf("[workspace] unhandled workspace event: %s", msg.Event)
	}
}

// GetActiveNode returns the currently active pane node
func (wm *WorkspaceManager) GetActiveNode() *paneNode {
	return wm.focusStateMachine.GetActivePane()
}

// SetActivePane requests focus change through the focus state machine
func (wm *WorkspaceManager) SetActivePane(node *paneNode, source FocusSource) {
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
