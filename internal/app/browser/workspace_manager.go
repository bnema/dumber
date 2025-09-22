package browser

import (
	"errors"
	"fmt"
	"log"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/bnema/dumber/internal/app/messaging"
	"github.com/bnema/dumber/internal/config"
	"github.com/bnema/dumber/pkg/webkit"
)

type paneNode struct {
	pane        *BrowserPane
	parent      *paneNode
	left        *paneNode
	right       *paneNode
	container   uintptr // GtkPaned for branch nodes, stable WebView container for leaves
	orientation webkit.Orientation
	isLeaf      bool
	isPopup     bool // Deprecated: use windowType instead
	// Window type tracking
	windowType     webkit.WindowType      // Tab or Popup
	windowFeatures *webkit.WindowFeatures // Features if popup
	isRelated      bool                   // Shares context
	parentPane     *paneNode              // Parent for related views
	autoClose      bool                   // Auto-close on OAuth success
	hoverToken     uintptr
}

// WorkspaceManager coordinates Zellij-style pane operations.
type WorkspaceManager struct {
	app             *BrowserApp
	window          *webkit.Window
	root            *paneNode
	active          *paneNode
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

	// NEW: Pane creation deduplicator
	paneDeduplicator *messaging.PaneRequestDeduplicator
}

const (
	activePaneClass = "workspace-pane-active"
	basePaneClass   = "workspace-pane"
	multiPaneClass  = "workspace-multi-pane"
)

// registerWorkspaceShortcuts registers global workspace navigation shortcuts on the given webView.
func (wm *WorkspaceManager) registerWorkspaceShortcuts(webView *webkit.WebView) {
	if wm == nil || webView == nil {
		return
	}

	registerFocusMove := func(key, dir string) {
		_ = webView.RegisterKeyboardShortcut(key, func() {
			log.Printf("[workspace] %s callback triggered on webView=%p", key, webView)
			if wm.app == nil {
				log.Printf("[workspace] %s rejected: app is nil", key)
				return
			}

			// Only allow the active webview to handle workspace navigation shortcuts
			if wm.app.activePane == nil || wm.app.activePane.webView != webView {
				log.Printf("[workspace] %s rejected: not active pane (active=%p, caller=%p)",
					key, wm.app.activePane.webView, webView)
				return
			}

			log.Printf("[workspace] %s calling FocusNeighbor(%s)", key, dir)
			if wm.FocusNeighbor(dir) {
				log.Printf("Shortcut: focus pane %s", dir)
			} else {
				log.Printf("[workspace] FocusNeighbor(%s) returned false", dir)
			}
		})
	}

	// Pane navigation shortcuts
	registerFocusMove("alt+ArrowLeft", "left")
	registerFocusMove("alt+ArrowRight", "right")
	registerFocusMove("alt+ArrowUp", "up")
	registerFocusMove("alt+ArrowDown", "down")
	registerFocusMove("cmdorctrl+ArrowUp", "up")
	registerFocusMove("cmdorctrl+ArrowDown", "down")

	// Zoom shortcuts are now handled by global window shortcuts in window_shortcuts.go
	// for consistency with other global shortcuts like Ctrl+L, Ctrl+F, etc.

	// Reduced logging: only log shortcuts registration during initialization, not on every hover
	if wm.app.config != nil && wm.app.config.Debug.EnableWorkspaceDebug {
		log.Printf("[workspace] registered navigation shortcuts on webView=%p", webView)
	}
}

