// workspace_css.go - CSS generation and styling for workspace panes
package browser

import (
	"fmt"
	"log"
	"strings"

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

// getActivePaneBorderColor returns the border color for active panes based on config and theme
func getActivePaneBorderColor(styling config.WorkspaceStylingConfig, isDark bool) string {
	// Use configured border color if set
	if styling.BorderColor != "" {
		// Check if it's a GTK theme variable (starts with @)
		if strings.HasPrefix(styling.BorderColor, "@") {
			return styling.BorderColor
		}
		// Return the configured color as-is (could be hex, rgb, etc.)
		return styling.BorderColor
	}

	// Fallback to hardcoded colors based on theme
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

	activeBorderColor := getActivePaneBorderColor(styling, isDark)

	// Pane mode border color - use configured value or fallback to orange
	paneModeColor := styling.PaneModeBorderColor
	if paneModeColor == "" {
		paneModeColor = "#FFA500" // Fallback to orange if not configured
	}

	css := fmt.Sprintf(`window {
	  background-color: %s;
	  padding: 0;
	  margin: 0;
	}

	/* Pane mode active - change window background to border color */
	window.pane-mode-active {
	  background-color: %s;
	}

	/* Workspace root container */
	paned, box {
	  background-color: %s;
	  transition: margin 150ms ease-in-out;
	}

	/* Base pane styling - subtle border for inactive panes */
	.workspace-pane, .stacked-pane-container {
	  border: 2px solid %s;
	  border-radius: %dpx;
	  transition: border-color %dms ease-in-out;
	  margin: 0;
	}

	/* Active pane border styling */
	.workspace-pane-active {
	  border-color: %s;
	}

	/* Stacked panes styling */
	.stacked-pane-container {
	  background-color: %s;
	}

	/* Active stacked pane container gets the outline color */
	.stacked-pane-container.workspace-pane-active {
	  border-color: %s;
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

	.stacked-pane-favicon {
	  margin-right: 4px;
	}

	.stacked-pane-active {
	  /* Active pane is fully visible */
	}

	.stacked-pane-collapsed {
	  /* Collapsed panes are hidden - handled in code via widget visibility */
	}`,
		windowBackgroundColor,          // window background
		paneModeColor,                  // window.pane-mode-active background (border color)
		windowBackgroundColor,          // paned, box background
		inactiveBorderColor,            // base pane border color (inactive)
		styling.BorderRadius,           // base pane border radius
		styling.TransitionDuration,     // base pane border transition
		activeBorderColor,              // workspace-pane-active border color
		windowBackgroundColor,          // stacked-pane-container background
		activeBorderColor,              // stacked-pane-container.workspace-pane-active border color
		getStackTitleBg(isDark),        // stacked-pane-title background
		inactiveBorderColor,            // stacked-pane-title border-bottom
		styling.TransitionDuration,     // stacked-pane-title transition
		getStackTitleHoverBg(isDark),   // stacked-pane-title:hover background
		getStackTitleTextColor(isDark), // stacked-pane-title-text color
	)

	return css
}

// Global CSS provider tracking to prevent duplication
var (
	globalCSSInitialized bool
	globalCSSContent     string
)

// ensureWorkspaceStyles ensures that CSS styles are applied for workspace panes
// Improved to prevent CSS provider duplication by using global CSS tracking
func (wm *WorkspaceManager) ensureWorkspaceStyles() {
	if wm == nil {
		return
	}

	// Generate CSS content
	workspaceCSS := wm.generateWorkspaceCSS()

	// Only apply CSS if it hasn't been applied globally or if content changed
	if !globalCSSInitialized || globalCSSContent != workspaceCSS {
		webkit.AddCSSProvider(workspaceCSS)
		globalCSSInitialized = true
		globalCSSContent = workspaceCSS
		log.Printf("[workspace] Applied workspace CSS styles (%d bytes)", len(workspaceCSS))
		log.Printf("[workspace] CSS preview: %s...", workspaceCSS[:min(200, len(workspaceCSS))])
	}

	wm.cssInitialized = true
}
