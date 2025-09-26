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

	log.Printf("[metadata] Initialized metadata for pane %s (type: %s, state: %s)",
		node.metadata.ID, node.metadata.Type, node.metadata.State)
}

// Manager access methods
func (wm *WorkspaceManager) Layout() *LayoutManager        { return wm.layoutManager }
func (wm *WorkspaceManager) Widget() *WidgetManager        { return wm.widgetManager }
func (wm *WorkspaceManager) Focus() *FocusManager          { return wm.focusManager }
func (wm *WorkspaceManager) Stack() *StackedPaneManager    { return wm.stackedPaneManager }
func (wm *WorkspaceManager) Close() *CloseManager          { return wm.closeManager }
func (wm *WorkspaceManager) CSS() *CSSManager              { return wm.cssManager }

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