// NewWorkspaceManager builds a workspace manager rooted at the provided pane.
func NewWorkspaceManager(app *BrowserApp, rootPane *BrowserPane) *WorkspaceManager {
	manager := &WorkspaceManager{
		app:              app,
		window:           rootPane.webView.Window(),
		viewToNode:       make(map[*webkit.WebView]*paneNode),
		lastSplitMsg:     make(map[*webkit.WebView]time.Time),
		lastExitMsg:      make(map[*webkit.WebView]time.Time),
		paneDeduplicator: messaging.NewPaneRequestDeduplicator(), // NEW: Initialize deduplicator
	}
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
	manager.ensureStyles()

	rootContainer := rootPane.webView.RootWidget()
	root := &paneNode{
		pane:      rootPane,
		container: rootContainer,
		isLeaf:    true,
	}
	manager.root = root
	manager.active = root
	manager.mainPane = root
	manager.viewToNode[rootPane.webView] = root
	manager.ensureHover(root)

	if handler := rootPane.MessageHandler(); handler != nil {
		handler.SetWorkspaceObserver(manager)
	}

	app.workspace = manager
	manager.focusNode(root)
	manager.registerWorkspaceShortcuts(root.pane.webView)
	// Ensure initial CSS classes are applied
	manager.ensurePaneBaseClasses()
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
		wm.focusNode(node)
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

		wm.focusNode(node)
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
		wm.focusNode(node)
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
	return wm.active
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

// DispatchPaneFocusEvent sends a workspace focus event to a pane's webview
func (wm *WorkspaceManager) DispatchPaneFocusEvent(node *paneNode, active bool) {
	if node == nil || node.pane == nil || node.pane.webView == nil {
		return
	}

	detail := map[string]any{
		"active":    active,
		"webview":   fmt.Sprintf("%p", node.pane.webView),
		"webviewId": node.pane.webView.ID(),
		"hasGUI":    node.pane.HasGUI(),
		"timestamp": time.Now().UnixMilli(),
	}

	if err := node.pane.webView.DispatchCustomEvent("dumber:workspace-focus", detail); err != nil {
		log.Printf("[workspace] failed to dispatch focus event: %v", err)
	} else if wm.app.config != nil && wm.app.config.Debug.EnableWorkspaceDebug {
		log.Printf("[workspace] dispatched focus event for webview %s (active=%v)", node.pane.webView.ID(), active)
	}
}


func (wm *WorkspaceManager) focusNode(node *paneNode) {
	if node == nil || !node.isLeaf || node.pane == nil || node.pane.webView == nil {
		return
	}

	// Don't focus a node that's not in our viewToNode map (likely destroyed)
	if _, exists := wm.viewToNode[node.pane.webView]; !exists {
		log.Printf("[workspace] focusNode: skipping destroyed/unknown webView=%p", node.pane.webView)
		return
	}

	previous := wm.active
	var previousContainer uintptr
	var previousWebView *webkit.WebView
	if previous != nil && previous != node && previous.pane != nil && previous.pane.webView != nil {
		previousContainer = previous.container
		previousWebView = previous.pane.webView
		wm.DispatchPaneFocusEvent(previous, false)
	}

	// Set previously active WebView as inactive
	if previousWebView != nil && previousWebView != node.pane.webView {
		previousWebView.SetActive(false)
	}

	wm.active = node
	wm.app.activePane = node.pane
	wm.app.webView = node.pane.webView
	wm.app.zoomController = node.pane.zoomController
	wm.app.navigationController = node.pane.navigationController
	wm.app.clipboardController = node.pane.clipboardController
	wm.app.messageHandler = node.pane.messageHandler
	wm.app.shortcutHandler = node.pane.shortcutHandler

	// Set this WebView as active
	if node.pane != nil && node.pane.webView != nil {
		node.pane.webView.SetActive(true)
	}

	// Re-register workspace navigation shortcuts on the newly focused webView
	wm.registerWorkspaceShortcuts(node.pane.webView)

	if handler := node.pane.messageHandler; handler != nil {
		handler.SetWorkspaceObserver(wm)
	}

	if wm.app.browserService != nil {
		wm.app.browserService.AttachWebView(node.pane.webView)
	}

	// Ensure all panes have proper base classes
	wm.ensurePaneBaseClasses()

	container := node.container
	viewWidget := node.pane.webView.Widget()
	if container != 0 && container != previousContainer {
		// Remove active class from previous container to ensure only one pane is visually active
		if previousContainer != 0 {
			webkit.WidgetRemoveCSSClass(previousContainer, activePaneClass)
		}

		// Add active class to current container (border visibility controlled by CSS multi-pane rules)
		webkit.WidgetAddCSSClass(container, activePaneClass)
		if !webkit.WidgetIsValid(container) {
			log.Printf("[workspace] focus aborted: container invalid widget=%#x", container)
			return
		}
		webkit.IdleAdd(func() bool {
			if !webkit.WidgetIsValid(container) {
				log.Printf("[workspace] focus aborted during idle: container invalid widget=%#x", container)
				return false
			}
			parent := webkit.WidgetGetParent(container)
			if parent != 0 {
				webkit.WidgetRealizeInContainer(container)
				if viewWidget != 0 {
					webkit.WidgetGrabFocus(viewWidget)
				}
				// Consolidated focus operation log
				if wm.app.config != nil && wm.app.config.Debug.EnableWorkspaceDebug {
					log.Printf("[workspace] focus operations completed: container=%#x parent=%#x viewWidget=%#x", container, parent, viewWidget)
				}
			} else {
				log.Printf("[workspace] focus deferred: widget not parented")
			}
			return false // Remove idle callback
		})
	}

	if node.pane != nil && node.pane.webView != nil {
		// Update pane focus time
		node.pane.UpdateLastFocus()

		// Always dispatch focus event to ensure JavaScript side knows the active pane
		// This fixes the issue where navigation within the same pane doesn't update JS state
		wm.DispatchPaneFocusEvent(node, true)

		// Lazy-load GUI components if first focus
		if !node.pane.HasGUI() {
			wm.ensureGUIInPane(node.pane)
		}
	}
}

// generateActivePaneCSS generates the CSS for workspace panes based on config
func (wm *WorkspaceManager) generateActivePaneCSS() string {
	cfg := config.Get()
	styling := cfg.Workspace.Styling

	// Get appropriate border colors based on GTK theme preference
	var inactiveBorderColor, windowBackgroundColor string
	isDark := webkit.PrefersDarkTheme()
	if isDark {
		inactiveBorderColor = "#333333" // Dark border for dark theme
		windowBackgroundColor = "#2b2b2b" // Dark window background
	} else {
		inactiveBorderColor = "#dddddd" // Light border for light theme
		windowBackgroundColor = "#ffffff" // Light window background
	}

	// Log the border color values for debugging
	log.Printf("[workspace] GTK prefers dark: %v, inactive border color: %s", isDark, inactiveBorderColor)

	css := fmt.Sprintf(`window {
	  background-color: %s;
	}

	.workspace-pane {
	  background-color: %s;
	  border: %dpx solid %s;
	  transition: border-color %dms ease-in-out;
	  border-radius: %dpx;
	}

	.workspace-pane.workspace-multi-pane {
	  border: %dpx solid %s;
	  border-radius: %dpx;
	}

	.workspace-pane.workspace-multi-pane.workspace-pane-active {
	  border-color: %s;
	}`,
		windowBackgroundColor,
		windowBackgroundColor,
		styling.BorderWidth,
		inactiveBorderColor,
		styling.TransitionDuration,
		styling.BorderRadius,
		styling.BorderWidth,
		inactiveBorderColor,
		styling.BorderRadius,
		styling.BorderColor,
	)

	// Log the actual CSS being generated
	log.Printf("[workspace] Generated CSS: %s", css)

	return css
}

func (wm *WorkspaceManager) ensureStyles() {
	if wm == nil || wm.cssInitialized {
		return
	}
	activePaneCSS := wm.generateActivePaneCSS()
	webkit.AddCSSProvider(activePaneCSS)
	wm.cssInitialized = true
}

// hasMultiplePanes returns true if there are multiple panes in the workspace
func (wm *WorkspaceManager) hasMultiplePanes() bool {
	return wm != nil && wm.app != nil && len(wm.app.panes) > 1
}

// ensurePaneBaseClasses ensures all panes have the proper base CSS classes
func (wm *WorkspaceManager) ensurePaneBaseClasses() {
	if wm == nil {
		return
	}

	leaves := wm.collectLeaves()
	for _, leaf := range leaves {
		if leaf != nil && leaf.container != 0 {
			webkit.WidgetAddCSSClass(leaf.container, basePaneClass)
			if wm.hasMultiplePanes() {
				webkit.WidgetAddCSSClass(leaf.container, multiPaneClass)
			} else {
				webkit.WidgetRemoveCSSClass(leaf.container, multiPaneClass)
			}
		}
	}
}

func (wm *WorkspaceManager) focusByView(view *webkit.WebView) {
	if wm == nil || view == nil {
		return
	}

	// Throttle focus changes to prevent infinite loops
	wm.focusThrottleMutex.Lock()
	const focusThrottleInterval = 100 * time.Millisecond
	if time.Since(wm.lastFocusChange) < focusThrottleInterval {
		wm.focusThrottleMutex.Unlock()
		return
	}
	wm.lastFocusChange = time.Now()
	wm.focusThrottleMutex.Unlock()

	if node, ok := wm.viewToNode[view]; ok {
		if wm.active != node {
			wm.focusNode(node)
		}
	}
}

func (wm *WorkspaceManager) ensureHover(node *paneNode) {
	if wm == nil || node == nil || !node.isLeaf {
		return
	}
	if node.container == 0 || node.hoverToken != 0 {
		return
	}

	token := webkit.WidgetAddHoverHandler(node.container, func() {
		if wm == nil {
			return
		}
		wm.focusNode(node)
	})
	node.hoverToken = token
	if token == 0 {
		log.Printf("[workspace] failed to attach hover handler: widget=%#x", node.container)
	}
}

func (wm *WorkspaceManager) detachHover(node *paneNode) {
	if wm == nil || node == nil || node.hoverToken == 0 {
		return
	}
	webkit.WidgetRemoveHoverHandler(node.container, node.hoverToken)
	node.hoverToken = 0
}

// FocusNeighbor moves focus to the nearest pane in the requested direction using the
// actual widget geometry to determine adjacency.
func (wm *WorkspaceManager) FocusNeighbor(direction string) bool {
	if wm == nil {
		return false
	}
	switch strings.ToLower(direction) {
	case "left", "right", "up", "down":
		return wm.focusAdjacent(strings.ToLower(direction))
	default:
		return false
	}
}

const focusEpsilon = 1e-3

func (wm *WorkspaceManager) focusAdjacent(direction string) bool {
	if wm.active == nil || !wm.active.isLeaf || wm.active.container == 0 {
		return false
	}

	if neighbor := wm.structuralNeighbor(wm.active, direction); neighbor != nil {
		wm.focusNode(neighbor)
		return true
	}

	currentBounds, ok := webkit.WidgetGetBounds(wm.active.container)
	if !ok {
		log.Printf("[workspace] unable to compute bounds for active pane")
		return false
	}

	cx := currentBounds.X + currentBounds.Width/2.0
	cy := currentBounds.Y + currentBounds.Height/2.0

	leaves := wm.collectLeaves()
	bestScore := math.MaxFloat64
	var best *paneNode
	var debugCandidates []string

	for _, candidate := range leaves {
		if candidate == nil || candidate == wm.active || candidate.container == 0 {
			continue
		}
		bounds, ok := webkit.WidgetGetBounds(candidate.container)
		if !ok {
			continue
		}
		tx := bounds.X + bounds.Width/2.0
		ty := bounds.Y + bounds.Height/2.0

		dx := tx - cx
		dy := ty - cy

		var score float64
		switch direction {
		case "left":
			if dx >= -focusEpsilon {
				continue
			}
			score = math.Abs(dx)*1000 + math.Abs(dy)
		case "right":
			if dx <= focusEpsilon {
				continue
			}
			score = math.Abs(dx)*1000 + math.Abs(dy)
		case "up":
			if dy >= -focusEpsilon {
				continue
			}
			score = math.Abs(dy)*1000 + math.Abs(dx)
		case "down":
			if dy <= focusEpsilon {
				continue
			}
			score = math.Abs(dy)*1000 + math.Abs(dx)
		}

		if direction == "up" || direction == "down" {
			debugCandidates = append(debugCandidates, fmt.Sprintf("cand=%#x dx=%.2f dy=%.2f score=%.2f", candidate.container, dx, dy, score))
		}

		if score < bestScore {
			bestScore = score
			best = candidate
		}
	}

	if best != nil {
		wm.focusNode(best)
		return true
	}

	if len(debugCandidates) > 0 {
		log.Printf("[workspace] focusAdjacent no candidate direction=%s current=%#x candidates=%s", direction, wm.active.container, strings.Join(debugCandidates, "; "))
	}
	return false
}

func (wm *WorkspaceManager) structuralNeighbor(node *paneNode, direction string) *paneNode {
	if node == nil || node.container == 0 {
		return nil
	}

	refBounds, ok := webkit.WidgetGetBounds(node.container)
	if !ok {
		return nil
	}
	cx := refBounds.X + refBounds.Width/2.0
	cy := refBounds.Y + refBounds.Height/2.0
	axisVertical := direction == "up" || direction == "down"

	for parent := node.parent; parent != nil; parent = parent.parent {
		switch direction {
		case "up":
			if axisVertical && parent.orientation == webkit.OrientationVertical && parent.right == node {
				if leaf := wm.closestLeafFromSubtree(parent.left, cx, cy, direction); leaf != nil {
					return leaf
				}
			}
		case "down":
			if axisVertical && parent.orientation == webkit.OrientationVertical && parent.left == node {
				if leaf := wm.closestLeafFromSubtree(parent.right, cx, cy, direction); leaf != nil {
					return leaf
				}
			}
		case "left":
			if !axisVertical && parent.orientation == webkit.OrientationHorizontal && parent.right == node {
				if leaf := wm.closestLeafFromSubtree(parent.left, cx, cy, direction); leaf != nil {
					return leaf
				}
			}
		case "right":
			if !axisVertical && parent.orientation == webkit.OrientationHorizontal && parent.left == node {
				if leaf := wm.closestLeafFromSubtree(parent.right, cx, cy, direction); leaf != nil {
					return leaf
				}
			}
		}
		node = parent
	}
	return nil
}

func (wm *WorkspaceManager) closestLeafFromSubtree(node *paneNode, cx, cy float64, direction string) *paneNode {
	leaves := wm.collectLeavesFrom(node)
	bestScore := math.MaxFloat64
	var best *paneNode
	for _, leaf := range leaves {
		if leaf == nil || leaf.container == 0 {
			continue
		}
		bounds, ok := webkit.WidgetGetBounds(leaf.container)
		if !ok {
			continue
		}
		tx := bounds.X + bounds.Width/2.0
		ty := bounds.Y + bounds.Height/2.0
		dx := tx - cx
		dy := ty - cy
		var score float64
		switch direction {
		case "left":
			if dx >= -focusEpsilon {
				continue
			}
			score = math.Abs(dx)*1000 + math.Abs(dy)
		case "right":
			if dx <= focusEpsilon {
				continue
			}
			score = math.Abs(dx)*1000 + math.Abs(dy)
		case "up":
			if dy >= -focusEpsilon {
				continue
			}
			score = math.Abs(dy)*1000 + math.Abs(dx)
		case "down":
			if dy <= focusEpsilon {
				continue
			}
			score = math.Abs(dy)*1000 + math.Abs(dx)
		default:
			continue
		}
		if score < bestScore {
			bestScore = score
			best = leaf
		}
	}
	if best == nil {
		return wm.boundaryFallback(node, direction)
	}
	return best
}

func (wm *WorkspaceManager) boundaryFallback(node *paneNode, direction string) *paneNode {
	return wm.boundaryFallbackWithDepth(node, direction, 0)
}

func (wm *WorkspaceManager) boundaryFallbackWithDepth(node *paneNode, direction string, depth int) *paneNode {
	// Prevent infinite recursion - max tree depth should be reasonable
	const maxDepth = 50
	if depth > maxDepth {
		log.Printf("[workspace] boundaryFallback: max depth exceeded, possible tree corruption")
		return nil
	}

	if node == nil {
		return nil
	}
	if node.isLeaf {
		return node
	}
	switch direction {
	case "up":
		if leaf := wm.boundaryFallbackWithDepth(node.right, direction, depth+1); leaf != nil {
			return leaf
		}
		return wm.boundaryFallbackWithDepth(node.left, direction, depth+1)
	case "down":
		if leaf := wm.boundaryFallbackWithDepth(node.left, direction, depth+1); leaf != nil {
			return leaf
		}
		return wm.boundaryFallbackWithDepth(node.right, direction, depth+1)
	case "left":
		if leaf := wm.boundaryFallbackWithDepth(node.right, direction, depth+1); leaf != nil {
			return leaf
		}
		return wm.boundaryFallbackWithDepth(node.left, direction, depth+1)
	case "right":
		if leaf := wm.boundaryFallbackWithDepth(node.left, direction, depth+1); leaf != nil {
			return leaf
		}
		return wm.boundaryFallbackWithDepth(node.right, direction, depth+1)
	default:
		return nil
	}
}

func (wm *WorkspaceManager) collectLeaves() []*paneNode {
	return wm.collectLeavesFrom(wm.root)
}

func (wm *WorkspaceManager) collectLeavesFrom(node *paneNode) []*paneNode {
	var leaves []*paneNode
	visited := make(map[*paneNode]bool)

	var walk func(*paneNode, int)
	walk = func(n *paneNode, depth int) {
		// Prevent infinite recursion and cycles
		const maxDepth = 50
		if n == nil || depth > maxDepth {
			return
		}
		if visited[n] {
			log.Printf("[workspace] collectLeavesFrom: cycle detected in tree")
			return
		}
		visited[n] = true

		if n.isLeaf {
			leaves = append(leaves, n)
			return
		}
		walk(n.left, depth+1)
		walk(n.right, depth+1)
	}
	walk(node, 0)
	return leaves
}

func (wm *WorkspaceManager) createWebView() (*webkit.WebView, error) {
	if wm == nil || wm.createWebViewFn == nil {
		return nil, errors.New("workspace manager missing webview factory")
	}
	return wm.createWebViewFn()
}

func (wm *WorkspaceManager) createPane(view *webkit.WebView) (*BrowserPane, error) {
	if wm == nil || wm.createPaneFn == nil {
		return nil, errors.New("workspace manager missing pane factory")
	}
	return wm.createPaneFn(view)
}

func (wm *WorkspaceManager) splitNode(target *paneNode, direction string) (*paneNode, error) {
	if target == nil || !target.isLeaf || target.pane == nil {
		return nil, errors.New("split target must be a leaf pane")
	}

	newView, err := wm.createWebView()
	if err != nil {
		return nil, err
	}

	newPane, err := wm.createPane(newView)
	if err != nil {
		return nil, err
	}

	if handler := newPane.MessageHandler(); handler != nil {
		handler.SetWorkspaceObserver(wm)
	}

	newContainer := newPane.webView.RootWidget()
	if newContainer == 0 {
		return nil, errors.New("new pane missing container")
	}
	webkit.WidgetSetHExpand(newContainer, true)
	webkit.WidgetSetVExpand(newContainer, true)
	webkit.WidgetRealizeInContainer(newContainer)

	existingContainer := target.container
	if existingContainer == 0 {
		return nil, errors.New("existing pane missing container")
	}
	webkit.WidgetSetHExpand(existingContainer, true)
	webkit.WidgetSetVExpand(existingContainer, true)
	webkit.WidgetRealizeInContainer(existingContainer)

	orientation, existingFirst := mapDirection(direction)
	log.Printf("[workspace] splitting direction=%s orientation=%v existingFirst=%v target.parent=%p", direction, orientation, existingFirst, target.parent)

	paned := webkit.NewPaned(orientation)
	if paned == 0 {
		return nil, errors.New("failed to create GtkPaned")
	}
	webkit.WidgetSetHExpand(paned, true)
	webkit.WidgetSetVExpand(paned, true)
	webkit.PanedSetResizeStart(paned, true)
	webkit.PanedSetResizeEnd(paned, true)

	newLeaf := &paneNode{
		pane:      newPane,
		container: newContainer,
		isLeaf:    true,
	}
	split := &paneNode{
		parent:      target.parent,
		container:   paned,
		orientation: orientation,
		isLeaf:      false,
	}

	parent := split.parent

	// Detach existing container from its current GTK parent before inserting into new paned.
	if parent == nil {
		// Target is the root - remove it from the window and unparent it
		log.Printf("[workspace] removing existing container=%#x from window", existingContainer)
		if wm.window != nil {
			wm.window.SetChild(0)
		}
	} else if parent.container != 0 {
		// Target has a parent paned - unparent it from there
		log.Printf("[workspace] unparenting existing container=%#x from parent paned=%#x", existingContainer, parent.container)
		if parent.left == target {
			webkit.PanedSetStartChild(parent.container, 0)
		} else if parent.right == target {
			webkit.PanedSetEndChild(parent.container, 0)
		}
	}

	// Properly unparent the widget from GTK's perspective
	webkit.WidgetUnparent(existingContainer)
	if parent != nil && parent.container != 0 {
		webkit.WidgetQueueAllocate(parent.container)
	}

	// Set up the tree structure first
	if existingFirst {
		split.left = target
		split.right = newLeaf
	} else {
		split.left = newLeaf
		split.right = target
	}

	target.parent = split
	newLeaf.parent = split

	// Update tree root/parent references
	if parent == nil {
		wm.root = split
	} else {
		if parent.left == target {
			parent.left = split
		} else if parent.right == target {
			parent.right = split
		}
	}

	// Use idle callback to ensure widget unparenting is complete before re-parenting
	webkit.IdleAdd(func() bool {
		// Add both containers to the new paned
		if existingFirst {
			webkit.PanedSetStartChild(paned, existingContainer)
			webkit.PanedSetEndChild(paned, newContainer)
			log.Printf("[workspace] added existing=%#x as start child, new=%#x as end child", existingContainer, newContainer)
		} else {
			webkit.PanedSetStartChild(paned, newContainer)
			webkit.PanedSetEndChild(paned, existingContainer)
			log.Printf("[workspace] added new=%#x as start child, existing=%#x as end child", newContainer, existingContainer)
		}

		// Attach the new paned to its parent
		if parent == nil {
			if wm.window != nil {
				wm.window.SetChild(paned)
				webkit.WidgetQueueAllocate(paned)
				log.Printf("[workspace] paned set as window child: paned=%#x", paned)
			}
		} else {
			if parent.left == split {
				webkit.PanedSetStartChild(parent.container, paned)
			} else if parent.right == split {
				webkit.PanedSetEndChild(parent.container, paned)
			}
			webkit.WidgetQueueAllocate(parent.container)
			webkit.WidgetQueueAllocate(paned)
			log.Printf("[workspace] paned inserted into parent=%#x", parent.container)
		}

		webkit.WidgetShow(paned)
		return false // Run once
	})

	wm.viewToNode[newPane.webView] = newLeaf
	wm.ensureHover(newLeaf)
	wm.ensureHover(target)
	wm.app.panes = append(wm.app.panes, newPane)
	if newPane.zoomController != nil {
		newPane.zoomController.ApplyInitialZoom()
	}

	// Update CSS classes for all panes now that we have multiple panes
	wm.ensurePaneBaseClasses()

	webkit.IdleAdd(func() bool {
		if newContainer != 0 {
			webkit.WidgetShow(newContainer)
		}
		wm.focusNode(newLeaf)
		return false
	})

	return newLeaf, nil
}

func (wm *WorkspaceManager) clonePaneState(_ *paneNode, target *paneNode) {
	if wm == nil || target == nil {
		return
	}
	if target.pane == nil || target.pane.webView == nil {
		return
	}

	const blankURL = "about:blank"

	if err := target.pane.webView.LoadURL(blankURL); err != nil {
		log.Printf("[workspace] failed to prepare blank pane: %v", err)
	}

	// Wait for about:blank to load before opening omnibox
	target.pane.webView.RegisterURIChangedHandler(func(uri string) {
		if uri == blankURL {
			// Defer omnibox opening to allow page to fully initialize
			webkit.IdleAdd(func() bool {
				if injErr := target.pane.webView.InjectScript("window.__dumber_omnibox?.open('omnibox');"); injErr != nil {
					log.Printf("[workspace] failed to open omnibox: %v", injErr)
				}
				return false // Remove idle callback
			})
		}
	})
}

func (wm *WorkspaceManager) closeCurrentPane() {
	if wm == nil || wm.active == nil {
		return
	}
	if err := wm.closePane(wm.active); err != nil {
		log.Printf("[workspace] close current pane failed: %v", err)
	}
}

func (wm *WorkspaceManager) closePane(node *paneNode) error {
	if wm == nil || node == nil || !node.isLeaf {
		return errors.New("close target must be a leaf pane")
	}
	if node.pane == nil || node.pane.webView == nil {
		return errors.New("close target missing webview")
	}

	remaining := len(wm.app.panes)
	willBeLastPane := remaining <= 1
	if willBeLastPane && wm.root == node {
		log.Printf("[workspace] closing final pane; exiting browser")
		wm.detachHover(node)
		if err := node.pane.webView.Destroy(); err != nil {
			log.Printf("[workspace] failed to destroy webview: %v", err)
		}
		webkit.QuitMainLoop()
		return nil
	}

	parent := node.parent
	if parent == nil {
		// This is the root pane (no parent in tree structure)
		if node != wm.root {
			return errors.New("inconsistent state: node has no parent but is not root")
		}

		// Check if we can find a replacement root
		replacement := wm.findReplacementRoot(node)
		if replacement == nil {
			// No other panes exist, this is the final pane
			log.Printf("[workspace] closing final pane; exiting browser")
			wm.detachHover(node)
			if err := node.pane.webView.Destroy(); err != nil {
				log.Printf("[workspace] failed to destroy webview: %v", err)
			}
			webkit.QuitMainLoop()
			return nil
		}

		// We have a replacement - delegate root status and close this pane
		log.Printf("[workspace] delegating root status from node=%#x to replacement=%#x", node.container, replacement.container)

		// Clean up references first
		delete(wm.viewToNode, node.pane.webView)
		delete(wm.lastSplitMsg, node.pane.webView)
		delete(wm.lastExitMsg, node.pane.webView)

		for i, pane := range wm.app.panes {
			if pane == node.pane {
				wm.app.panes = append(wm.app.panes[:i], wm.app.panes[i+1:]...)
				break
			}
		}

		if wm.mainPane == node {
			wm.mainPane = nil
		}

		// Clear current active if it's the node being closed
		if wm.active == node {
			wm.active = nil
		}

		// Set replacement as new root
		wm.root = replacement
		replacement.parent = nil

		// Replace window child directly (GTK handles reparenting automatically)
		if wm.window != nil {
			wm.window.SetChild(replacement.container)
			if replacement.container != 0 {
				webkit.WidgetQueueAllocate(replacement.container)
				webkit.WidgetShow(replacement.container)
			}
		}

		// Focus a suitable pane
		focusTarget := wm.leftmostLeaf(replacement)
		if focusTarget != nil {
			wm.focusNode(focusTarget)
		}

		// Destroy the webview and detach hover AFTER rearranging hierarchy
		// Only destroy the webview if this is the final pane, otherwise just clean up
		wm.detachHover(node)
		if willBeLastPane {
			// This is the last pane, safe to destroy completely
			if err := node.pane.webView.Destroy(); err != nil {
				log.Printf("[workspace] failed to destroy webview: %v", err)
			}
		} else {
			// Multiple panes remain, don't destroy the window - just clean up the webview
			log.Printf("[workspace] skipping webview destruction to preserve window (panes remaining: %d)", remaining-1)
			// TODO: Add a method to destroy just the webview without the window
		}

		wm.updateMainPane()
		// Update CSS classes after pane count changes
		wm.ensurePaneBaseClasses()
		log.Printf("[workspace] root pane closed and delegated; panes remaining=%d", len(wm.app.panes))
		return nil
	}

	var sibling *paneNode
	if parent.left == node {
		sibling = parent.right
	} else {
		sibling = parent.left
	}
	if sibling == nil {
		return errors.New("pane close failed: missing sibling")
	}

	log.Printf("[workspace] closing pane: target=%#x parent=%#x sibling=%#x remaining=%d", node.container, parent.container, sibling.container, remaining)

	// Find focus target before modifying the tree structure
	focusTarget := wm.leftmostLeaf(sibling)
	if focusTarget == nil {
		focusTarget = wm.leftmostLeaf(wm.root)
	}

	// Clean up references first
	delete(wm.viewToNode, node.pane.webView)
	delete(wm.lastSplitMsg, node.pane.webView)
	delete(wm.lastExitMsg, node.pane.webView)

	for i, pane := range wm.app.panes {
		if pane == node.pane {
			wm.app.panes = append(wm.app.panes[:i], wm.app.panes[i+1:]...)
			break
		}
	}

	if wm.mainPane == node {
		wm.mainPane = nil
	}

	// Clear current active if it's the node being closed
	if wm.active == node {
		wm.active = nil
	}

	grand := parent.parent
	if grand == nil {
		// Parent is the root node. Promote sibling to become the new root.
		// The sibling can be either a leaf (when only 2 panes total) or a branch (when more panes exist)
		log.Printf("[workspace] promoting sibling to root: container=%#x, isLeaf=%v", sibling.container, sibling.isLeaf)
		wm.root = sibling
		sibling.parent = nil

		// Unparent the sibling from the paned first, then set it as window child
		if wm.window != nil {
			// GTK requires explicit unparenting before reparenting
			if parent.left == sibling {
				webkit.PanedSetStartChild(parent.container, 0)
			} else {
				webkit.PanedSetEndChild(parent.container, 0)
			}
			// Now we can safely set it as window child
			wm.window.SetChild(sibling.container)
			if sibling.container != 0 {
				webkit.WidgetQueueAllocate(sibling.container)
			}
		}
	} else {
		// Parent has a grandparent, so promote sibling to take parent's place
		log.Printf("[workspace] promoting sibling to parent's position: sibling=%#x grand=%#x", sibling.container, grand.container)

		// First unparent the sibling from its current parent to avoid GTK-CRITICAL errors
		if parent.container != 0 && !parent.isLeaf {
			if parent.left == sibling {
				webkit.PanedSetStartChild(parent.container, 0)
			} else if parent.right == sibling {
				webkit.PanedSetEndChild(parent.container, 0)
			}
		}

		// Now safely reparent the sibling to the grandparent
		sibling.parent = grand
		if grand.container != 0 && !grand.isLeaf {
			if grand.left == parent {
				grand.left = sibling
				webkit.PanedSetStartChild(grand.container, sibling.container)
			} else if grand.right == parent {
				grand.right = sibling
				webkit.PanedSetEndChild(grand.container, sibling.container)
			}
			webkit.WidgetQueueAllocate(grand.container)
		}
	}

	// Keep sibling subtree visible
	if sibling.container != 0 {
		webkit.WidgetShow(sibling.container)
	}

	// Clean up parent node references
	parent.left = nil
	parent.right = nil
	node.parent = nil

	// Focus the target pane
	if focusTarget != nil && focusTarget != node {
		wm.focusNode(focusTarget)
	}

	// Destroy the webview and detach hover AFTER all hierarchy changes are complete
	// Only destroy the webview if this is the final pane, otherwise just clean up
	wm.detachHover(node)
	if willBeLastPane {
		// This is the last pane, safe to destroy completely
		if err := node.pane.webView.Destroy(); err != nil {
			log.Printf("[workspace] failed to destroy webview: %v", err)
		}
	} else {
		// Multiple panes remain, don't destroy the window - just clean up the webview
		log.Printf("[workspace] skipping webview destruction to preserve window (panes remaining: %d)", remaining-1)
		// TODO: Add a method to destroy just the webview without the window
	}

	wm.updateMainPane()
	// Update CSS classes after pane count changes
	wm.ensurePaneBaseClasses()
	log.Printf("[workspace] pane closed; panes remaining=%d", len(wm.app.panes))
	return nil
}

func (wm *WorkspaceManager) leftmostLeaf(node *paneNode) *paneNode {
	for node != nil && !node.isLeaf {
		if node.left != nil {
			node = node.left
			continue
		}
		node = node.right
	}
	return node
}

// findReplacementRoot finds a suitable replacement when closing the current root pane
func (wm *WorkspaceManager) findReplacementRoot(excludeNode *paneNode) *paneNode {
	if wm == nil || wm.root == nil {
		return nil
	}

	// If root is being closed and there are other panes, find a replacement
	leaves := wm.collectLeaves()
	for _, leaf := range leaves {
		if leaf != excludeNode && leaf != nil && leaf.isLeaf {
			// Find the topmost ancestor that's not the current root
			current := leaf
			for current.parent != nil && current.parent != wm.root {
				current = current.parent
			}

			// If this leaf is a direct child of root, or root only has one subtree,
			// we can promote this subtree to be the new root
			if current.parent == wm.root {
				// If the sibling is being excluded, promote this subtree
				var sibling *paneNode
				if wm.root.left == current {
					sibling = wm.root.right
				} else {
					sibling = wm.root.left
				}

				if sibling == excludeNode {
					// The sibling is being closed, so promote this subtree
					return current
				}
			}

			// Otherwise, return the first suitable leaf
			return leaf
		}
	}

	return nil
}

func (wm *WorkspaceManager) updateMainPane() {
	if len(wm.app.panes) == 1 {
		if leaf := wm.viewToNode[wm.app.panes[0].webView]; leaf != nil {
			wm.mainPane = leaf
		}
		return
	}

	if wm.mainPane == nil || !wm.mainPane.isLeaf {
		if wm.active != nil && wm.active.isLeaf {
			wm.mainPane = wm.active
		}
	}
}

func (wm *WorkspaceManager) HandlePopup(source *webkit.WebView, url string) *webkit.WebView {
	log.Printf("[workspace] HandlePopup called for URL: %s", url)

	// Check for frame type markers added by WebKit layer
	isBlankTarget := strings.HasSuffix(url, "#__dumber_frame_blank")
	isPopupTarget := strings.HasSuffix(url, "#__dumber_frame_popup")

	// Clean the URL by removing our markers
	if isBlankTarget || isPopupTarget {
		if isBlankTarget {
			url = strings.TrimSuffix(url, "#__dumber_frame_blank")
			log.Printf("[workspace] Detected _blank target - will create regular pane for: %s", url)
		} else {
			url = strings.TrimSuffix(url, "#__dumber_frame_popup")
			log.Printf("[workspace] Detected popup target - will create popup pane for: %s", url)
		}
	}

	if wm == nil || source == nil {
		log.Printf("[workspace] HandlePopup: nil workspace manager or source - allowing native popup")
		return nil
	}

	node := wm.viewToNode[source]
	if node == nil {
		log.Printf("[workspace] popup from unknown webview - allowing native popup")
		return nil
	}

	// Note: HandlePopup is now obsolete - window.open is handled directly via JavaScript bypass

	cfg := wm.app.config
	if cfg == nil {
		log.Printf("[workspace] HandlePopup: nil config - allowing native popup")
		return nil
	}

	popCfg := cfg.Workspace.Popups
	log.Printf("[workspace] Popup config - OpenInNewPane: %v, Placement: %s", popCfg.OpenInNewPane, popCfg.Placement)

	if !popCfg.OpenInNewPane {
		log.Printf("[workspace] Popup creation disabled in config - allowing native popup")
		return nil
	}

	// Smart detection path: create temporary view and decide placement once type is known
	if popCfg.EnableSmartDetection {
		webkitCfg, err := wm.app.buildWebkitConfig()
		if err != nil {
			log.Printf("[workspace] failed to build webkit config: %v - allowing native popup", err)
			return nil
		}
		webkitCfg.CreateWindow = false
		// Create as related to avoid WindowFeatures crash; we'll decide final placement later
		newView, err := webkit.NewWebViewWithRelated(webkitCfg, source)
		if err != nil {
			log.Printf("[workspace] failed to create temp WebView: %v - allowing native popup", err)
			return nil
		}

		// Register detection callback
		newView.OnWindowTypeDetected(func(t webkit.WindowType, feat *webkit.WindowFeatures) {
			wm.RunOnUI(func() {
				wm.handleDetectedWindowType(node, newView, url, t, feat)
			})
		})

		// Fallback: if detection never fires, treat as popup as before
		go func() {
			time.Sleep(1500 * time.Millisecond)
			if newView != nil {
				wm.RunOnUI(func() {
					if wm.viewToNode[newView] == nil { // not yet placed
						wm.handleDetectedWindowType(node, newView, url, webkit.WindowTypePopup, nil)
					}
				})
			}
		}()

		return newView
	}

	// Legacy path preserved
	webkitCfg, err := wm.app.buildWebkitConfig()
	if err != nil {
		log.Printf("[workspace] failed to build webkit config: %v - allowing native popup", err)
		return nil
	}
	webkitCfg.CreateWindow = false
	newView, err := webkit.NewWebViewWithRelated(webkitCfg, source)
	if err != nil {
		log.Printf("[workspace] failed to create placeholder WebView: %v - allowing native popup", err)
		return nil
	}

	// Register workspace navigation shortcuts on popup WebView before creating pane
	// This ensures shortcuts are available immediately when the WebView is ready
	wm.registerWorkspaceShortcuts(newView)
	log.Printf("[workspace] Registered workspace shortcuts for new WebView")

	// Create a pane for the new WebView
	newPane, err := wm.createPane(newView)
	if err != nil {
		log.Printf("[workspace] failed to create popup pane: %v - allowing native popup", err)
		return nil
	}

	// Determine placement direction
	direction := strings.ToLower(popCfg.Placement)
	if direction == "" {
		direction = "right"
	}

	// Determine target node for splitting
	target := node
	if !popCfg.FollowPaneContext && wm.active != nil {
		target = wm.active
	}

	// Add the popup pane to the workspace using manual pane insertion
	if err := wm.insertPopupPane(target, newPane, direction); err != nil {
		log.Printf("[workspace] popup pane insertion failed: %v - allowing native popup", err)
		return nil
	}

	// Apply different behavior based on target type
	if isBlankTarget {
		log.Printf("[workspace] Treating _blank target as regular pane - no auto-close behavior")
		// For _blank targets, just ensure GUI - no popup-specific behavior
	} else {
		log.Printf("[workspace] Treating as popup pane - applying popup-specific behavior")
		// Mark as popup for auto-close handling (OAuth flows, etc.)
		newNode := wm.viewToNode[newView]
		if newNode != nil {
			newNode.isPopup = true
			log.Printf("[workspace] Marked pane as popup for auto-close handling")

			// Register close handler for popup auto-close on window.close()
			newView.RegisterCloseHandler(func() {
				log.Printf("[workspace] Popup requesting close via window.close()")
				// Look up the node at close time
				if node := wm.viewToNode[newView]; node != nil && node.isPopup {
					log.Printf("[workspace] Closing popup pane")
					// Brief delay to allow any final redirects to complete
					time.AfterFunc(200*time.Millisecond, func() {
						webkit.IdleAdd(func() bool {
							if err := wm.closePane(node); err != nil {
								log.Printf("[workspace] Failed to close popup pane: %v", err)
							}
							return false
						})
					})
				} else {
					log.Printf("[workspace] Could not find popup node for close handler")
				}
			})
		} else {
			log.Printf("[workspace] Warning: could not find node for popup WebView in viewToNode map")
		}
	}

	// Ensure GUI components are available in the new pane
	wm.ensureGUIInPane(newPane)

	// Inject GUI components into the popup pane
	wm.ensureGUIInPane(newPane)

	// Load the URL if provided
	if url != "" {
		paneType := "popup"
		if isBlankTarget {
			paneType = "_blank target"
		}
		log.Printf("[webkit] LoadURL (%s): %s", paneType, url)
		if err := newView.LoadURL(url); err != nil {
			log.Printf("[workspace] failed to load %s URL: %v", paneType, err)
		}
		// Ensure the WebView is visible after loading URL
		if err := newView.Show(); err != nil {
			log.Printf("[workspace] failed to show popup WebView: %v", err)
		}
	}

	if isBlankTarget {
		log.Printf("[workspace] Created regular pane for _blank target URL: %s", url)
	} else {
		log.Printf("[workspace] Created popup pane for URL: %s", url)
	}
	return newView
}

// registerOAuthAutoClose sets up OAuth auto-close functionality for popups
// Note: OAuth detection is now handled by the main-world.js injection script
func (wm *WorkspaceManager) registerOAuthAutoClose(view *webkit.WebView, url string) {
	log.Printf("[workspace] OAuth auto-close enabled for popup with URL: %s", url)
	log.Printf("[workspace] OAuth detection will be handled by main-world.js injection script")
}

func (wm *WorkspaceManager) applyWindowFeatures(view *webkit.WebView, intent *messaging.WindowIntent, isPopup bool) {
	if intent == nil {
		return
	}

	features := &webkit.WindowFeatures{}

	// Apply dimensions if specified
	if intent.Width != nil {
		features.Width = *intent.Width
	}
	if intent.Height != nil {
		features.Height = *intent.Height
	}

	// Apply visibility features based on window type
	defaultToolbar := !isPopup
	defaultLocation := !isPopup
	defaultMenubar := !isPopup

	if intent.Toolbar != nil {
		features.ToolbarVisible = *intent.Toolbar
	} else {
		features.ToolbarVisible = defaultToolbar
	}

	if intent.Location != nil {
		features.LocationbarVisible = *intent.Location
	} else {
		features.LocationbarVisible = defaultLocation
	}

	if intent.Menubar != nil {
		features.MenubarVisible = *intent.Menubar
	} else {
		features.MenubarVisible = defaultMenubar
	}

	if intent.Resizable != nil {
		features.Resizable = *intent.Resizable
	} else {
		features.Resizable = true // Usually resizable unless explicitly disabled
	}

	view.SetWindowFeatures(features)
	windowTypeStr := "tab"
	if isPopup {
		windowTypeStr = "popup"
	}
	log.Printf("[workspace] Applied %s window features from intent: size=%dx%d, toolbar=%t, location=%t, menubar=%t, resizable=%t",
		windowTypeStr, features.Width, features.Height, features.ToolbarVisible, features.LocationbarVisible, features.MenubarVisible, features.Resizable)
}

func (wm *WorkspaceManager) handleIntentAsTab(sourceNode *paneNode, url string, intent *messaging.WindowIntent) *webkit.WebView {
	log.Printf("[workspace] Handling intent as tab: %s", url)

	webkitCfg, err := wm.app.buildWebkitConfig()
	if err != nil {
		log.Printf("[workspace] failed to build webkit config: %v - allowing native popup", err)
		return nil
	}
	webkitCfg.CreateWindow = false

	newView, err := webkit.NewWebView(webkitCfg)
	if err != nil {
		log.Printf("[workspace] failed to create tab WebView: %v - allowing native popup", err)
		return nil
	}

	newPane, err := wm.createPane(newView)
	if err != nil {
		log.Printf("[workspace] failed to create tab pane: %v - allowing native popup", err)
		return nil
	}

	direction := strings.ToLower(wm.app.config.Workspace.Popups.Placement)
	if direction == "" {
		direction = "right"
	}

	if err := wm.insertPopupPane(sourceNode, newPane, direction); err != nil {
		log.Printf("[workspace] tab pane insertion failed: %v - allowing native popup", err)
		return nil
	}

	node := wm.viewToNode[newView]
	if node != nil {
		node.windowType = webkit.WindowTypeTab
		node.isRelated = false

		// Apply window features from JavaScript intent
		wm.applyWindowFeatures(newView, intent, false)
	}

	wm.ensureGUIInPane(newPane)

	if url != "" {
		if err := newView.LoadURL(url); err != nil {
			log.Printf("[workspace] failed to load tab URL: %v", err)
		}
		if err := newView.Show(); err != nil {
			log.Printf("[workspace] failed to show tab WebView: %v", err)
		}
	}

	log.Printf("[workspace] Created tab pane for URL: %s", url)
	return newView
}

// handleIntentAsPopup creates a related popup pane based on window.open intent
func (wm *WorkspaceManager) handleIntentAsPopup(sourceNode *paneNode, url string, intent *messaging.WindowIntent) *webkit.WebView {
	log.Printf("[workspace] Handling intent as popup: %s", url)

	webkitCfg, err := wm.app.buildWebkitConfig()
	if err != nil {
		log.Printf("[workspace] failed to build webkit config: %v - allowing native popup", err)
		return nil
	}
	webkitCfg.CreateWindow = false

	newView, err := webkit.NewWebViewWithRelated(webkitCfg, sourceNode.pane.webView)
	if err != nil {
		log.Printf("[workspace] failed to create popup WebView: %v - allowing native popup", err)
		return nil
	}

	// Log the parent-popup WebView ID relationship for OAuth auto-close
	parentWebViewID := sourceNode.pane.webView.ID()
	popupWebViewID := newView.ID()
	log.Printf("[workspace] Created popup WebView: parentID=%s popupID=%s url=%s", parentWebViewID, popupWebViewID, url)

	// Store popup WebView ID in parent's localStorage for OAuth callback lookup
	storeScript := fmt.Sprintf(`
		try {
			const parentWebViewId = '%s';
			const popupWebViewId = '%s';
			const popupMapping = {
				parentId: parentWebViewId,
				popupId: popupWebViewId,
				timestamp: Date.now(),
				url: '%s'
			};
			localStorage.setItem('popup_mapping_' + parentWebViewId, JSON.stringify(popupMapping));
			console.log('[workspace] Stored popup mapping:', popupMapping);
		} catch(e) {
			console.warn('[workspace] Failed to store popup mapping:', e);
		}
	`, parentWebViewID, popupWebViewID, url)

	// Inject into parent WebView so it can find its popup later
	if err := sourceNode.pane.webView.InjectScript(storeScript); err != nil {
		log.Printf("[workspace] Failed to inject popup mapping script into parent: %v", err)
	}

	newPane, err := wm.createPane(newView)
	if err != nil {
		log.Printf("[workspace] failed to create popup pane: %v - allowing native popup", err)
		return nil
	}

	direction := strings.ToLower(wm.app.config.Workspace.Popups.Placement)
	if direction == "" {
		direction = "right"
	}

	if err := wm.insertPopupPane(sourceNode, newPane, direction); err != nil {
		log.Printf("[workspace] popup pane insertion failed: %v - allowing native popup", err)
		return nil
	}

	node := wm.viewToNode[newView]
	var requestID string
	if node != nil {
		node.windowType = webkit.WindowTypePopup
		node.isRelated = true
		node.parentPane = sourceNode
		node.isPopup = true
		node.autoClose = wm.shouldAutoClose(url)

		// Store requestID for deduplication cleanup
		if intent != nil {
			requestID = intent.RequestID
		}

		// Apply window features from JavaScript intent
		wm.applyWindowFeatures(newView, intent, true)
	}

	// Register close handler for popup auto-close
	newView.RegisterCloseHandler(func() {
		log.Printf("[workspace] Popup requesting close via window.close()")

		// Clear the RequestID from deduplicator to allow new popups with same ID
		if requestID != "" && wm.paneDeduplicator != nil {
			wm.paneDeduplicator.ClearRequestID(requestID)
		}

		if n := wm.viewToNode[newView]; n != nil && n.isPopup {
			time.AfterFunc(200*time.Millisecond, func() {
				webkit.IdleAdd(func() bool {
					if err := wm.closePane(n); err != nil {
						log.Printf("[workspace] Failed to close popup pane: %v", err)
					}
					return false
				})
			})
		}
	})

	// URL-based auto-close for OAuth popups
	if node != nil && node.isPopup && node.autoClose {
		wm.registerOAuthAutoClose(newView, url)
	}

	wm.ensureGUIInPane(newPane)

	if url != "" {
		if err := newView.LoadURL(url); err != nil {
			log.Printf("[workspace] failed to load popup URL: %v", err)
		}
		if err := newView.Show(); err != nil {
			log.Printf("[workspace] failed to show popup WebView: %v", err)
		}
	}

	log.Printf("[workspace] Created popup pane for URL: %s", url)
	return newView
}

// insertIndependentPane inserts a new independent pane next to the source
func (wm *WorkspaceManager) insertIndependentPane(sourceNode *paneNode, webView *webkit.WebView, url string) error {
	newPane, err := wm.createPane(webView)
	if err != nil {
		return err
	}
	direction := strings.ToLower(wm.app.config.Workspace.Popups.Placement)
	if direction == "" {
		direction = "right"
	}
	if err := wm.insertPopupPane(sourceNode, newPane, direction); err != nil { // reuse insertion primitive
		return err
	}
	node := wm.viewToNode[webView]
	if node != nil {
		node.windowType = webkit.WindowTypeTab
		node.isRelated = false
	}
	if url != "" {
		_ = webView.LoadURL(url)
	}
	return nil
}

// configureRelatedPopup creates a related view and inserts it
func (wm *WorkspaceManager) configureRelatedPopup(sourceNode *paneNode, webView *webkit.WebView, url string, feat *webkit.WindowFeatures) {
	// Use the WebView that was already created and returned to WebKit
	related := webView
	newPane, err := wm.createPane(related)
	if err != nil {
		log.Printf("[workspace] failed to create related popup pane: %v", err)
		return
	}
	direction := strings.ToLower(wm.app.config.Workspace.Popups.Placement)
	if direction == "" {
		direction = "right"
	}
	if err := wm.insertPopupPane(sourceNode, newPane, direction); err != nil {
		log.Printf("[workspace] failed to insert related popup pane: %v", err)
		return
	}
	node := wm.viewToNode[related]
	if node != nil {
		node.windowType = webkit.WindowTypePopup
		node.windowFeatures = feat
		node.isRelated = true
		node.parentPane = sourceNode
		node.isPopup = true
		// Heuristic + config for auto-close intent
		node.autoClose = wm.shouldAutoClose(url)
	}
	// Pipe into existing auto-close flow only for popups (confirmed by detection)
	related.RegisterCloseHandler(func() {
		log.Printf("[workspace] Popup requesting close via window.close()")
		if n := wm.viewToNode[related]; n != nil && n.isPopup {
			time.AfterFunc(200*time.Millisecond, func() {
				webkit.IdleAdd(func() bool {
					if err := wm.closePane(n); err != nil {
						log.Printf("[workspace] Failed to close popup pane: %v", err)
					}
					return false
				})
			})
		}
	})

	// URL-based fallback: if providers don't call window.close(), auto-close on OAuth callback URLs
	if node != nil && node.isPopup && node.autoClose {
		wm.registerOAuthAutoClose(related, url)
	}
	if url != "" {
		_ = related.LoadURL(url)
	}
}

// shouldAutoClose checks simple OAuth-like URL patterns and config flag
func (wm *WorkspaceManager) shouldAutoClose(url string) bool {
	log.Printf("[workspace] shouldAutoClose called for URL: %s", url)

	if wm == nil || wm.app == nil || wm.app.config == nil {
		log.Printf("[workspace] shouldAutoClose: missing config, returning true")
		return true
	}
	if !wm.app.config.Workspace.Popups.OAuthAutoClose {
		log.Printf("[workspace] shouldAutoClose: OAuthAutoClose disabled in config, returning false")
		return false
	}

	u := strings.ToLower(url)
	log.Printf("[workspace] shouldAutoClose: checking lowercase URL: %s", u)

	// RFC 6749 compliant OAuth 2.0 URL patterns
	oauthPatterns := []string{
		// Standard OAuth endpoints
		"oauth", "authorize", "authorization",
		// Standard callback/redirect patterns
		"callback", "redirect", "auth/callback",
		// OpenID Connect patterns
		"oidc", "openid",
		// Common OAuth parameter indicators
		"response_type=", "client_id=", "redirect_uri=", "scope=", "state=",
		// Standard OAuth response parameters
		"code=", "access_token=", "id_token=", "token_type=",
		// Error response parameters
		"error=", "error_description=",
	}

	// Check for OAuth patterns in URL
	for _, pattern := range oauthPatterns {
		if strings.Contains(u, pattern) {
			log.Printf("[workspace] shouldAutoClose: MATCHED pattern '%s' in URL, returning true", pattern)
			return true
		}
	}

	log.Printf("[workspace] shouldAutoClose: no OAuth patterns matched, returning false")
	return false
}

// isPopupVerificationPage determines if a URL is a popup verification page that should redirect instead of close

// RunOnUI schedules a function; here simply executes inline as GTK main loop is single-threaded
func (wm *WorkspaceManager) RunOnUI(fn func()) {
	if fn != nil {
		fn()
	}
}

// handleDetectedWindowType handles window type detection from smart detection path
func (wm *WorkspaceManager) handleDetectedWindowType(sourceNode *paneNode, webView *webkit.WebView, url string, windowType webkit.WindowType, features *webkit.WindowFeatures) {
	if wm.viewToNode[webView] != nil {
		return // Already placed
	}

	log.Printf("[workspace] Smart detection result: type=%d url=%s", windowType, url)

	switch windowType {
	case webkit.WindowTypeTab:
		// For tabs, create a NEW independent WebView (can't use the related one)
		webkitCfg, err := wm.app.buildWebkitConfig()
		if err != nil {
			log.Printf("[workspace] failed to build webkit config for tab: %v", err)
			return
		}
		webkitCfg.CreateWindow = false

		// Create independent WebView like handleIntentAsTab does
		independentView, err := webkit.NewWebView(webkitCfg)
		if err != nil {
			log.Printf("[workspace] failed to create independent tab WebView: %v", err)
			return
		}

		// The related webView was just for detection - we don't use it for tabs
		// Insert the new independent view as a tab
		if err := wm.insertIndependentPane(sourceNode, independentView, url); err != nil {
			log.Printf("[workspace] Failed to insert independent pane: %v", err)
		}

	case webkit.WindowTypePopup:
		// For popups, use the related WebView we already created
		wm.configureRelatedPopup(sourceNode, webView, url, features)
	default:
		// Fallback to popup behavior
		wm.configureRelatedPopup(sourceNode, webView, url, features)
	}
}

// insertPopupPane inserts a pre-created popup pane into the workspace
func (wm *WorkspaceManager) insertPopupPane(target *paneNode, newPane *BrowserPane, direction string) error {
	if target == nil || !target.isLeaf || target.pane == nil {
		return errors.New("insert target must be a leaf pane")
	}

	if newPane == nil || newPane.webView == nil {
		return errors.New("new pane missing webview")
	}

	if handler := newPane.MessageHandler(); handler != nil {
		handler.SetWorkspaceObserver(wm)
	}

	newContainer := newPane.webView.RootWidget()
	if newContainer == 0 {
		return errors.New("new pane missing container")
	}

	webkit.WidgetSetHExpand(newContainer, true)
	webkit.WidgetSetVExpand(newContainer, true)
	webkit.WidgetRealizeInContainer(newContainer)

	// Also realize the WebView widget itself for proper popup rendering
	webViewWidget := newPane.webView.Widget()
	if webViewWidget != 0 {
		webkit.WidgetRealizeInContainer(webViewWidget)
	}

	existingContainer := target.container
	if existingContainer == 0 {
		return errors.New("existing pane missing container")
	}

	orientation, existingFirst := mapDirection(direction)
	log.Printf("[workspace] inserting popup direction=%s orientation=%v existingFirst=%v target.parent=%p", direction, orientation, existingFirst, target.parent)

	paned := webkit.NewPaned(orientation)
	if paned == 0 {
		return errors.New("failed to create GtkPaned")
	}
	webkit.WidgetSetHExpand(paned, true)
	webkit.WidgetSetVExpand(paned, true)
	webkit.PanedSetResizeStart(paned, true)
	webkit.PanedSetResizeEnd(paned, true)

	newLeaf := &paneNode{
		pane:      newPane,
		container: newContainer,
		isLeaf:    true,
	}
	split := &paneNode{
		parent:      target.parent,
		container:   paned,
		orientation: orientation,
		isLeaf:      false,
	}

	parent := split.parent

	// Detach existing container from its current GTK parent before inserting into new paned.
	if parent == nil {
		// Target is the root - remove it from the window
		log.Printf("[workspace] removing existing container=%#x from window", existingContainer)
		if wm.window != nil {
			wm.window.SetChild(0)
		}
	} else if parent.container != 0 {
		// Target has a parent paned - unparent it from there
		log.Printf("[workspace] unparenting existing container=%#x from parent paned=%#x", existingContainer, parent.container)
		if parent.left == target {
			webkit.PanedSetStartChild(parent.container, 0)
		} else if parent.right == target {
			webkit.PanedSetEndChild(parent.container, 0)
		}
		webkit.WidgetQueueAllocate(parent.container)
	}

	// Add both containers to the new paned
	if existingFirst {
		split.left = target
		split.right = newLeaf
		webkit.PanedSetStartChild(paned, existingContainer)
		webkit.PanedSetEndChild(paned, newContainer)
		log.Printf("[workspace] added existing=%#x as start child, new=%#x as end child", existingContainer, newContainer)
	} else {
		split.left = newLeaf
		split.right = target
		webkit.PanedSetStartChild(paned, newContainer)
		webkit.PanedSetEndChild(paned, existingContainer)
		log.Printf("[workspace] added new=%#x as start child, existing=%#x as end child", newContainer, existingContainer)
	}

	target.parent = split
	newLeaf.parent = split

	if parent == nil {
		wm.root = split
		if wm.window != nil {
			wm.window.SetChild(paned)
			webkit.WidgetQueueAllocate(paned)
			log.Printf("[workspace] paned set as window child: paned=%#x", paned)
		}
	} else {
		if parent.left == target {
			parent.left = split
			webkit.PanedSetStartChild(parent.container, paned)
		} else if parent.right == target {
			parent.right = split
			webkit.PanedSetEndChild(parent.container, paned)
		}
		webkit.WidgetQueueAllocate(parent.container)
		webkit.WidgetQueueAllocate(paned)
		log.Printf("[workspace] paned inserted into parent=%#x", parent.container)
	}

	webkit.WidgetShow(paned)

	wm.viewToNode[newPane.webView] = newLeaf
	wm.ensureHover(newLeaf)
	wm.ensureHover(target)
	wm.app.panes = append(wm.app.panes, newPane)
	if newPane.zoomController != nil {
		newPane.zoomController.ApplyInitialZoom()
	}

	// Update CSS classes for all panes now that we have multiple panes
	wm.ensurePaneBaseClasses()

	webkit.IdleAdd(func() bool {
		if newContainer != 0 {
			webkit.WidgetShow(newContainer)
		}
		wm.focusNode(newLeaf)
		return false
	})

	return nil
}

// ensureGUIInPane lazily loads GUI components into a pane when it gains focus
// ensureGUIInPane is now a no-op since GUI is injected globally by WebKit
// This prevents duplicate GUI injection that was causing duplicate log messages
func (wm *WorkspaceManager) ensureGUIInPane(pane *BrowserPane) {
	if pane == nil {
		return
	}

	// GUI is already injected globally via WebKit's enableUserContentManager
	// Just mark the pane as having GUI to prevent unnecessary calls
	if !pane.HasGUI() {
		pane.SetHasGUI(true)
		pane.SetGUIComponent("manager", true)
		pane.SetGUIComponent("omnibox", true)
	}
}

func mapDirection(direction string) (webkit.Orientation, bool) {
	switch direction {
	case "left":
		return webkit.OrientationHorizontal, false
	case "up":
		return webkit.OrientationVertical, false
	case "down":
		return webkit.OrientationVertical, true
	default:
		return webkit.OrientationHorizontal, true
	}
}
