// workspace_types.go - Type definitions and constants for workspace management
package browser

import (
	"os"

	"github.com/bnema/dumber/pkg/webkit"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// DebugLevel controls validation and safety checks
type DebugLevel int

const (
	// DebugOff disables all validation (production mode)
	DebugOff DebugLevel = iota
	// DebugBasic enables basic validation only (development default)
	DebugBasic
	// DebugFull enables all validation and detailed logging (testing)
	DebugFull
)

// getDebugLevel reads debug level from environment variable
func getDebugLevel() DebugLevel {
	switch os.Getenv("DUMBER_DEBUG_WORKSPACE") {
	case "off", "0":
		return DebugOff
	case "basic", "1":
		return DebugBasic
	case "full", "2":
		return DebugFull
	default:
		// Default to basic for development
		return DebugBasic
	}
}

// paneNode represents a node in the workspace pane tree structure.
// It can be either a leaf node (containing a browser pane) or a branch node
// (containing child nodes for split panes or stacked panes).
//
// Node Types:
// 1. Regular Leaf (isLeaf=true, windowType=Tab): Normal browsing pane
// 2. Popup Leaf (isLeaf=true, windowType=Popup): OAuth/feature-restricted popup
// 3. Stacked Container (isStacked=true, no left/right): Terminal branch with stackedPanes[]
// 4. Split Branch (isLeaf=false, has left+right): Pure layout node with GtkPaned
type paneNode struct {
	pane   *BrowserPane
	parent *paneNode
	left   *paneNode
	right  *paneNode

	// Widget management - gotk4 widgets (all ops on main thread)
	container   gtk.Widgetter // Main container: *gtk.Paned (branch), *gtk.Box (stack), or WebView widget (leaf)
	orientation gtk.Orientation
	isLeaf      bool
	isPopup     bool // Deprecated: use windowType instead

	// Window type tracking
	windowType           webkit.WindowType      // Tab or Popup
	windowFeatures       *webkit.WindowFeatures // Features if popup
	isRelated            bool                   // Shares context
	parentPane           *paneNode              // Parent for related views
	activePopupChildren  []string               // WebView IDs of active popup children (for tracking related popups)
	autoClose            bool                   // Auto-close on OAuth success
	requestID            string                 // Request ID for popup (used for localStorage cleanup)
	hoverToken           uintptr
	focusControllerToken uintptr
	pendingHoverReattach bool
	pendingFocusReattach bool

	// Stacked panes support - terminal branch nodes
	isStacked        bool        // Whether this node contains stacked panes
	stackedPanes     []*paneNode // List of stacked panes (if isStacked)
	activeStackIndex int         // Index of currently visible pane in stack
	titleBar         gtk.Widgetter     // *gtk.Box for title bar (when collapsed)
	stackWrapper     gtk.Widgetter     // Internal *gtk.Box containing the actual stacked widgets (titles + webviews)

	// Cleanup tracking
	widgetValid        bool                 // Guard flagged before GTK destruction
	cleanupGeneration  uint                 // Helps assert that asynchronous callbacks skip stale nodes
	pendingIdleHandles map[uintptr]struct{} // Idle callbacks touching this node
}

// Workspace CSS class constants
const (
	basePaneClass       = "workspace-pane"
	multiPaneClass      = "workspace-multi-pane"
	activePaneClass     = "workspace-pane-active"
	outlinePaneClass    = "workspace-pane-active-outline"
	stackContainerClass = "stacked-pane-container"
)

// Focus calculation epsilon for geometric comparisons
const focusEpsilon = 1e-3
