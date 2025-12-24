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
	return GenerateCSSWithScaleAndFonts(p, scale, DefaultFontConfig())
}

// FontConfig holds font settings for CSS generation.
type FontConfig struct {
	SansFont      string
	MonospaceFont string
}

// DefaultFontConfig returns safe font fallbacks.
func DefaultFontConfig() FontConfig {
	return FontConfig{
		SansFont:      "sans-serif",
		MonospaceFont: "monospace",
	}
}

// FontCSSVars generates CSS custom properties for fonts.
func FontCSSVars(fonts FontConfig) string {
	var sb strings.Builder
	// Quote the configured family; include generic fallback.
	sb.WriteString(fmt.Sprintf("  --font-sans: %q, sans-serif;\n", fonts.SansFont))
	sb.WriteString(fmt.Sprintf("  --font-mono: %q, monospace;\n", fonts.MonospaceFont))
	return sb.String()
}

func generateFontCSS() string {
	return `/* Font styling */
window, tooltip, popover {
	font-family: var(--font-sans);
}
`
}

// GenerateCSSWithScaleAndFonts creates GTK4 CSS using the provided palette, UI scale factor and fonts.
// Scale affects font sizes and widget padding/margins proportionally.
func GenerateCSSWithScaleAndFonts(p Palette, scale float64, fonts FontConfig) string {
	if scale <= 0 {
		scale = 1.0
	}

	defaults := DefaultFontConfig()
	fonts = FontConfig{
		SansFont:      Coalesce(fonts.SansFont, defaults.SansFont),
		MonospaceFont: Coalesce(fonts.MonospaceFont, defaults.MonospaceFont),
	}

	var sb strings.Builder

	// CSS custom properties (variables) - GTK4 uses :root selector
	sb.WriteString("/* Theme variables */\n")
	sb.WriteString(":root {\n")
	sb.WriteString(p.ToCSSVars())
	sb.WriteString(FontCSSVars(fonts))
	sb.WriteString("}\n\n")

	// Global font styling
	sb.WriteString(generateFontCSS())
	sb.WriteString("\n")

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

	// Find bar styling
	sb.WriteString(generateFindBarCSS(p))
	sb.WriteString("\n")

	// Pane styling
	sb.WriteString(generatePaneCSS(p))
	sb.WriteString("\n")

	// Stacked pane styling
	sb.WriteString(generateStackedPaneCSS(p))
	sb.WriteString("\n")

	// Progress bar styling
	sb.WriteString(generateProgressBarCSS(p))
	sb.WriteString("\n")

	// Toaster styling
	sb.WriteString(generateToasterCSS(p))
	sb.WriteString("\n")

	// Session manager styling
	sb.WriteString(generateSessionManagerCSS(p))

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
	background-color: shade(var(--surface-variant), 1.2);
	border-bottom: 0.125em solid var(--accent);
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

/* Omnibox outer container - for positioning in overlay */
.omnibox-outer {
	/* Positioning is handled via SetHalign/SetValign/SetMarginTop in Go */
}

/* Omnibox main container - the visible popup */
.omnibox-container {
	background-color: var(--surface-variant);
	border: 0.0625em solid var(--border);
	border-radius: 0.1875em;
	padding: 0;
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

entry.omnibox-entry:focus,
entry.omnibox-entry:focus-within,
entry.omnibox-entry:focus-visible {
	border-color: var(--accent);
	background-color: shade(var(--bg), 1.05);
	outline-style: none;
	outline-width: 0;
	outline-color: transparent;
}

/* Override any internal focus styling */
entry.omnibox-entry > *:focus,
entry.omnibox-entry > *:focus-visible {
	outline-style: none;
	outline-color: transparent;
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
	transition: background-color 100ms ease-in-out, border-left 100ms ease-in-out;
	min-height: 2.75em;
}

.omnibox-row:last-child {
	border-bottom: none;
}

.omnibox-row:hover {
	background-color: alpha(var(--accent), 0.12);
	border-left: 0.1875em solid var(--accent);
}

.omnibox-row:selected {
	background-color: alpha(var(--accent), 0.2);
	border-left: 0.1875em solid var(--accent);
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
	color: var(--text);
	font-weight: 400;
}

/* Also style labels inside omnibox rows directly */
.omnibox-row label {
	color: var(--text);
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
	background-color: alpha(var(--muted), 0.3);
	color: var(--muted);
	border-radius: 0.25em;
	padding: 0.125em 0.375em;
	font-size: 0.625em;
	font-weight: 500;
	font-family: var(--font-mono);
	margin-left: 0.5em;
}

.omnibox-row:hover .omnibox-shortcut-badge {
	background-color: alpha(var(--accent), 0.2);
	color: var(--accent);
}

.omnibox-row:selected .omnibox-shortcut-badge {
	background-color: alpha(var(--accent), 0.3);
	color: var(--accent);
}

/* Zoom indicator in omnibox header */
.omnibox-zoom-indicator {
	background-color: alpha(var(--accent), 0.2);
	color: var(--accent);
	border-radius: 0.25em;
	padding: 0.125em 0.5em;
	font-size: 0.6875em;
	font-weight: 600;
	margin-left: auto;
}
`
}

// generateFindBarCSS creates find bar styles.
// Uses em units for scalable UI.
func generateFindBarCSS(p Palette) string {
	_ = p
	return `/* ===== Find Bar Styling ===== */
.find-bar-outer {
	margin: 0.5em;
}

.find-bar-container {
	background-color: var(--surface-variant);
	border: 0.0625em solid var(--border);
	border-radius: 0.25em;
	box-shadow: 0 0.25em 0.75em alpha(black, 0.2);
	padding: 0.5em;
	min-width: 20em;
}

.find-bar-input-row {
	margin-bottom: 0.25em;
}

.find-bar-entry {
	background-color: var(--bg);
	color: var(--text);
	border: 0.0625em solid var(--border);
	border-radius: 0.1875em;
	padding: 0.4em 0.5em;
	caret-color: var(--accent);
}

entry.find-bar-entry:focus,
entry.find-bar-entry:focus-within,
entry.find-bar-entry:focus-visible {
	border-color: var(--accent);
	outline-style: none;
	outline-width: 0;
	outline-color: transparent;
}

.find-bar-entry.not-found {
	background-color: alpha(#ef4444, 0.15);
	border-color: #ef4444;
}

.find-bar-count {
	font-size: 0.8em;
	color: var(--muted);
	min-width: 3.5em;
}

.find-bar-count.find-bar-count-has {
	color: var(--accent);
}

.find-bar-nav,
.find-bar-close,
.find-bar-toggle {
	background-image: none;
	background-color: alpha(var(--bg), 0.6);
	border: 0.0625em solid var(--border);
	border-radius: 0.1875em;
	padding: 0.25em 0.5em;
	transition: background-color 120ms ease-in-out, border-color 120ms ease-in-out;
}

.find-bar-nav:hover,
.find-bar-close:hover,
.find-bar-toggle:hover {
	background-color: alpha(var(--accent), 0.15);
}

.find-bar-toggle:checked {
	background-color: alpha(var(--accent), 0.2);
	border-color: var(--accent);
	color: var(--accent);
}
`
}

// generatePaneCSS creates pane border styles.
// Uses em units for scalable UI.
func generatePaneCSS(p Palette) string {
	return `/* ===== Pane Styling ===== */

/* Pane overlay container - theme background prevents white flash */
.pane-overlay {
	background-color: var(--bg);
}

/* Pane border - default transparent */
.pane-border {
	border: 0.0625em solid transparent;
}

/* Active pane - accent border */
.pane-active {
	border: 0.0625em solid var(--accent);
}

/* Hide active border when only one pane exists */
.single-pane .pane-active {
	border-color: transparent;
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
	background-color: shade(var(--surface-variant), 1.15);
}

button.stacked-pane-title-button:focus {
	outline: none;
	box-shadow: none;
}

/* Title bar content box */
.stacked-pane-titlebar.active {
	background-color: shade(var(--surface-variant), 1.2);
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

// generateProgressBarCSS creates progress bar styles for page loading indication.
func generateProgressBarCSS(p Palette) string {
	return `/* ===== Progress Bar Styling ===== */

/* Native GtkProgressBar with osd class */
progressbar.osd {
	min-height: 4px;
}

progressbar.osd trough {
	min-height: 4px;
	min-width: 0px;
	background-color: alpha(var(--bg), 0.3);
}

progressbar.osd progress {
	min-height: 4px;
	min-width: 0px;
	background-color: var(--accent);
}
`
}

// generateSessionManagerCSS creates session manager modal styles.
// Uses em units for scalable UI, matches omnibox styling patterns.
func generateSessionManagerCSS(p Palette) string {
	return `/* ===== Session Manager Styling ===== */

/* Session manager outer container - for positioning in overlay */
.session-manager-outer {
	/* Positioning is handled via SetHalign/SetValign in Go */
}

/* Session manager main container - the visible popup */
.session-manager-container {
	background-color: var(--surface-variant);
	border: 0.0625em solid var(--border);
	border-radius: 0.1875em;
	padding: 0;
	min-width: 28em;
	max-width: 40em;
}

/* Header with title */
.session-manager-header {
	background-color: shade(var(--surface-variant), 1.1);
	border-bottom: 0.0625em solid var(--border);
	padding: 0.5em 0.75em;
}

.session-manager-title {
	font-size: 0.9375em;
	font-weight: 600;
	color: var(--text);
}

/* Scrolled window for session list */
.session-manager-scrolled {
	background-color: shade(var(--surface-variant), 0.95);
}

/* List box */
.session-manager-list {
	background-color: transparent;
}

/* Session rows */
.session-manager-row {
	padding: 0.5em 0.75em;
	margin: 0;
	border-radius: 0;
	border-left: 0.1875em solid transparent;
	border-bottom: 0.0625em solid alpha(var(--border), 0.5);
	transition: background-color 100ms ease-in-out, border-left 100ms ease-in-out;
	min-height: 2.75em;
}

.session-manager-row:last-child {
	border-bottom: none;
}

.session-manager-row:hover {
	background-color: alpha(var(--accent), 0.12);
	border-left: 0.1875em solid var(--accent);
}

.session-manager-row:selected,
row:selected .session-manager-row {
	background-color: alpha(var(--accent), 0.2);
	border-left: 0.1875em solid var(--accent);
}

/* Status indicator dot */
.session-status {
	font-size: 0.875em;
	min-width: 1em;
	margin-right: 0.5em;
}

/* Current session - accent color dot */
.session-current .session-status {
	color: var(--accent);
}

/* Active session (other instance) - muted dot */
.session-active .session-status {
	color: var(--muted);
}

/* Exited session - no dot, dimmed */
.session-exited {
	opacity: 0.7;
}

.session-exited .session-status {
	color: transparent;
}

/* Session ID label */
.session-id {
	font-size: 0.875em;
	font-weight: 500;
	color: var(--text);
	font-family: var(--font-mono);
}

.session-current .session-id {
	color: var(--accent);
}

/* Tab/pane count label */
.session-count {
	font-size: 0.75em;
	color: var(--muted);
}

/* Relative time label */
.session-time {
	font-size: 0.75em;
	color: var(--muted);
	margin-left: auto;
}

/* Section divider for EXITED sessions */
.session-divider {
	font-size: 0.6875em;
	font-weight: 600;
	color: var(--muted);
	text-transform: uppercase;
	letter-spacing: 0.05em;
	padding: 0.5em 0.75em 0.25em 0.75em;
	background-color: alpha(var(--border), 0.3);
	border-bottom: 0.0625em solid var(--border);
}

/* Footer with keyboard shortcuts */
.session-manager-footer {
	background-color: shade(var(--surface-variant), 0.9);
	border-top: 0.0625em solid var(--border);
	padding: 0.375em 0.75em;
	font-size: 0.6875em;
	color: var(--muted);
	font-family: var(--font-mono);
}

/* Session mode border (purple) */
.session-mode-active {
	background-color: transparent;
	box-shadow: inset 0 0 0 0.25em #9B59B6;
	border-radius: 0;
}
`
}

// generateToasterCSS creates toast notification styles.
// Uses em units for scalable UI.
func generateToasterCSS(p Palette) string {
	return `/* ===== Toaster Styling ===== */

/* Toast notification container */
.toast {
	background-color: var(--surface-variant);
	border-radius: 0.25em;
	padding: 0.375em 0.625em;
	margin: 0.5em;
	font-size: 0.875em;
	font-weight: 600;
	box-shadow: 0 2px 8px alpha(black, 0.3);
	min-width: 2.5em;
}

/* Toast level: info (default, accent color) */
.toast-info {
	background-color: alpha(var(--accent), 0.9);
	color: var(--bg);
}

/* Toast level: success */
.toast-success {
	background-color: alpha(var(--success), 0.9);
	color: var(--bg);
}

/* Toast level: warning */
.toast-warning {
	background-color: alpha(var(--warning), 0.9);
	color: var(--bg);
}

/* Toast level: error */
.toast-error {
	background-color: alpha(var(--destructive), 0.9);
	color: var(--bg);
}
`
}
