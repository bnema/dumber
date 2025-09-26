package browser

import (
	"fmt"
	"log"

	"github.com/bnema/dumber/internal/config"
	"github.com/bnema/dumber/pkg/webkit"
)

const (
	basePaneClass  = "workspace-pane"
	multiPaneClass = "workspace-multi-pane"
)

// CSSManager handles CSS styling and visual state
type CSSManager struct {
	wm             *WorkspaceManager
	cssInitialized bool
}

// NewCSSManager creates a new CSS manager
func NewCSSManager(wm *WorkspaceManager) *CSSManager {
	return &CSSManager{wm: wm}
}

// EnsureStyles initializes CSS styles for the workspace
func (cm *CSSManager) EnsureStyles() {
	if cm.cssInitialized {
		return
	}

	styling := config.GetStyling()
	if styling == nil {
		log.Printf("[css] No styling config found, using defaults")
		return
	}

	css := cm.generateActivePaneCSS(styling)

	if cm.wm.window != nil {
		if err := cm.wm.window.InjectCSS(css); err != nil {
			log.Printf("[css] Failed to inject workspace CSS: %v", err)
		} else {
			log.Printf("[css] Injected workspace CSS")
		}
	}

	cm.cssInitialized = true
}

// generateActivePaneCSS creates CSS for active pane styling
func (cm *CSSManager) generateActivePaneCSS(styling *config.StylingConfig) string {
	// Get stack title colors
	stackTitleBg := getStackTitleBg(styling)
	stackTitleHoverBg := getStackTitleHoverBg(styling)
	stackTitleTextColor := getStackTitleTextColor(styling)

	return fmt.Sprintf(`
/* Base pane styling */
.%s {
	margin: 2px;
	border-radius: 4px;
	transition: border-color 0.2s ease;
}

/* Active pane border */
.workspace-pane-active {
	border: 2px solid %s !important;
	border-radius: 4px;
}

/* Active border for stacked panes (they use different CSS class) */
.stacked-pane-active {
	border: 2px solid %s !important;
	border-radius: 4px;
}

/* Multi-pane workspace styling */
.%s {
	/* Additional multi-pane specific styling */
}

/* Stacked pane title styling */
.stacked-pane-title {
	background-color: %s;
	color: %s;
	padding: 4px 8px;
	border-radius: 4px 4px 0 0;
	font-size: 12px;
	font-weight: 500;
	cursor: pointer;
	transition: background-color 0.2s ease;
}

.stacked-pane-title:hover {
	background-color: %s;
}

.stacked-pane-title-text {
	font-family: monospace;
	font-size: 11px;
}

/* Collapsed stacked pane styling */
.stacked-pane-collapsed {
	/* Additional styling for collapsed panes */
}
`,
		basePaneClass,
		styling.BorderColor,   // Regular active pane border
		styling.BorderColor,   // Stacked pane active border
		multiPaneClass,
		stackTitleBg,         // Title background
		stackTitleTextColor,  // Title text color
		stackTitleHoverBg,    // Title hover background
	)
}

// EnsurePaneBaseClasses ensures all panes have proper base CSS classes
func (cm *CSSManager) EnsurePaneBaseClasses() {
	leaves := cm.wm.layoutManager.CollectLeaves()
	hasMultiple := len(leaves) > 1

	for _, leaf := range leaves {
		if leaf == nil || leaf.container == nil {
			continue
		}

		leaf.container.Execute(func(containerPtr uintptr) error {
			// Add base class
			webkit.WidgetAddCSSClass(containerPtr, basePaneClass)

			// Add/remove multi-pane class based on count
			if hasMultiple {
				webkit.WidgetAddCSSClass(containerPtr, multiPaneClass)
			} else {
				webkit.WidgetRemoveCSSClass(containerPtr, multiPaneClass)
			}

			return nil
		})
	}
}

// getStackTitleBg returns the background color for stack titles
func getStackTitleBg(styling *config.StylingConfig) string {
	if styling.StackTitleBg != "" {
		return styling.StackTitleBg
	}
	return "#2d2d2d" // Default dark background
}

// getStackTitleHoverBg returns the hover background color for stack titles
func getStackTitleHoverBg(styling *config.StylingConfig) string {
	if styling.StackTitleHoverBg != "" {
		return styling.StackTitleHoverBg
	}
	return "#404040" // Default hover background
}

// getStackTitleTextColor returns the text color for stack titles
func getStackTitleTextColor(styling *config.StylingConfig) string {
	if styling.StackTitleTextColor != "" {
		return styling.StackTitleTextColor
	}
	return "#ffffff" // Default white text
}