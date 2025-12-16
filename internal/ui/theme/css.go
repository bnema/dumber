// Package theme provides GTK CSS styling for UI components.
package theme

import (
	"fmt"
	"strings"
)

// GenerateCSS creates GTK4 CSS using the provided palette with default scale.
func GenerateCSS(p Palette) string {
	return GenerateCSSWithScale(p, 1.0)
}

// GenerateCSSWithScale creates GTK4 CSS using the provided palette and UI scale factor.
// Scale affects font sizes and widget padding/margins proportionally.
func GenerateCSSWithScale(p Palette, scale float64) string {
	if scale <= 0 {
		scale = 1.0
	}

	var sb strings.Builder

	// CSS custom properties (variables) - GTK4 uses :root selector
	sb.WriteString("/* Theme variables */\n")
	sb.WriteString(":root {\n")
	sb.WriteString(p.ToCSSVars())
	sb.WriteString("}\n\n")

	// Global UI scaling via font-size on all widgets
	if scale != 1.0 {
		sb.WriteString(generateScalingCSS(scale))
		sb.WriteString("\n")
	}

	// Tab bar styling
	sb.WriteString(generateTabBarCSS(p))
	sb.WriteString("\n")

	// Omnibox styling
	sb.WriteString(generateOmniboxCSS(p))
	sb.WriteString("\n")

	// Pane styling
	sb.WriteString(generatePaneCSS(p))
	sb.WriteString("\n")

	// Stacked pane styling
	sb.WriteString(generateStackedPaneCSS(p))

	return sb.String()
}

// generateScalingCSS creates CSS rules that scale the UI.
// GTK4 CSS doesn't support transform:scale() well, so we scale font-size
// and use relative units (em) for padding/margins in other rules.
func generateScalingCSS(scale float64) string {
	// Use 16px base so em conversions are correct:
	// 1px = 0.0625em (1/16), 4px = 0.25em, etc.
	basePx := int(16 * scale)
	return fmt.Sprintf(`/* UI Scaling (%.0f%%) */
window {
	font-size: %dpx;
}
`, scale*100, basePx)
}

// generateTabBarCSS creates tab bar styles.
// Uses em units for scalable UI.
func generateTabBarCSS(p Palette) string {
	return `/* Tab bar styling */
.tab-bar {
	background-color: var(--surface);
	border-top: 0.125em solid var(--border);
	padding: 0;
	min-height: 2em;
}

/* Tab button styling */
button.tab-button {
	background-color: var(--surface-variant);
	background-image: none;
	border: none;
	border-right: 0.0625em solid var(--border);
	border-radius: 0;
	padding: 0.25em 0.5em;
	transition: background-color 200ms ease-in-out;
}

button.tab-button:hover {
	background-color: shade(var(--surface-variant), 1.2);
}

button.tab-button.tab-button-active {
	background-color: shade(var(--surface-variant), 1.4);
	font-weight: 600;
}

/* Tab title text */
.tab-title {
	font-size: 0.75em;
	color: var(--text);
	font-weight: 500;
}
`
}

