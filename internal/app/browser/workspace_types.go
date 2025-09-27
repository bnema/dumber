// workspace_types.go - Type definitions and constants for workspace management
package browser

import (
	"github.com/bnema/dumber/pkg/webkit"
)

// paneNode represents a node in the workspace pane tree structure.
// It can be either a leaf node (containing a browser pane) or a branch node
// (containing child nodes for split panes or stacked panes).
type paneNode struct {
	pane   *BrowserPane
	parent *paneNode
	left   *paneNode
	right  *paneNode

	// Widget management with proper lifecycle tracking
	container   *SafeWidget // Main container (GtkPaned for branch nodes, wrapper GtkBox for stacked nodes, WebView container for leaves)
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

	// Stacked panes support with proper widget management
	isStacked        bool        // Whether this node contains stacked panes
	stackedPanes     []*paneNode // List of stacked panes (if isStacked)
	activeStackIndex int         // Index of currently visible pane in stack
	titleBar         *SafeWidget // GtkBox for title bar (when collapsed)
	stackWrapper     *SafeWidget // Internal GtkBox containing the actual stacked widgets (titles + webviews)
}

// Workspace CSS class constants
const (
	basePaneClass  = "workspace-pane"
	multiPaneClass = "workspace-multi-pane"
)

// Focus calculation epsilon for geometric comparisons
const focusEpsilon = 1e-3
