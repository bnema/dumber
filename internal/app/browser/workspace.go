package browser

import (
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/bnema/dumber/internal/app/messaging"
	"github.com/bnema/dumber/pkg/webkit"
)

// IDManager provides human-readable sequential IDs for debugging
type IDManager struct {
	mu        sync.Mutex
	webViewCounter int
	paneCounter    int
	widgetCounter  int

	// Maps from original IDs to readable IDs
	webViewIDs map[string]int    // webkit ID -> sequential number
	paneIDs    map[string]int    // pane ID -> sequential number
	widgetIDs  map[uintptr]int   // widget pointer -> sequential number

	// Reverse maps for lookups
	webViewByNum map[int]string
	paneByNum    map[int]string
	widgetByNum  map[int]uintptr
}

// NewIDManager creates a new ID manager
func NewIDManager() *IDManager {
	return &IDManager{
		webViewIDs:   make(map[string]int),
		paneIDs:      make(map[string]int),
		widgetIDs:    make(map[uintptr]int),
		webViewByNum: make(map[int]string),
		paneByNum:    make(map[int]string),
		widgetByNum:  make(map[int]uintptr),
	}
}

// GetWebViewID returns a human-readable ID for a WebView
func (im *IDManager) GetWebViewID(webkitID string) int {
	im.mu.Lock()
	defer im.mu.Unlock()

	if id, exists := im.webViewIDs[webkitID]; exists {
		return id
	}

	im.webViewCounter++
	im.webViewIDs[webkitID] = im.webViewCounter
	im.webViewByNum[im.webViewCounter] = webkitID
	return im.webViewCounter
}

// GetPaneID returns a human-readable ID for a pane
func (im *IDManager) GetPaneID(paneID string) int {
	im.mu.Lock()
	defer im.mu.Unlock()

	if id, exists := im.paneIDs[paneID]; exists {
		return id
	}

	im.paneCounter++
	im.paneIDs[paneID] = im.paneCounter
	im.paneByNum[im.paneCounter] = paneID
	return im.paneCounter
}

// GetWidgetID returns a human-readable ID for a widget pointer
func (im *IDManager) GetWidgetID(widgetPtr uintptr) int {
	im.mu.Lock()
	defer im.mu.Unlock()

	if id, exists := im.widgetIDs[widgetPtr]; exists {
		return id
	}

	im.widgetCounter++
	im.widgetIDs[widgetPtr] = im.widgetCounter
	im.widgetByNum[im.widgetCounter] = widgetPtr
	return im.widgetCounter
}

// FormatWebView formats a WebView for logging with readable ID
func (im *IDManager) FormatWebView(view *webkit.WebView) string {
	if view == nil {
		return "webview:nil"
	}
	webkitID := view.ID()
	readableID := im.GetWebViewID(webkitID)
	return fmt.Sprintf("webview:%d", readableID)
}

// FormatPane formats a pane for logging with readable ID
func (im *IDManager) FormatPane(paneID string) string {
	if paneID == "" {
		return "pane:nil"
	}
	readableID := im.GetPaneID(paneID)
	return fmt.Sprintf("pane:%d", readableID)
}

// FormatWidget formats a widget pointer for logging with readable ID
func (im *IDManager) FormatWidget(widgetPtr uintptr) string {
	if widgetPtr == 0 {
		return "widget:nil"
	}
	readableID := im.GetWidgetID(widgetPtr)
	return fmt.Sprintf("widget:%d", readableID)
}

// WorkspaceManager coordinates all workspace operations through specialized managers
type WorkspaceManager struct {
	// Core references
	app    *BrowserApp
	window *webkit.Window

	// Specialized managers
	layoutManager      *LayoutManager
	widgetManager      *WidgetManager
	focusManager       *FocusManager
	stackedPaneManager *StackedPaneManager
	closeManager       *CloseManager
	cssManager         *CSSManager

	// Core state
	root             *paneNode
	mainPane         *paneNode
	currentlyFocused *paneNode
	viewToNode       map[*webkit.WebView]*paneNode

	// Anti-spam protection
	lastSplitMsg     map[*webkit.WebView]time.Time
	lastExitMsg      map[*webkit.WebView]time.Time
	paneDeduplicator *messaging.PaneRequestDeduplicator

	// Pane mode coordination
	paneModeActive   bool
	splitting        bool
	paneModeSource   *webkit.WebView
	lastPaneModeEntry time.Time
	paneMutex        sync.Mutex

	// Focus throttling
	lastFocusChange    time.Time
	focusThrottleMutex sync.Mutex

	// Stack operation timing
	lastStackOperation time.Time

	// Widget management
	widgetMutex    sync.Mutex
	widgetRegistry *WidgetRegistry

	// Factory functions
	createWebViewFn func() (*webkit.WebView, error)
	createPaneFn    func(*webkit.WebView) (*BrowserPane, error)

	// ID management for human-readable debugging
	idManager *IDManager
}

