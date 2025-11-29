// workspace_css.go - CSS generation and styling for workspace panes
package browser

import (
	"fmt"
	"strings"

	"github.com/bnema/dumber/internal/config"
	"github.com/bnema/dumber/internal/logging"
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

// getEntryBg returns the background color for text entry widgets
func getEntryBg(isDark bool) string {
	if isDark {
		return "#2a2a2a" // Darker background for dark theme
	}
	return "#e0e0e0" // Darker gray for light theme
}

// getEntryFocusBg returns the background color for focused entry widgets
func getEntryFocusBg(isDark bool) string {
	if isDark {
		return "#323232" // Slightly lighter when focused
	}
	return "#f5f5f5" // Very light gray when focused
}

// getEntryBorderColor returns the border color for entry widgets
func getEntryBorderColor(isDark bool) string {
	if isDark {
		return "#555555" // Subtle border for dark theme
	}
	return "#cccccc" // Subtle border for light theme
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
	var windowBackgroundColor string
	isDark := webkit.PrefersDarkTheme()
	if isDark {
		windowBackgroundColor = "#2b2b2b" // Dark window background
	} else {
		windowBackgroundColor = "#ffffff" // Light window background
	}

	activeBorderColor := getActivePaneBorderColor(styling, isDark)

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

	/* Pane mode border overlay - floats over content without layout shift */
	.pane-mode-border {
	  border: %dpx solid %s;
	  border-radius: 0;
	  background-color: transparent;
	  pointer-events: none;
	}

	/* Tab mode border overlay - floats over content without layout shift */
	.tab-mode-border {
	  border: %dpx solid %s;
	  border-radius: 0;
	  background-color: transparent;
	  pointer-events: none;
	}

	/* Pane border overlay - active pane indicator without layout shift */
	.pane-border-overlay {
	  border: %dpx solid %s;
	  border-radius: 0;
	  background-color: transparent;
	  pointer-events: none;
	  transition-property: opacity;
	  transition-duration: %dms;
	  transition-timing-function: ease-in-out;
	}

	/* Workspace root container */
	paned, box {
	  background-color: %s;
	}

	/* Base pane styling - no borders (using overlays instead) */
	.workspace-pane, .stacked-pane-container {
	  border: none;
	  margin: 0;
	}

	/* Stacked panes styling */
	.stacked-pane-container {
	  background-color: %s;
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

	/* Tab progress bar */
	progressbar.tab-progress-bar {
	  min-height: 4px;
	  padding: 0;
	  margin: 0;
	  border: none;
	  transition: opacity 180ms ease-in-out;
	}

	progressbar.tab-progress-bar trough {
	  min-height: 4px;
	  background: transparent;
	  border: none;
	  padding: 0;
	}

	progressbar.tab-progress-bar progress {
	  min-height: 4px;
	  border-radius: 0;
	  background-image: linear-gradient(90deg, %s 0%%, %s 50%%, %s 100%%);
	  background-size: 240%% 100%%;
	  background-position: 0%% 0%%;
	  transition: width 180ms ease-in-out, background-position 420ms linear;
	  animation: tab-progress-stripe 1.3s linear infinite;
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

	/* Tab rename entry */
	entry.tab-rename-entry {
	  background-color: %s;
	  color: %s;
	  font-size: %dpx;
	  padding: 2px %dpx;
	  border: 1px solid %s;
	  border-radius: 3px;
	  box-shadow: inset 1px 1px 3px rgba(0, 0, 0, 0.4);
	  min-width: 80px;
	  min-height: 0;
	}

	entry.tab-rename-entry:focus {
	  background-color: %s;
	  border-color: %s;
	  box-shadow: inset 1px 1px 4px rgba(0, 0, 0, 0.5);
	  outline: none;
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
	}

	@keyframes tab-progress-stripe {
	  0%% { background-position: 0%% 0%%; }
	  100%% { background-position: -100%% 0%%; }
	}`,
		windowBackgroundColor,          // window background
		styling.PaneModeBorderWidth,    // .pane-mode-border border-width
		paneModeColor,                  // .pane-mode-border border-color
		styling.TabModeBorderWidth,     // .tab-mode-border border-width
		tabModeColor,                   // .tab-mode-border border-color
		styling.BorderWidth,            // .pane-border-overlay border-width
		activeBorderColor,              // .pane-border-overlay border-color
		styling.TransitionDuration,     // .pane-border-overlay transition-duration
		windowBackgroundColor,          // paned, box background
		windowBackgroundColor,          // stacked-pane-container background
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
		"#606060",                        // tab progress gradient start (neutral gray)
		"#747474",                        // tab progress gradient middle (neutral gray)
		"#8a8a8a",                        // tab progress gradient end (neutral gray)
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
		// Tab rename entry styles
		getEntryBg(isDark),               // entry.tab-rename-entry background
		getStackTitleTextColor(isDark),   // entry.tab-rename-entry color (text)
		titleFontSize,                    // entry.tab-rename-entry font-size
		titlePaddingHorizontal,           // entry.tab-rename-entry padding horizontal (vertical is hardcoded 2px)
		getEntryBorderColor(isDark),      // entry.tab-rename-entry border-color
		getEntryFocusBg(isDark),          // entry.tab-rename-entry:focus background
		activeBorderColor,                // entry.tab-rename-entry:focus border-color (use active pane color)
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
			logging.Error(fmt.Sprintf("[workspace] WARNING: Failed to add CSS provider: %v", err))
		}
		globalCSSInitialized = true
		globalCSSContent = workspaceCSS
		logging.Info(fmt.Sprintf("[workspace] Applied workspace CSS styles (%d bytes)", len(workspaceCSS)))

		previewLen := 200
		if len(workspaceCSS) < previewLen {
			previewLen = len(workspaceCSS)
		}
		logging.Info(fmt.Sprintf("[workspace] CSS preview: %s...", workspaceCSS[:previewLen]))
	}

	wm.cssInitialized = true
}
