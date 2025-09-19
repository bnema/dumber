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
	hoverToken  uintptr
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
}

const (
	// TODO : should be defined via Config + Defaults
	activePaneCSS = `.workspace-pane-active {
	  border: 2px solid @theme_selected_bg_color;
	  border-radius: 0px;
	  transition: border-color 120ms ease-in-out;
}`
	activePaneClass = "workspace-pane-active"
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

	registerFocusMove("alt+ArrowLeft", "left")
	registerFocusMove("alt+ArrowRight", "right")
	registerFocusMove("alt+ArrowUp", "up")
	registerFocusMove("alt+ArrowDown", "down")
	registerFocusMove("cmdorctrl+ArrowUp", "up")
	registerFocusMove("cmdorctrl+ArrowDown", "down")

	// Reduced logging: only log shortcuts registration during initialization, not on every hover
	if wm.app.config != nil && wm.app.config.Debug.EnableWorkspaceDebug {
		log.Printf("[workspace] registered navigation shortcuts on webView=%p", webView)
	}
}

// NewWorkspaceManager builds a workspace manager rooted at the provided pane.
func NewWorkspaceManager(app *BrowserApp, rootPane *BrowserPane) *WorkspaceManager {
	manager := &WorkspaceManager{
		app:          app,
		window:       rootPane.webView.Window(),
		viewToNode:   make(map[*webkit.WebView]*paneNode),
		lastSplitMsg: make(map[*webkit.WebView]time.Time),
		lastExitMsg:  make(map[*webkit.WebView]time.Time),
	}
	manager.createWebViewFn = func() (*webkit.WebView, error) {
		if manager.app == nil {
			return nil, errors.New("workspace manager missing app reference")
		}
		cfg, err := manager.app.buildWebkitConfig()
		if err != nil {
			return nil, err
		}
		cfg.CreateWindow = false
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
	default:
		log.Printf("[workspace] unhandled workspace event: %s", msg.Event)
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
		detail := map[string]any{
			"active":    false,
			"webview":   fmt.Sprintf("%p", previous.pane.webView),
			"paneId":    previous.pane.ID(),
			"timestamp": time.Now().UnixMilli(),
		}
		if err := previous.pane.webView.DispatchCustomEvent("dumber:workspace-focus", detail); err != nil {
			log.Printf("[workspace] failed to notify focus loss: %v", err)
		} else if wm.app.config != nil && wm.app.config.Debug.EnableWorkspaceDebug {
			log.Printf("[workspace] notified focus loss for pane %s", previous.pane.ID())
		}
	}

	if wm.active != nil && wm.active.container != 0 {
		webkit.WidgetRemoveCSSClass(wm.active.container, activePaneClass)
	}

	wm.active = node
	wm.app.activePane = node.pane
	wm.app.webView = node.pane.webView
	wm.app.zoomController = node.pane.zoomController
	wm.app.navigationController = node.pane.navigationController
	wm.app.clipboardController = node.pane.clipboardController
	wm.app.messageHandler = node.pane.messageHandler
	wm.app.shortcutHandler = node.pane.shortcutHandler

	// Re-register workspace navigation shortcuts on the newly focused webView
	wm.registerWorkspaceShortcuts(node.pane.webView)

	if handler := node.pane.messageHandler; handler != nil {
		handler.SetWorkspaceObserver(wm)
	}

	if wm.app.browserService != nil {
		wm.app.browserService.AttachWebView(node.pane.webView)
	}

	container := node.container
	viewWidget := node.pane.webView.Widget()
	if container != 0 && container != previousContainer {
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

	if node.pane != nil && node.pane.webView != nil && node.pane.webView != previousWebView {
		// Update pane focus time
		node.pane.UpdateLastFocus()

		// Enhanced focus event with pane ID and GUI status
		detail := map[string]any{
			"active":    true,
			"webview":   fmt.Sprintf("%p", node.pane.webView),
			"paneId":    node.pane.ID(),
			"hasGUI":    node.pane.HasGUI(),
			"timestamp": time.Now().UnixMilli(),
		}

		// Dispatch focus event to new active pane
		if err := node.pane.webView.DispatchCustomEvent("dumber:workspace-focus", detail); err != nil {
			log.Printf("[workspace] failed to notify focus gain: %v", err)
		} else if wm.app.config != nil && wm.app.config.Debug.EnableWorkspaceDebug {
			log.Printf("[workspace] notified focus gain for pane %s", node.pane.ID())
		}

		// Lazy-load GUI components if first focus
		if !node.pane.HasGUI() {
			wm.ensureGUIInPane(node.pane)
		}
	}
}

func (wm *WorkspaceManager) ensureStyles() {
	if wm == nil || wm.cssInitialized {
		return
	}
	webkit.AddCSSProvider(activePaneCSS)
	wm.cssInitialized = true
}

func (wm *WorkspaceManager) focusByView(view *webkit.WebView) {
	if wm == nil || view == nil {
		return
	}
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
	if node == nil {
		return nil
	}
	if node.isLeaf {
		return node
	}
	switch direction {
	case "up":
		if leaf := wm.boundaryFallback(node.right, direction); leaf != nil {
			return leaf
		}
		return wm.boundaryFallback(node.left, direction)
	case "down":
		if leaf := wm.boundaryFallback(node.left, direction); leaf != nil {
			return leaf
		}
		return wm.boundaryFallback(node.right, direction)
	case "left":
		if leaf := wm.boundaryFallback(node.right, direction); leaf != nil {
			return leaf
		}
		return wm.boundaryFallback(node.left, direction)
	case "right":
		if leaf := wm.boundaryFallback(node.left, direction); leaf != nil {
			return leaf
		}
		return wm.boundaryFallback(node.right, direction)
	default:
		return nil
	}
}

func (wm *WorkspaceManager) collectLeaves() []*paneNode {
	return wm.collectLeavesFrom(wm.root)
}

func (wm *WorkspaceManager) collectLeavesFrom(node *paneNode) []*paneNode {
	var leaves []*paneNode
	var walk func(*paneNode)
	walk = func(n *paneNode) {
		if n == nil {
			return
		}
		if n.isLeaf {
			leaves = append(leaves, n)
			return
		}
		walk(n.left)
		walk(n.right)
	}
	walk(node)
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
	// Note: GTK4 may emit a critical error about widget parenting here when splitting
	// the root pane, but the operation still succeeds. This is a limitation of GTK's
	// widget hierarchy tracking when widgets are moved between window and paned containers.
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
		node.pane.webView.Destroy()
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
			node.pane.webView.Destroy()
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
			node.pane.webView.Destroy()
		} else {
			// Multiple panes remain, don't destroy the window - just clean up the webview
			log.Printf("[workspace] skipping webview destruction to preserve window (panes remaining: %d)", remaining-1)
			// TODO: Add a method to destroy just the webview without the window
		}

		wm.updateMainPane()
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
		// Parent is the root node. We need to restructure.
		// If going from 2 panes to 1, promote sibling to become the new root
		if remaining == 2 { // Will become 1 after this close
			log.Printf("[workspace] promoting sibling to root (last 2 panes): container=%#x", sibling.container)
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
			// More than 2 panes remain, so keep the tree structure
			// The sibling should remain as a child of root, but we need to remove the parent paned
			log.Printf("[workspace] removing parent paned, keeping sibling under root: container=%#x", sibling.container)
			// This shouldn't happen in a proper binary tree structure
			log.Printf("[workspace] ERROR: unexpected tree state - parent is root but more than 2 panes remain")
			return errors.New("unexpected tree state during pane closure")
		}
	} else {
		// Parent has a grandparent, so promote sibling to take parent's place
		log.Printf("[workspace] promoting sibling to parent's position: sibling=%#x grand=%#x", sibling.container, grand.container)
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
		node.pane.webView.Destroy()
	} else {
		// Multiple panes remain, don't destroy the window - just clean up the webview
		log.Printf("[workspace] skipping webview destruction to preserve window (panes remaining: %d)", remaining-1)
		// TODO: Add a method to destroy just the webview without the window
	}

	wm.updateMainPane()
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