// NewWorkspaceManager creates a new workspace manager with all specialized managers
func NewWorkspaceManager(app *BrowserApp, rootPane *BrowserPane) *WorkspaceManager {
	wm := &WorkspaceManager{
		app:              app,
		window:           rootPane.webView.Window(),
		viewToNode:       make(map[*webkit.WebView]*paneNode),
		lastSplitMsg:     make(map[*webkit.WebView]time.Time),
		lastExitMsg:      make(map[*webkit.WebView]time.Time),
		paneDeduplicator: messaging.NewPaneRequestDeduplicator(),
		widgetRegistry:   NewWidgetRegistry(),
		idManager:        NewIDManager(),
	}

	// Initialize all managers
	wm.layoutManager = NewLayoutManager(wm)
	wm.widgetManager = NewWidgetManager(wm)
	wm.focusManager = NewFocusManager(wm)
	wm.stackedPaneManager = NewStackedPaneManager(wm)
	wm.closeManager = NewCloseManager(wm)
	wm.cssManager = NewCSSManager(wm)

	// Set up factory functions
	wm.createWebViewFn = func() (*webkit.WebView, error) {
		if wm.app == nil {
			return nil, errors.New("workspace manager missing app reference")
		}
		cfg, err := wm.app.buildWebkitConfig()
		if err != nil {
			return nil, err
		}
		cfg.CreateWindow = true
		return webkit.NewWebView(cfg)
	}

	wm.createPaneFn = func(view *webkit.WebView) (*BrowserPane, error) {
		if wm.app == nil {
			return nil, errors.New("workspace manager missing app reference")
		}
		return wm.app.createPaneForView(view)
	}

	// Initialize CSS
	wm.cssManager.EnsureStyles()

	// Create and initialize root node
	rootContainer := rootPane.webView.RootWidget()
	root := &paneNode{
		pane:   rootPane,
		isLeaf: true,
	}

	// Initialize widgets and metadata
	wm.widgetManager.InitializePaneWidgets(root, rootContainer)
	wm.initializePaneMetadata(root, PaneTypeRegular)

	// Set up workspace state
	wm.root = root
	wm.mainPane = root
	wm.viewToNode[rootPane.webView] = root
	wm.ensureHover(root)

	return wm
}

// OnWorkspaceMessage implements messaging.WorkspaceObserver
func (wm *WorkspaceManager) OnWorkspaceMessage(source *webkit.WebView, msg messaging.Message) {
	// Handle messages directly based on type and action, avoiding repetitive HandleMessage methods
	switch msg.Type {
	case "workspace":
		// For workspace type messages, route based on Event field
		switch msg.Event {
		case "pane-split":
			wm.handleSplitRequest(source, msg)
		case "pane-close":
			wm.handleCloseRequest(source, msg)
		case "pane-stack":
			wm.handleStackRequest(source, msg)
		case "pane-focus":
			wm.handleFocusRequest(source, msg)
		case "pane-popup":
			wm.handlePopupRequest(source, msg)
		case "pane-mode-entered", "pane-mode-exited":
			// These are informational events, just log them
			log.Printf("[workspace] Pane mode event: %s", msg.Event)
		default:
			log.Printf("Unknown workspace event: %s", msg.Event)
		}
	case "pane":
		switch msg.Action {
		case "split":
			wm.handleSplitRequest(source, msg)
		case "close":
			wm.handleCloseRequest(source, msg)
		default:
			log.Printf("Unknown pane action: %s", msg.Action)
		}
	case "focus":
		wm.handleFocusRequest(source, msg)
	case "stack":
		wm.handleStackRequest(source, msg)
	case "popup":
		wm.handlePopupRequest(source, msg)
	default:
		log.Printf("Unknown workspace message type: %s", msg.Type)
	}
}

// Message handler methods - specific and focused instead of generic HandleMessage

