// workspace_css.go - CSS generation and styling for workspace panes
package browser

import (
	"fmt"
	"log"

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

// generateActivePaneCSS generates the CSS for workspace panes based on config
func (wm *WorkspaceManager) generateActivePaneCSS() string {
	cfg := config.Get()
	styling := cfg.Workspace.Styling

	// Get appropriate border colors based on GTK theme preference
	var inactiveBorderColor, windowBackgroundColor string
	isDark := webkit.PrefersDarkTheme()
	if isDark {
		inactiveBorderColor = "#333333"   // Dark border for dark theme
		windowBackgroundColor = "#2b2b2b" // Dark window background
	} else {
		inactiveBorderColor = "#dddddd"   // Light border for light theme
		windowBackgroundColor = "#ffffff" // Light window background
	}

	// Log the border color values for debugging
	log.Printf("[workspace] GTK prefers dark: %v, inactive border color: %s", isDark, inactiveBorderColor)

	css := fmt.Sprintf(`window {
	  background-color: %s;
	}

	.workspace-pane {
	  background-color: %s;
	  border: %dpx solid %s;
	  transition: border-color %dms ease-in-out;
	  border-radius: %dpx;
	}

	.workspace-pane.workspace-multi-pane {
	  border: %dpx solid %s;
	  border-radius: %dpx;
	}

	.workspace-pane.workspace-multi-pane.workspace-pane-active {
	  border-color: %s;
	}

	/* Active border for ALL workspace panes (including stacked panes) */
	.workspace-pane.workspace-pane-active {
	  border-color: %s !important;
	}

	/* Stacked panes styling */
	.stacked-pane-container {
	  background-color: %s;
	  border-radius: %dpx;
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
		windowBackgroundColor,
		windowBackgroundColor,
		styling.BorderWidth,
		inactiveBorderColor,
		styling.TransitionDuration,
		styling.BorderRadius,
		styling.BorderWidth,
		inactiveBorderColor,
		styling.BorderRadius,
		styling.BorderColor,
		styling.BorderColor, // NEW: .workspace-pane.workspace-pane-active border-color
		// Additional parameters for stacked pane styles
		windowBackgroundColor,          // stacked-pane-container background
		styling.BorderRadius,           // stacked-pane-container border-radius
		getStackTitleBg(isDark),        // stacked-pane-title background
		inactiveBorderColor,            // stacked-pane-title border-bottom
		styling.TransitionDuration,     // stacked-pane-title transition
		getStackTitleHoverBg(isDark),   // stacked-pane-title:hover background
		getStackTitleTextColor(isDark), // stacked-pane-title-text color
	)

	// Log the actual CSS being generated
	log.Printf("[workspace] Generated CSS: %s", css)

	return css
}

// ensureStyles ensures that CSS styles are applied to the workspace
func (wm *WorkspaceManager) ensureStyles() {
	if wm == nil || wm.cssInitialized {
		return
	}
	activePaneCSS := wm.generateActivePaneCSS()
	webkit.AddCSSProvider(activePaneCSS)
	wm.cssInitialized = true
}

// ensurePaneBaseClasses ensures all panes have the proper base CSS classes
func (wm *WorkspaceManager) ensurePaneBaseClasses() {
	if wm == nil {
		return
	}

	leaves := wm.collectLeaves()
	for _, leaf := range leaves {
		if leaf != nil && leaf.container != nil {
			leaf.container.Execute(func(containerPtr uintptr) error {
				webkit.WidgetAddCSSClass(containerPtr, basePaneClass)
				if wm.hasMultiplePanes() {
					webkit.WidgetAddCSSClass(containerPtr, multiPaneClass)
				} else {
					webkit.WidgetRemoveCSSClass(containerPtr, multiPaneClass)
				}
				return nil
			})
		}
	}
}
