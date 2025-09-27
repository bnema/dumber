// workspace_utils.go - Utility functions and helpers for workspace management
package browser

import (
	"errors"
	"fmt"

	"github.com/bnema/dumber/pkg/webkit"
)

// hasMultiplePanes returns true if there are multiple panes in the workspace
func (wm *WorkspaceManager) hasMultiplePanes() bool {
	return wm != nil && wm.app != nil && len(wm.app.panes) > 1
}

// UpdateTitleBar updates the title bar label for a WebView in stacked panes
func (wm *WorkspaceManager) UpdateTitleBar(webView *webkit.WebView, title string) {
	// Delegate to StackedPaneManager which has the correct logic
	if wm.stackedPaneManager != nil {
		wm.stackedPaneManager.UpdateTitleBar(webView, title)
	}
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

// mapDirection maps a direction string to GTK orientation and positioning
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

// Widget management utilities

// validateWidgetsForReparenting validates that all widgets needed for reparenting are valid
func (wm *WorkspaceManager) validateWidgetsForReparenting(sibling, parent, grand *paneNode) error {
	// Validate sibling container (this is the most critical one)
	if sibling.container == nil {
		return errors.New("sibling container is nil")
	}
	if !sibling.container.IsValid() {
		return fmt.Errorf("sibling container %s is not valid", sibling.container.String())
	}

	// Validate parent container if present
	if parent != nil && parent.container != nil && !parent.container.IsValid() {
		return fmt.Errorf("parent container %s is not valid", parent.container.String())
	}

	// Validate grandparent container if present
	if grand != nil && grand.container != nil && !grand.container.IsValid() {
		return fmt.Errorf("grandparent container %s is not valid", grand.container.String())
	}

	return nil
}

// safeWidgetOperation performs a widget operation with proper locking and validation
func (wm *WorkspaceManager) safeWidgetOperation(operation func() error) error {
	wm.widgetMutex.Lock()
	defer wm.widgetMutex.Unlock()
	return operation()
}

// registerWidget registers a widget with the registry and returns a SafeWidget
func (wm *WorkspaceManager) registerWidget(ptr uintptr, typeInfo string) *SafeWidget {
	if ptr == 0 {
		return nil
	}
	return wm.widgetRegistry.Register(ptr, typeInfo)
}

// initializePaneWidgets sets up SafeWidget wrappers for a paneNode
func (wm *WorkspaceManager) initializePaneWidgets(node *paneNode, containerPtr uintptr) {
	// Register the main container
	node.container = wm.registerWidget(containerPtr, "pane-container")

	// Initialize other widget fields as nil (will be set when needed)
	node.titleBar = nil
	node.stackWrapper = nil
}

// Helper functions for safe widget operations

// getContainerPtr returns the container widget pointer for a pane node
func (node *paneNode) getContainerPtr() uintptr {
	if node.container != nil {
		return node.container.Ptr()
	}
	return 0
}

// getTitleBarPtr returns the title bar widget pointer for a pane node
func (node *paneNode) getTitleBarPtr() uintptr {
	if node.titleBar != nil {
		return node.titleBar.Ptr()
	}
	return 0
}

// getStackWrapperPtr returns the stack wrapper widget pointer for a pane node
func (node *paneNode) getStackWrapperPtr() uintptr {
	if node.stackWrapper != nil {
		return node.stackWrapper.Ptr()
	}
	return 0
}

// setContainer sets the SafeWidget container
func (wm *WorkspaceManager) setContainer(node *paneNode, ptr uintptr, typeInfo string) {
	node.container = wm.registerWidget(ptr, typeInfo)
}

// setTitleBar sets the SafeWidget titleBar
func (wm *WorkspaceManager) setTitleBar(node *paneNode, ptr uintptr) {
	node.titleBar = wm.registerWidget(ptr, "title-bar")
}

// setStackWrapper sets the SafeWidget stackWrapper
func (wm *WorkspaceManager) setStackWrapper(node *paneNode, ptr uintptr) {
	node.stackWrapper = wm.registerWidget(ptr, "stack-wrapper")
}