func (wm *WorkspaceManager) handleSplitRequest(source *webkit.WebView, msg messaging.Message) {
	if wm.layoutManager == nil {
		return
	}

	// Parse direction and delegate to layout manager's specific method
	direction := msg.Direction
	if direction == "" {
		direction = "right" // default
	}

	// Find the source pane node
	_, exists := wm.viewToNode[source]
	if !exists {
		log.Printf("[workspace] Source WebView not found for split request: %s", wm.idManager.FormatWebView(source))
		return
	}

	// Check if this pane is already being split (prevent duplicate operations)
	if wm.splitting && wm.paneModeSource == source {
		log.Printf("[workspace] Ignoring duplicate split request from %s (already splitting)", wm.idManager.FormatWebView(source))
		return
	}

	// Mark as splitting to prevent duplicates
	wm.splitting = true
	wm.paneModeSource = source

	log.Printf("[workspace] Processing split request: direction=%s source=%s", direction, wm.idManager.FormatWebView(source))

	// Delegate to layout manager
	if err := wm.layoutManager.SplitPane(source, direction); err != nil {
		log.Printf("[workspace] Split failed: %v", err)
	}

	// Clear splitting state after operation
	wm.splitting = false
	wm.paneModeSource = nil
}

func (wm *WorkspaceManager) handleCloseRequest(source *webkit.WebView, msg messaging.Message) {
	if wm.closeManager == nil {
		return
	}

	// Find the pane node for this WebView
	node, exists := wm.viewToNode[source]
	if !exists {
		log.Printf("[workspace] WebView not found for close request: %s", wm.idManager.FormatWebView(source))
		return
	}

	wm.closeManager.ClosePane(node)
}

func (wm *WorkspaceManager) handleFocusRequest(source *webkit.WebView, msg messaging.Message) {
	if wm.focusManager == nil {
		return
	}
	direction := msg.Direction
	if direction != "" {
		wm.focusManager.FocusDirection(direction)
	} else {
		wm.focusManager.FocusPane(source)
	}
}

func (wm *WorkspaceManager) handleStackRequest(source *webkit.WebView, msg messaging.Message) {
	if wm.stackedPaneManager == nil {
		return
	}
	action := msg.Action
	switch action {
	case "navigate":
		direction := msg.Direction
		wm.stackedPaneManager.NavigateStack(direction)
	case "stack-pane":
		wm.stackedPaneManager.CreateStack(source)
	default:
		log.Printf("Unknown stack action: %s", action)
	}
}

func (wm *WorkspaceManager) handlePopupRequest(source *webkit.WebView, msg messaging.Message) {
	if wm.layoutManager == nil {
		return
	}
	targetURL := msg.URL
	wm.layoutManager.CreatePopup(source, targetURL)
}

// initializePaneMetadata creates and initializes metadata for a pane node
func (wm *WorkspaceManager) initializePaneMetadata(node *paneNode, paneType PaneType) {
	if node == nil {
		return
	}

	// Generate ID based on the pane
	var paneID string
	if node.pane != nil && node.pane.webView != nil {
		paneID = node.pane.webView.ID()
	} else {
		paneID = fmt.Sprintf("pane_%p_%d", node, time.Now().UnixNano())
	}

	node.metadata = NewPaneMetadata(paneID, paneType)

	// Set initial state based on pane type
	if node.isLeaf && node.pane != nil {
		node.metadata.SetState(PaneStateActive)
		if node.pane.webView != nil {
			node.metadata.SetURL(node.pane.webView.GetURI())
			node.metadata.SetTitle(node.pane.webView.GetTitle())
		}
	} else {
		node.metadata.SetState(PaneStateInactive)
	}

	// Set stack-specific metadata
	if node.isStacked {
		node.metadata.StackSize = len(node.stackedPanes)
		node.metadata.StackIndex = node.activeStackIndex
	}

	log.Printf("[metadata] Initialized metadata for %s (type: %s, state: %s)",
		wm.idManager.FormatPane(node.metadata.ID), node.metadata.Type, node.metadata.State)
}

// Manager access methods
func (wm *WorkspaceManager) Layout() *LayoutManager        { return wm.layoutManager }
func (wm *WorkspaceManager) Widget() *WidgetManager        { return wm.widgetManager }
func (wm *WorkspaceManager) Focus() *FocusManager          { return wm.focusManager }
func (wm *WorkspaceManager) Stack() *StackedPaneManager    { return wm.stackedPaneManager }
func (wm *WorkspaceManager) Close() *CloseManager          { return wm.closeManager }
func (wm *WorkspaceManager) CSS() *CSSManager              { return wm.cssManager }
func (wm *WorkspaceManager) IDs() *IDManager               { return wm.idManager }

// Factory function access
func (wm *WorkspaceManager) createWebView() (*webkit.WebView, error) { return wm.createWebViewFn() }
func (wm *WorkspaceManager) createPane(view *webkit.WebView) (*BrowserPane, error) { return wm.createPaneFn(view) }