// generateOmniboxCSS creates omnibox styles.
// Uses em units for scalable UI.
func generateOmniboxCSS(p Palette) string {
	return `/* ===== Omnibox Styling ===== */

/* Omnibox window - floating popup */
window.omnibox-window {
	background-color: var(--surface-variant);
	border: 0.0625em solid var(--border);
	border-radius: 0.1875em;
}

/* Main container */
.omnibox-container {
	padding: 0;
	background-color: transparent;
}

/* Header with History/Favorites toggle */
.omnibox-header {
	background-color: shade(var(--surface-variant), 1.1);
	border-bottom: 0.0625em solid var(--border);
	padding: 0.375em 0.75em;
}

.omnibox-header-btn {
	background-color: transparent;
	background-image: none;
	border: none;
	border-radius: 0.125em;
	padding: 0.25em 0.75em;
	margin-right: 0.5em;
	font-size: 0.8125em;
	font-weight: 500;
	color: var(--muted);
	transition: all 100ms ease-in-out;
}

.omnibox-header-btn:hover {
	background-color: alpha(var(--accent), 0.15);
	color: var(--text);
}

.omnibox-header-btn.omnibox-header-active {
	background-color: alpha(var(--accent), 0.2);
	color: var(--accent);
	font-weight: 600;
}

/* Search entry field */
.omnibox-entry {
	background-color: var(--bg);
	color: var(--text);
	border: 0.0625em solid var(--border);
	border-radius: 0.125em;
	padding: 0.625em 0.75em;
	margin: 0.5em 0.75em;
	font-size: 1em;
	caret-color: var(--accent);
}

.omnibox-entry:focus {
	border-color: var(--accent);
	background-color: shade(var(--bg), 1.05);
}

/* Scrolled window for suggestions */
.omnibox-scrolled {
	background-color: shade(var(--surface-variant), 0.95);
	border-top: 0.0625em solid var(--border);
}

/* List box */
.omnibox-listbox {
	background-color: transparent;
}

/* Suggestion rows */
.omnibox-row {
	padding: 0.5em 0.75em;
	margin: 0;
	border-radius: 0;
	border-left: 0.1875em solid transparent;
	border-bottom: 0.0625em solid alpha(var(--border), 0.5);
	transition: background-color 100ms ease-in-out, border-left-color 100ms ease-in-out;
}

.omnibox-row:last-child {
	border-bottom: none;
}

.omnibox-row:hover {
	background-color: alpha(var(--accent), 0.12);
	border-left-color: var(--accent);
}

.omnibox-row:selected {
	background-color: alpha(var(--accent), 0.2);
	border-left-color: var(--accent);
}

/* Favicon in omnibox rows */
.omnibox-favicon {
	min-width: 1em;
	min-height: 1em;
	margin-right: 0.5em;
}

/* Suggestion title/URL */
.omnibox-suggestion-title {
	font-size: 0.875em;
	color: var(--text, #ffffff);
	font-weight: 400;
}

/* Also style labels inside omnibox rows directly */
.omnibox-row label {
	color: var(--text, #ffffff);
}

.omnibox-row:selected .omnibox-suggestion-title {
	color: var(--text);
	font-weight: 500;
}

/* URL text below title */
.omnibox-suggestion-url {
	font-size: 0.75em;
	color: var(--muted);
	font-weight: 400;
}

.omnibox-row:selected .omnibox-suggestion-url {
	color: var(--muted);
}

/* Keyboard shortcut badge */
.omnibox-shortcut-badge {
	background-color: alpha(var(--muted, #909090), 0.3);
	color: var(--muted, #909090);
	border-radius: 0.25em;
	padding: 0.125em 0.375em;
	font-size: 0.625em;
	font-weight: 500;
	font-family: monospace;
	margin-left: 0.5em;
}

.omnibox-row:hover .omnibox-shortcut-badge {
	background-color: alpha(var(--accent, #4ade80), 0.2);
	color: var(--accent, #4ade80);
}

.omnibox-row:selected .omnibox-shortcut-badge {
	background-color: alpha(var(--accent, #4ade80), 0.3);
	color: var(--accent, #4ade80);
}

/* Zoom indicator in omnibox header */
.omnibox-zoom-indicator {
	background-color: alpha(var(--accent, #4ade80), 0.2);
	color: var(--accent, #4ade80);
	border-radius: 0.25em;
	padding: 0.125em 0.5em;
	font-size: 0.6875em;
	font-weight: 600;
	margin-left: auto;
}
`
}

// generatePaneCSS creates pane border styles.
// Uses em units for scalable UI.
func generatePaneCSS(p Palette) string {
	return `/* ===== Pane Styling ===== */

/* Pane border - default transparent */
.pane-border {
	border: 0.0625em solid transparent;
}

/* Active pane - accent border */
.pane-active {
	border: 0.0625em solid var(--accent);
}

/* Pane mode active - thick blue inset border (for overlay) */
.pane-mode-active {
	background-color: transparent;
	box-shadow: inset 0 0 0 0.25em #4A90E2;
	border-radius: 0;
}

/* Tab mode active - thick orange inset border (for overlay) */
.tab-mode-active {
	background-color: transparent;
	box-shadow: inset 0 0 0 0.25em #FFA500;
	border-radius: 0;
}
`
}

// generateStackedPaneCSS creates stacked pane (Zellij-style tabs within panes) styles.
// Uses em units for scalable UI.
func generateStackedPaneCSS(p Palette) string {
	return `/* ===== Stacked Pane Styling ===== */

/* Title bar for inactive panes in a stack */
.stacked-pane-titlebar {
	background-color: var(--surface-variant);
	border-bottom: 0.0625em solid var(--border);
	padding: 0.25em 0.5em;
	min-height: 1.5em;
}

/* Title bar button wrapper - clickable area */
button.stacked-pane-title-button {
	background-color: var(--surface-variant);
	background-image: none;
	border: none;
	border-bottom: 0.0625em solid var(--border);
	border-radius: 0;
	padding: 0;
	margin: 0;
	transition: background-color 150ms ease-in-out;
}

button.stacked-pane-title-button:hover {
	background-color: alpha(var(--accent), 0.15);
}

button.stacked-pane-title-button:focus {
	outline: none;
	box-shadow: none;
}

/* Title bar content box */
.stacked-pane-titlebar.active {
	background-color: alpha(var(--accent), 0.1);
	border-left: 0.1875em solid var(--accent);
}

/* Favicon image in title bar */
.stacked-pane-titlebar image {
	min-width: 1em;
	min-height: 1em;
	margin-right: 0.375em;
}

/* Title text in title bar */
.stacked-pane-titlebar label {
	color: var(--text);
	font-size: 0.75em;
	font-weight: 400;
}

button.stacked-pane-title-button:hover .stacked-pane-titlebar label {
	color: var(--accent);
}
`
}