func (wm *WorkspaceManager) HandlePopup(source *webkit.WebView, url string) bool {
	if wm == nil || source == nil {
		return false
	}

	node := wm.viewToNode[source]
	if node == nil {
		log.Printf("[workspace] popup from unknown webview")
		return false
	}

	cfg := wm.app.config
	if cfg == nil {
		return false
	}

	popCfg := cfg.Workspace.Popups
	if !popCfg.OpenInNewPane {
		return false
	}

	direction := strings.ToLower(popCfg.Placement)
	if direction == "" {
		direction = "right"
	}

	target := node
	if !popCfg.FollowPaneContext && wm.active != nil {
		target = wm.active
	}

	newNode, err := wm.splitNode(target, direction)
	if err != nil {
		log.Printf("[workspace] popup split failed: %v", err)
		return false
	}

	if url == "" {
		return true
	}

	if newNode == nil || newNode.pane == nil {
		return true
	}

	if newNode.pane.navigationController != nil {
		if err := newNode.pane.navigationController.NavigateToURL(url); err != nil {
			log.Printf("[workspace] popup navigation failed: %v", err)
			if newNode.pane.webView != nil {
				if loadErr := newNode.pane.webView.LoadURL(url); loadErr != nil {
					log.Printf("[workspace] popup load fallback failed: %v", loadErr)
				}
			}
		}
		return true
	}

	if newNode.pane.webView != nil {
		if err := newNode.pane.webView.LoadURL(url); err != nil {
			log.Printf("[workspace] popup load failed: %v", err)
		}
	}

	return true
}

// ensureGUIInPane lazily loads GUI components into a pane when it gains focus
func (wm *WorkspaceManager) ensureGUIInPane(pane *BrowserPane) {
	if pane == nil || pane.HasGUI() {
		return
	}

	log.Printf("[workspace] Injecting GUI components into pane %s", pane.ID())

	// Inject GUI manager and omnibox
	script := fmt.Sprintf(`
		(async function() {
			try {
				// Set up GUI manager for this pane
				window.__dumber_pane.active = true;
				window.__dumber_gui_bootstrap && window.__dumber_gui_bootstrap('%s');

				// For now, mark as ready - full GUIManager will be implemented later
				console.log('[workspace] GUI components ready for pane %s');
			} catch (error) {
				console.error('[workspace] Failed to initialize GUI:', error);
			}
		})();
	`, pane.ID(), pane.ID())

	if err := pane.WebView().InjectScript(script); err != nil {
		log.Printf("[workspace] Failed to inject GUI into pane %s: %v", pane.ID(), err)
		return
	}

	// Mark GUI as injected
	pane.SetHasGUI(true)
	pane.SetGUIComponent("manager", true)
	pane.SetGUIComponent("omnibox", true)
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
