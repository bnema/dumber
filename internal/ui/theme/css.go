// Package theme provides GTK CSS styling for UI components.
package theme

import (
	"github.com/jwijenbergh/puregotk/v4/gdk"
	"github.com/jwijenbergh/puregotk/v4/gtk"
)

// tabBarCSS contains GTK4 CSS styling for the tab bar and tab buttons.
// These styles match the old gotk4 implementation.
const tabBarCSS = `
/* Tab bar styling */
.tab-bar {
	background-color: #2b2b2b;
	border-top: 2px solid #353535;
	padding: 0;
	min-height: 32px;
}

/* Tab button styling - matches stacked pane titles */
button.tab-button {
	background-color: #404040;
	background-image: none;
	border: none;
	border-right: 1px solid #353535;
	border-radius: 0;
	padding: 4px 8px;
	transition: background-color 200ms ease-in-out;
}

button.tab-button:hover {
	background-color: #505050;
}

button.tab-button.tab-button-active {
	background-color: #707070;
	font-weight: 600;
}

/* Tab title text */
.tab-title {
	font-size: 12px;
	color: #ffffff;
	font-weight: 500;
}

/* ===== Omnibox Styling ===== */
/* Matches Svelte5 omnibox with GTK4-native CSS */

/* Omnibox window - floating popup with subtle depth */
window.omnibox-window {
	background-color: #2d2d2d;
	border: 1px solid #454545;
	border-radius: 3px;
}

/* Main container */
.omnibox-container {
	padding: 0;
	background-color: transparent;
}

/* Header with History/Favorites toggle - slightly elevated */
.omnibox-header {
	background-color: shade(#2d2d2d, 1.1);
	border-bottom: 1px solid #404040;
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
	color: #909090;
	transition: all 100ms ease-in-out;
}

.omnibox-header-btn:hover {
	background-color: alpha(#4a90e2, 0.15);
	color: #cccccc;
}

.omnibox-header-btn.omnibox-header-active {
	background-color: alpha(#4a90e2, 0.2);
	color: #4a90e2;
	font-weight: 600;
}

/* Search entry field - recessed appearance */
.omnibox-entry {
	background-color: #1a1a1a;
	color: #e8e8e8;
	border: 1px solid #404040;
	border-radius: 2px;
	padding: 10px 12px;
	margin: 8px 12px;
	font-size: 16px;
	caret-color: #4a90e2;
}

.omnibox-entry:focus {
	border-color: #4a90e2;
	background-color: #1e1e1e;
}

/* Scrolled window for suggestions */
.omnibox-scrolled {
	background-color: #2a2a2a;
	border-top: 1px solid #404040;
}

/* List box */
.omnibox-listbox {
	background-color: transparent;
}

/* Suggestion rows - with left accent border */
.omnibox-row {
	padding: 8px 12px;
	margin: 0;
	border-radius: 0;
	border-left: 3px solid transparent;
	border-bottom: 1px solid alpha(#505050, 0.5);
	transition: background-color 100ms ease-in-out, border-left-color 100ms ease-in-out;
}

.omnibox-row:last-child {
	border-bottom: none;
}

.omnibox-row:hover {
	background-color: alpha(#4a90e2, 0.12);
	border-left-color: #4a90e2;
}

.omnibox-row:selected {
	background-color: alpha(#4a90e2, 0.2);
	border-left-color: #4a90e2;
}

/* Suggestion title/URL */
.omnibox-suggestion-title {
	font-size: 14px;
	color: #e0e0e0;
	font-weight: 400;
}

.omnibox-row:selected .omnibox-suggestion-title {
	color: #ffffff;
}

/* Keyboard shortcut label (Ctrl+1-9) */
.omnibox-shortcut-label {
	font-size: 11px;
	color: #707070;
	font-family: monospace;
}

.omnibox-row:hover .omnibox-shortcut-label,
.omnibox-row:selected .omnibox-shortcut-label {
	color: alpha(#ffffff, 0.6);
}

/* Search shortcut badge */
.omnibox-shortcut-badge {
	background-color: #4a90e2;
	color: #ffffff;
	border-radius: 0;
	border: 1px solid #3a80d2;
	padding: 2px 8px;
	font-size: 11px;
	font-weight: 600;
	margin-right: 8px;
}
`

// ApplyCSS loads tab bar styling into the display.
func ApplyCSS(display *gdk.Display) {
	if display == nil {
		return
	}

	provider := gtk.NewCssProvider()
	if provider == nil {
		return
	}

	provider.LoadFromString(tabBarCSS)
	gtk.StyleContextAddProviderForDisplay(display, provider, uint(gtk.STYLE_PROVIDER_PRIORITY_APPLICATION))
}
