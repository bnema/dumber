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

// getStackTitleBorderColor returns a subtle border color slightly darker than the title background
func getStackTitleBorderColor(isDark bool) string {
	if isDark {
		return "#353535" // Slightly darker than #404040
	}
	return "#e5e5e5" // Slightly darker than #f0f0f0
}

// getActiveTabBg returns a distinct background color for the active tab
func getActiveTabBg(isDark bool) string {
	if isDark {
		return "#707070" // Significantly lighter gray than inactive (#404040) - similar to stacked panes
	}
	return "#c0c0c0" // Significantly darker gray than inactive (#f0f0f0) - similar to stacked panes
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

// getInactivePaneBorderColor returns the border color for inactive panes based on config and theme
func getInactivePaneBorderColor(styling config.WorkspaceStylingConfig, isDark bool) string {
	// Use configured border color if set
	if styling.InactiveBorderColor != "" {
		// Check if it's a GTK theme variable (starts with @)
		if strings.HasPrefix(styling.InactiveBorderColor, "@") {
			return styling.InactiveBorderColor
		}
		// Return the configured color as-is (could be hex, rgb, etc.)
		return styling.InactiveBorderColor
	}

	// Fallback to hardcoded colors based on theme
	if isDark {
		return "#333333" // Dark border for dark theme
	}
	return "#dddddd" // Light border for light theme
}

// generateWorkspaceCSS generates the complete CSS for workspace panes and stacked panes
func (wm *WorkspaceManager) generateWorkspaceCSS() string {
	cfg := config.Get()
	styling := cfg.Workspace.Styling

	// Get appropriate colors based on GTK theme preference
	var windowBackgroundColor string
	isDark := webkit.PrefersDarkTheme()
	if isDark {
		windowBackgroundColor = "#2b2b2b" // Dark window background
	} else {
		windowBackgroundColor = "#ffffff" // Light window background
	}

	activeBorderColor := getActivePaneBorderColor(styling, isDark)
	inactiveBorderColor := getInactivePaneBorderColor(styling, isDark)

	// Pane mode border color - use configured value or fallback to blue
	paneModeColor := styling.PaneModeBorderColor
	if paneModeColor == "" {
		paneModeColor = "#4A90E2" // Fallback to blue if not configured
	}

	// Tab mode border color - use configured value or fallback to orange
	tabModeColor := styling.TabModeBorderColor
	if tabModeColor == "" {
		tabModeColor = "#FFA500" // Fallback to orange if not configured
	}

	// Stacked title border - subtle separator between titles
	stackedTitleBorder := fmt.Sprintf("1px solid %s", getStackTitleBorderColor(isDark))

	// UI Scale calculations for title bar elements (base values at 1.0 scale)
	uiScale := styling.UIScale
	if uiScale <= 0 {
		uiScale = 1.0 // Fallback to default if invalid
	}
	titleFontSize := int(12 * uiScale)         // Base: 12px
	titlePaddingVertical := int(4 * uiScale)   // Base: 4px
	titlePaddingHorizontal := int(8 * uiScale) // Base: 8px
	titleMinHeight := int(24 * uiScale)        // Base: 24px
	faviconMargin := int(4 * uiScale)          // Base: 4px

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

	/* Base pane styling - configurable inactive borders */
	.workspace-pane, .stacked-pane-container {
	  border-width: %dpx;
	  border-style: solid;
	  border-color: %s;
	  border-radius: %dpx;
	  margin: 0;
	  transition-property: border-color;
	  transition-duration: %dms;
	  transition-timing-function: ease-in-out;
	}

	/* Active pane border styling */
	.workspace-pane-active {
	  border-width: %dpx;
	  border-color: %s;
	}

	/* Stacked panes styling */
	.stacked-pane-container {
	  background-color: %s;
	}

	/* Stacked pane containers keep inactive border when active */
	.stacked-pane-container.workspace-pane-active {
	  border-width: %dpx;
	  border-color: %s;
	}

	.stacked-pane-title {
	  background-color: %s;
	  border-bottom: %s;
	  padding: %dpx %dpx;
	  min-height: %dpx;
	  transition: background-color %dms ease-in-out;
	}

	.stacked-pane-title:hover {
	  background-color: %s;
	}

	.stacked-pane-title-text {
	  font-size: %dpx;
	  color: %s;
	  font-weight: 500;
	}

	.stacked-pane-favicon {
	  margin-right: %dpx;
	}

	.stacked-pane-active {
	  /* Active pane is fully visible */
	}

	.stacked-pane-collapsed {
	  /* Collapsed panes are hidden - handled in code via widget visibility */
	}

	/* Tab bar styling */
	.tab-bar {
	  background-color: %s;
	  border-top: 2px solid %s;
	  padding: 0;
	  min-height: %dpx;
	}

	/* Tab mode active - window background changes to border color (like pane mode) */
	window.tab-mode-active {
	  background-color: %s;
	}

	/* Tab button styling - matches stacked pane titles */
	button.tab-button {
	  background-color: %s;
	  background-image: none;
	  border: none;
	  border-right: 1px solid %s;
	  border-radius: 0;
	  padding: %dpx %dpx;
	  transition: background-color %dms ease-in-out;
	}

	button.tab-button:hover {
	  background-color: %s;
	  background-image: none;
	}

	button.tab-button.tab-button-active {
	  background-color: %s;
	  background-image: none;
	  border: none;
	  border-right: 1px solid %s;
	  border-radius: 0;
	  font-weight: 600;
	}

	/* Tab title text */
	.tab-title {
	  font-size: %dpx;
	  color: %s;
	  font-weight: 500;
	}

	/* Tab content area */
	.tab-content-area {
	  background-color: %s;
	}

	/* Tab workspace containers */
	.tab-workspace-container {
	  background-color: %s;
	}

	/* Pulse animation for tab mode */
	@keyframes pulse-border {
	  0%%, 100%% { opacity: 1.0; }
	  50%% { opacity: 0.6; }
	}`,
		windowBackgroundColor,          // window background
		paneModeColor,                  // window.pane-mode-active background (border color)
		windowBackgroundColor,          // paned, box background
		styling.InactiveBorderWidth,    // base pane border-width (inactive)
		inactiveBorderColor,            // base pane border-color (inactive)
		styling.BorderRadius,           // base pane border radius
		styling.TransitionDuration,     // transition-duration
		styling.BorderWidth,            // workspace-pane-active border-width
		activeBorderColor,              // workspace-pane-active border color
		windowBackgroundColor,          // stacked-pane-container background
		styling.InactiveBorderWidth,    // stacked-pane-container.active border-width
		inactiveBorderColor,            // stacked-pane-container.active border-color
		getStackTitleBg(isDark),        // stacked-pane-title background
		stackedTitleBorder,             // stacked-pane-title border-bottom
		titlePaddingVertical,           // stacked-pane-title padding vertical
		titlePaddingHorizontal,         // stacked-pane-title padding horizontal
		titleMinHeight,                 // stacked-pane-title min-height
		styling.TransitionDuration,     // stacked-pane-title transition
		getStackTitleHoverBg(isDark),   // stacked-pane-title:hover background
		titleFontSize,                  // stacked-pane-title-text font-size
		getStackTitleTextColor(isDark), // stacked-pane-title-text color
		faviconMargin,                  // stacked-pane-favicon margin-right
		// Tab bar styles
		windowBackgroundColor,            // tab-bar background
		getStackTitleBorderColor(isDark), // tab-bar border-top
		int(32*uiScale),                  // tab-bar min-height
		tabModeColor,                     // window.tab-mode-active .tab-bar border-top-color (orange, distinct from pane mode blue)
		getStackTitleBg(isDark),          // tab-button background (same as stacked title)
		getStackTitleBorderColor(isDark), // tab-button border-right
		titlePaddingVertical,             // tab-button padding vertical
		titlePaddingHorizontal,           // tab-button padding horizontal
		styling.TransitionDuration,       // tab-button transition
		getStackTitleHoverBg(isDark),     // tab-button:hover background
		getActiveTabBg(isDark),           // tab-button-active background (much more visible)
		getStackTitleBorderColor(isDark), // tab-button-active border-right
		titleFontSize,                    // tab-title font-size
		getStackTitleTextColor(isDark),   // tab-title color
		windowBackgroundColor,            // tab-content-area background
		windowBackgroundColor,            // tab-workspace-container background
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
		if err := webkit.AddCSSProvider(workspaceCSS); err != nil {
			log.Printf("[workspace] WARNING: Failed to add CSS provider: %v", err)
		}
		globalCSSInitialized = true
		globalCSSContent = workspaceCSS
		log.Printf("[workspace] Applied workspace CSS styles (%d bytes)", len(workspaceCSS))
		log.Printf("[workspace] CSS preview: %s...", workspaceCSS[:min(200, len(workspaceCSS))])
	}

	wm.cssInitialized = true
}
