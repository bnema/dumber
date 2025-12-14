// Package theme provides GTK CSS styling for UI components.
package theme

import (
	"strings"
)

// GenerateCSS creates GTK4 CSS using the provided palette.
func GenerateCSS(p Palette) string {
	var sb strings.Builder

	// CSS custom properties (variables) - GTK4 uses :root selector
	sb.WriteString("/* Theme variables */\n")
	sb.WriteString(":root {\n")
	sb.WriteString(p.ToCSSVars())
	sb.WriteString("}\n\n")

	// Tab bar styling
	sb.WriteString(generateTabBarCSS(p))
	sb.WriteString("\n")

	// Omnibox styling
	sb.WriteString(generateOmniboxCSS(p))
	sb.WriteString("\n")

	// Pane styling
	sb.WriteString(generatePaneCSS(p))

	return sb.String()
}

// generateTabBarCSS creates tab bar styles.
func generateTabBarCSS(p Palette) string {
	return `/* Tab bar styling */
.tab-bar {
	background-color: var(--surface);
	border-top: 2px solid var(--border);
	padding: 0;
	min-height: 32px;
}

/* Tab button styling */
button.tab-button {
	background-color: var(--surface-variant);
	background-image: none;
	border: none;
	border-right: 1px solid var(--border);
	border-radius: 0;
	padding: 4px 8px;
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
	font-size: 12px;
	color: var(--text);
	font-weight: 500;
}
`
}

// generateOmniboxCSS creates omnibox styles.
func generateOmniboxCSS(p Palette) string {
	return `/* ===== Omnibox Styling ===== */

/* Omnibox window - floating popup */
window.omnibox-window {
	background-color: var(--surface-variant);
	border: 1px solid var(--border);
	border-radius: 3px;
}

/* Main container */
.omnibox-container {
	padding: 0;
	background-color: transparent;
}

/* Header with History/Favorites toggle */
.omnibox-header {
	background-color: shade(var(--surface-variant), 1.1);
	border-bottom: 1px solid var(--border);
	padding: 6px 12px;
}

.omnibox-header-btn {
	background-color: transparent;
	background-image: none;
	border: none;
	border-radius: 2px;
	padding: 4px 12px;
	margin-right: 8px;
	font-size: 13px;
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
	border: 1px solid var(--border);
	border-radius: 2px;
	padding: 10px 12px;
	margin: 8px 12px;
	font-size: 16px;
	caret-color: var(--accent);
}

.omnibox-entry:focus {
	border-color: var(--accent);
	background-color: shade(var(--bg), 1.05);
}

/* Scrolled window for suggestions */
.omnibox-scrolled {
	background-color: shade(var(--surface-variant), 0.95);
	border-top: 1px solid var(--border);
}

/* List box */
.omnibox-listbox {
	background-color: transparent;
}

/* Suggestion rows */
.omnibox-row {
	padding: 8px 12px;
	margin: 0;
	border-radius: 0;
	border-left: 3px solid transparent;
	border-bottom: 1px solid alpha(var(--border), 0.5);
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

/* Suggestion title/URL */
.omnibox-suggestion-title {
	font-size: 14px;
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

/* Keyboard shortcut badge */
.omnibox-shortcut-badge {
	background-color: alpha(var(--muted, #909090), 0.3);
	color: var(--muted, #909090);
	border-radius: 4px;
	padding: 2px 6px;
	font-size: 10px;
	font-weight: 500;
	font-family: monospace;
	margin-left: 8px;
}

.omnibox-row:hover .omnibox-shortcut-badge {
	background-color: alpha(var(--accent, #4ade80), 0.2);
	color: var(--accent, #4ade80);
}

.omnibox-row:selected .omnibox-shortcut-badge {
	background-color: alpha(var(--accent, #4ade80), 0.3);
	color: var(--accent, #4ade80);
}
`
}

// generatePaneCSS creates pane border styles.
func generatePaneCSS(p Palette) string {
	return `/* ===== Pane Styling ===== */

/* Pane border - default transparent */
.pane-border {
	border: 1px solid transparent;
}

/* Active pane - accent border */
.pane-active {
	border: 1px solid var(--accent);
}

/* Pane mode active - thick blue border */
.pane-mode-active {
	border: 4px solid #4A90E2;
}

/* Tab mode active - thick orange border */
.tab-mode-active {
	border: 4px solid #FFA500;
}
`
}
