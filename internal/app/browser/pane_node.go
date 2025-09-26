package browser

import (
	"github.com/bnema/dumber/pkg/webkit"
)

// paneNode represents a node in the workspace layout tree
type paneNode struct {
	pane        *BrowserPane
	parent      *paneNode
	left        *paneNode
	right       *paneNode

	// Widget management with proper lifecycle tracking
	container    *SafeWidget // Main container (GtkPaned for branch nodes, wrapper GtkBox for stacked nodes, WebView container for leaves)
	orientation  webkit.Orientation
	isLeaf       bool
	isPopup      bool // Deprecated: use windowType instead

	// Window type tracking
	windowType     webkit.WindowType      // Tab or Popup
	windowFeatures *webkit.WindowFeatures // Features if popup
	isRelated      bool                   // Shares context
	parentPane     *paneNode              // Parent for related views
	autoClose      bool                   // Auto-close on OAuth success
	hoverToken     uintptr

	// Stacked panes support with proper widget management
	isStacked        bool        // Whether this node contains stacked panes
	stackedPanes     []*paneNode // List of stacked panes (if isStacked)
	activeStackIndex int         // Index of currently visible pane in stack
	titleBar         *SafeWidget // GtkBox for title bar (when collapsed)
	stackWrapper     *SafeWidget // Internal GtkBox containing the actual stacked widgets (titles + webviews)

	// Phase 2: Unified Pane Type System
	metadata *PaneMetadata // Unified metadata for all pane types
}

// getContainerPtr safely gets the container pointer for GTK operations
func (n *paneNode) getContainerPtr() uintptr {
	if n == nil || n.container == nil {
		return 0
	}
	var ptr uintptr
	n.container.Execute(func(containerPtr uintptr) error {
		ptr = containerPtr
		return nil
	})
	return ptr
}

// getTitleBarPtr safely gets the title bar pointer for GTK operations
func (n *paneNode) getTitleBarPtr() uintptr {
	if n == nil || n.titleBar == nil {
		return 0
	}
	var ptr uintptr
	n.titleBar.Execute(func(titlePtr uintptr) error {
		ptr = titlePtr
		return nil
	})
	return ptr
}

// getStackWrapperPtr safely gets the stack wrapper pointer for GTK operations
func (n *paneNode) getStackWrapperPtr() uintptr {
	if n == nil || n.stackWrapper == nil {
		return 0
	}
	var ptr uintptr
	n.stackWrapper.Execute(func(stackPtr uintptr) error {
		ptr = stackPtr
		return nil
	})
	return ptr
}