// Legacy support methods for existing code that depends on WorkspaceManager
func (wm *WorkspaceManager) splitNode(target *paneNode, orientation webkit.Orientation) (*paneNode, error) {
	return wm.layoutManager.SplitNode(target, orientation)
}

func (wm *WorkspaceManager) closeCurrentPane() error {
	return wm.closeManager.CloseCurrentPane()
}

func (wm *WorkspaceManager) closePane(node *paneNode) error {
	return wm.closeManager.ClosePane(node)
}

func (wm *WorkspaceManager) collectLeaves() []*paneNode {
	return wm.layoutManager.CollectLeaves()
}

func (wm *WorkspaceManager) ensureStyles() {
	wm.cssManager.EnsureStyles()
}

func (wm *WorkspaceManager) ensurePaneBaseClasses() {
	wm.cssManager.EnsurePaneBaseClasses()
}

func (wm *WorkspaceManager) initializePaneWidgets(node *paneNode, container uintptr) {
	wm.widgetManager.InitializePaneWidgets(node, container)
}

func (wm *WorkspaceManager) safeWidgetOperation(op func() error) error {
	return wm.widgetManager.SafeWidgetOperation(op)
}

func (wm *WorkspaceManager) validateWidgetsForReparenting(widgets ...*SafeWidget) error {
	return wm.widgetManager.ValidateWidgetsForReparenting(widgets...)
}

func (wm *WorkspaceManager) setContainer(node *paneNode, container uintptr, typeInfo string) {
	wm.widgetManager.SetContainer(node, container, typeInfo)
}

func (wm *WorkspaceManager) setTitleBar(node *paneNode, titleBar uintptr) {
	wm.widgetManager.SetTitleBar(node, titleBar)
}

func (wm *WorkspaceManager) setStackWrapper(node *paneNode, stackWrapper uintptr) {
	wm.widgetManager.SetStackWrapper(node, stackWrapper)
}

func (wm *WorkspaceManager) removeFromMaps(view *webkit.WebView) {
	wm.closeManager.RemoveFromMaps(view)
}

func (wm *WorkspaceManager) removeFromAppPanes(pane *BrowserPane) {
	wm.closeManager.RemoveFromAppPanes(pane)
}

// ensureHover sets up hover tracking for a pane node
func (wm *WorkspaceManager) ensureHover(node *paneNode) {
	// TODO: Extract hover logic from deprecated workspace_manager.go
	// This handles GTK hover events for UI feedback
}

// detachHover removes hover tracking for a pane node
func (wm *WorkspaceManager) detachHover(node *paneNode) {
	// TODO: Extract hover logic from deprecated workspace_manager.go
	// This cleans up GTK hover event handlers
}

// FocusNeighbor moves focus to the nearest pane in the requested direction
func (wm *WorkspaceManager) FocusNeighbor(direction string) bool {
	if wm.focusManager == nil {
		return false
	}
	return wm.focusManager.FocusNeighbor(direction)
}

// GetActiveNode returns the currently active pane node
func (wm *WorkspaceManager) GetActiveNode() *paneNode {
	if wm.focusManager == nil {
		return nil
	}
	return wm.focusManager.GetActiveNode()
}

// ensureGUIInPane ensures GUI content is loaded in the specified pane
func (wm *WorkspaceManager) ensureGUIInPane(pane *paneNode) error {
	if wm.layoutManager == nil {
		return errors.New("layout manager not initialized")
	}
	return wm.layoutManager.EnsureGUIInPane(pane)
}

// RegisterNavigationHandler registers a navigation handler
func (wm *WorkspaceManager) RegisterNavigationHandler(handler interface{}) {
	if wm.layoutManager != nil {
		wm.layoutManager.RegisterNavigationHandler(handler)
	}
}

// UpdateTitleBar updates the title bar for a specific pane
func (wm *WorkspaceManager) UpdateTitleBar(view *webkit.WebView, title string) {
	if wm.layoutManager != nil {
		wm.layoutManager.UpdateTitleBar(view, title)
	}
}

// focusByView focuses a pane by its web view
func (wm *WorkspaceManager) focusByView(view *webkit.WebView) bool {
	if wm.focusManager == nil {
		return false
	}
	return wm.focusManager.FocusByView(view)
}

// HandlePopup handles popup window creation
func (wm *WorkspaceManager) HandlePopup(sourceView *webkit.WebView, targetURL string) *webkit.WebView {
	// Popup handling should be in LayoutManager since it's about window/pane layout
	if wm.layoutManager == nil {
		return nil
	}
	return wm.layoutManager.HandlePopup(sourceView, targetURL)
}