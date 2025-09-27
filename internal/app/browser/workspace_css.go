// workspace_css.go - CSS generation and styling for workspace panes
package browser

import (
	"fmt"

	"github.com/bnema/dumber/internal/config"
	"github.com/bnema/dumber/pkg/webkit"
)

// Helper functions for stacked pane CSS colors

// getStackTitleBg returns the background color for stack titles based on theme
func getStackTitleBg(isDark bool) string {
	if isDark {
		return "#404040"
	}
	return "#f0f0f0"
}

// getStackTitleHoverBg returns the hover background color for stack titles based on theme
func getStackTitleHoverBg(isDark bool) string {
	if isDark {
		return "#505050"
	}
	return "#e0e0e0"
}

// getStackTitleTextColor returns the text color for stack titles based on theme
func getStackTitleTextColor(isDark bool) string {
	if isDark {
		return "#ffffff"
	}
	return "#333333"
}

// getActivePaneBorderColor returns the border color for active panes based on theme
func getActivePaneBorderColor(isDark bool) string {
	if isDark {
		return "#4A90E2" // Deeper blue pastel for dark theme
	}
	return "#87CEEB" // Sky blue pastel for light theme
}

// generateWorkspaceCSS generates the complete CSS for workspace panes and stacked panes
func (wm *WorkspaceManager) generateWorkspaceCSS() string {
	cfg := config.Get()
	styling := cfg.Workspace.Styling

	// Get appropriate colors based on GTK theme preference
	var inactiveBorderColor, windowBackgroundColor string
	isDark := webkit.PrefersDarkTheme()
	if isDark {
		inactiveBorderColor = "#333333"   // Dark border for dark theme
		windowBackgroundColor = "#2b2b2b" // Dark window background
	} else {
		inactiveBorderColor = "#dddddd"   // Light border for light theme
		windowBackgroundColor = "#ffffff" // Light window background
	}

	activeBorderColor := getActivePaneBorderColor(isDark)

	css := fmt.Sprintf(`window {
	  background-color: %s;
	}

	/* Active pane border styling using outline (no layout impact) */
	.workspace-pane-active {
	  outline: 2px solid %s;
	  outline-offset: -2px;
	  transition: outline-color %dms ease-in-out;
	}

	/* Stacked panes styling */
	.stacked-pane-container {
	  background-color: %s;
	  border-radius: %dpx;
	}

	/* Active stacked pane container gets the outline */
	.stacked-pane-container.workspace-pane-active {
	  outline: 2px solid %s;
	  outline-offset: -2px;
	  transition: outline-color %dms ease-in-out;
	}

	.stacked-pane-title {
	  background-color: %s;
	  border-bottom: 1px solid %s;
	  padding: 4px 8px;
	  min-height: 24px;
	  transition: background-color %dms ease-in-out;
	}

	.stacked-pane-title:hover {
	  background-color: %s;
	}

	.stacked-pane-title-text {
	  font-size: 12px;
	  color: %s;
	  font-weight: 500;
	}

	.stacked-pane-active {
	  /* Active pane is fully visible */
	}

	.stacked-pane-collapsed {
	  /* Collapsed panes are hidden - handled in code via widget visibility */
	}`,
		windowBackgroundColor,          // window background
		activeBorderColor,              // workspace-pane-active border
		styling.TransitionDuration,     // workspace-pane-active transition
		windowBackgroundColor,          // stacked-pane-container background
		styling.BorderRadius,           // stacked-pane-container border-radius
		activeBorderColor,              // stacked-pane-container.workspace-pane-active border
		styling.TransitionDuration,     // stacked-pane-container.workspace-pane-active transition
		getStackTitleBg(isDark),        // stacked-pane-title background
		inactiveBorderColor,            // stacked-pane-title border-bottom
		styling.TransitionDuration,     // stacked-pane-title transition
		getStackTitleHoverBg(isDark),   // stacked-pane-title:hover background
		getStackTitleTextColor(isDark), // stacked-pane-title-text color
	)

	return css
}

// generateStackedPaneCSS generates the CSS for stacked pane title bars (deprecated - use generateWorkspaceCSS)
func (wm *WorkspaceManager) generateStackedPaneCSS() string {
	// Delegate to the new comprehensive CSS generator
	return wm.generateWorkspaceCSS()
}

// ensureWorkspaceStyles ensures that CSS styles are applied for workspace panes
func (wm *WorkspaceManager) ensureWorkspaceStyles() {
	if wm == nil || wm.cssInitialized {
		return
	}
	workspaceCSS := wm.generateWorkspaceCSS()
	webkit.AddCSSProvider(workspaceCSS)
	wm.cssInitialized = true
}

// ensureStackedPaneStyles ensures that CSS styles are applied for stacked panes (deprecated - use ensureWorkspaceStyles)
func (wm *WorkspaceManager) ensureStackedPaneStyles() {
	wm.ensureWorkspaceStyles()
}
