package browser

import (
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/bnema/dumber/pkg/webkit"
)

// LayoutManager handles tree structure and layout operations
type LayoutManager struct {
	wm *WorkspaceManager
}

// NewLayoutManager creates a new layout manager
func NewLayoutManager(wm *WorkspaceManager) *LayoutManager {
	return &LayoutManager{wm: wm}
}

// SplitNode creates a new split in the workspace
func (lm *LayoutManager) SplitNode(splitTarget *paneNode, orientation webkit.Orientation) (*paneNode, error) {
	if splitTarget == nil || !splitTarget.isLeaf {
		return nil, errors.New("split target must be a leaf pane")
	}

	// Create new WebView and pane
	newView, err := lm.wm.createWebView()
	if err != nil {
		return nil, err
	}

	newPane, err := lm.wm.createPane(newView)
	if err != nil {
		return nil, err
	}

	if handler := newPane.MessageHandler(); handler != nil {
		handler.SetWorkspaceObserver(lm.wm)
	}

	// Create container for new pane
	newContainer := newPane.webView.RootWidget()
	if newContainer == 0 {
		return nil, errors.New("new pane missing container")
	}
	webkit.WidgetSetHExpand(newContainer, true)
	webkit.WidgetSetVExpand(newContainer, true)
	webkit.WidgetRealizeInContainer(newContainer)

	// Create GtkPaned for split
	paned := webkit.NewPaned(orientation)
	if paned == 0 {
		return nil, errors.New("failed to create GtkPaned")
	}
	webkit.WidgetSetHExpand(paned, true)
	webkit.WidgetSetVExpand(paned, true)
	webkit.PanedSetResizeStart(paned, true)
	webkit.PanedSetResizeEnd(paned, true)

	newLeaf := &paneNode{
		pane:   newPane,
		isLeaf: true,
	}
	lm.wm.widgetManager.InitializePaneWidgets(newLeaf, newContainer)
	lm.wm.initializePaneMetadata(newLeaf, PaneTypeRegular)

	split := &paneNode{
		parent:      splitTarget.parent,
		orientation: orientation,
		isLeaf:      false,
	}
	lm.wm.widgetManager.InitializePaneWidgets(split, paned)
	lm.wm.initializePaneMetadata(split, PaneTypeRegular)

	parent := split.parent

	// Detach split target container from its current GTK parent before inserting into new paned
	existingContainer := splitTarget.getContainerPtr()
	if parent == nil {
		// Target is the root - remove it from the window
		log.Printf("[layout] removing existing container=%#x from window", existingContainer)
		if lm.wm.window != nil {
			lm.wm.window.SetChild(0)
		}
		// Unparent if it has a GTK parent
		if webkit.WidgetGetParent(existingContainer) != 0 {
			webkit.WidgetUnparent(existingContainer)
		}
	} else if parent.container != nil {
		// Target has a parent paned - remove it (automatically unparents in GTK4)
		parent.container.Execute(func(panedPtr uintptr) error {
			if parent.left == splitTarget {
				webkit.PanedSetStartChild(panedPtr, 0)
			} else if parent.right == splitTarget {
				webkit.PanedSetEndChild(panedPtr, 0)
			}
			return nil
		})
	}

	// Set up the split structure
	split.left = splitTarget
	split.right = newLeaf
	splitTarget.parent = split
	newLeaf.parent = split

	// Update parent references
	if parent == nil {
		lm.wm.root = split
		if lm.wm.window != nil {
			lm.wm.window.SetChild(paned)
		}
	} else {
		if parent.left == splitTarget {
			parent.left = split
			parent.container.Execute(func(panedPtr uintptr) error {
				webkit.PanedSetStartChild(panedPtr, paned)
				return nil
			})
		} else if parent.right == splitTarget {
			parent.right = split
			parent.container.Execute(func(panedPtr uintptr) error {
				webkit.PanedSetEndChild(panedPtr, paned)
				return nil
			})
		}
	}

	// Add children to the new paned
	webkit.PanedSetStartChild(paned, existingContainer)
	newLeaf.container.Execute(func(containerPtr uintptr) error {
		webkit.PanedSetEndChild(paned, containerPtr)
		return nil
	})

	// Update workspace state
	lm.wm.viewToNode[newPane.webView] = newLeaf
	lm.wm.ensureHover(newLeaf)
	lm.wm.app.panes = append(lm.wm.app.panes, newPane)
	if newPane.zoomController != nil {
		newPane.zoomController.ApplyInitialZoom()
	}

	// Update CSS classes
	lm.wm.cssManager.EnsurePaneBaseClasses()

	// Focus the new pane
	lm.wm.focusManager.SetActivePane(newLeaf)
	lm.wm.currentlyFocused = newLeaf

	log.Printf("[layout] split complete: splitTarget=%p newLeaf=%p", splitTarget, newLeaf)
	return newLeaf, nil
}

// CollectLeaves returns all leaf nodes in the workspace
func (lm *LayoutManager) CollectLeaves() []*paneNode {
	return lm.collectLeavesFrom(lm.wm.root)
}

// collectLeavesFrom recursively collects leaf nodes from a subtree
func (lm *LayoutManager) collectLeavesFrom(node *paneNode) []*paneNode {
	if node == nil {
		return nil
	}

	if node.isLeaf {
		return []*paneNode{node}
	}

	if node.isStacked {
		// For stacked panes, return only the currently active pane
		if len(node.stackedPanes) > 0 {
			activeIndex := node.activeStackIndex
			if activeIndex >= 0 && activeIndex < len(node.stackedPanes) {
				return []*paneNode{node.stackedPanes[activeIndex]}
			}
		}
		return nil
	}

	// Regular branch node - collect from both children
	var leaves []*paneNode
	leaves = append(leaves, lm.collectLeavesFrom(node.left)...)
	leaves = append(leaves, lm.collectLeavesFrom(node.right)...)
	return leaves
}

// FindReplacementRoot finds a suitable replacement when a node is removed
func (lm *LayoutManager) FindReplacementRoot(toRemove *paneNode) *paneNode {
	if toRemove == nil || toRemove.parent == nil {
		return nil
	}

	parent := toRemove.parent
	var sibling *paneNode

	if parent.left == toRemove {
		sibling = parent.right
	} else if parent.right == toRemove {
		sibling = parent.left
	}

	return sibling
}

// UpdateMainPane updates the main pane reference
func (lm *LayoutManager) UpdateMainPane() {
	if lm.wm.root != nil && lm.wm.root.isLeaf {
		lm.wm.mainPane = lm.wm.root
	}
}

// SplitPane creates a new split with the given direction
func (lm *LayoutManager) SplitPane(source *webkit.WebView, direction string) error {
	if lm.wm == nil {
		return fmt.Errorf("layout manager not initialized")
	}
	
	// Find the source node
	sourceNode, exists := lm.wm.viewToNode[source]
	if !exists {
		return fmt.Errorf("source view not found in workspace")
	}
	
	// Convert direction string to webkit.Orientation
	var orientation webkit.Orientation
	switch direction {
	case "right", "left":
		orientation = webkit.OrientationHorizontal
	case "up", "down":
		orientation = webkit.OrientationVertical
	default:
		orientation = webkit.OrientationHorizontal // default
	}
	
	// Use existing SplitNode method
	_, err := lm.SplitNode(sourceNode, orientation)
	return err
}

// CreatePopup creates a popup window for the given URL
func (lm *LayoutManager) CreatePopup(source *webkit.WebView, targetURL string) error {
	// TODO: Implement popup creation logic
	// This should create a new popup window with the given URL
	log.Printf("[layout] CreatePopup called for URL: %s", targetURL)
	return fmt.Errorf("popup creation not yet implemented")
}

// EnsureGUIInPane ensures GUI content is loaded in the specified pane
func (lm *LayoutManager) EnsureGUIInPane(pane *paneNode) error {
	if pane == nil || pane.pane == nil || pane.pane.webView == nil {
		return fmt.Errorf("invalid pane for GUI initialization")
	}
	
	// Check if GUI is already loaded
	currentURL := pane.pane.webView.GetURI()
	if currentURL != "" && !strings.Contains(currentURL, "about:blank") {
		return nil // GUI already loaded
	}
	
	// Load GUI into the pane
	// TODO: Get actual GUI URL from config
	guiURL := "file:///gui/index.html" // placeholder
	pane.pane.webView.LoadURL(guiURL)
	
	log.Printf("[layout] Loaded GUI into pane %s", pane.pane.id)
	return nil
}

// RegisterNavigationHandler registers a navigation handler
func (lm *LayoutManager) RegisterNavigationHandler(handler interface{}) {
	// TODO: Implement navigation handler registration
	log.Printf("[layout] Navigation handler registered: %T", handler)
}

// UpdateTitleBar updates the title bar for a specific pane
func (lm *LayoutManager) UpdateTitleBar(view *webkit.WebView, title string) {
	if lm.wm == nil || view == nil {
		return
	}
	
	// Find the node associated with this view
	node, exists := lm.wm.viewToNode[view]
	if !exists {
		return
	}
	
	// If this is a stacked pane, update its title bar
	if node.parent != nil && node.parent.isStacked {
		if lm.wm.stackedPaneManager != nil {
			lm.wm.stackedPaneManager.UpdateTitleBar(view, title)
		}
	}
	
	log.Printf("[layout] Updated title bar for %s: %s", lm.wm.idManager.FormatWebView(view), title)
}

// HandlePopup handles popup window creation
func (lm *LayoutManager) HandlePopup(sourceView *webkit.WebView, targetURL string) *webkit.WebView {
	// TODO: Implement actual popup handling
	log.Printf("[layout] HandlePopup called - source: %s, URL: %s", lm.wm.idManager.FormatWebView(sourceView), targetURL)
	
	// For now, just return nil (no popup created)
	return nil
}
