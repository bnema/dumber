package theme

import (
	"fmt"
)

// generateHistorySidebarCSS creates GTK4 CSS for the history sidebar component.
func generateHistorySidebarCSS(p Palette) string {
	accentAlpha := fmt.Sprintf("alpha(%s, 0.18)", p.Accent)

	return fmt.Sprintf(`/* ===== History Sidebar Styling ===== */

.history-sidebar-outer {
	background-color: var(--surface);
	border-left: 1px solid var(--border);
}

.history-sidebar-search-box {
	padding: 6px 8px;
	border-bottom: 1px solid var(--border);
	background-color: var(--surface);
}

.history-sidebar-search {
	padding: 2px 6px;
	font-size: 0.85em;
}

.history-sidebar-groups {
	background-color: var(--surface);
}

.history-sidebar-group-header {
	padding: 4px 10px;
	padding-top: 6px;
	font-size: 0.75em;
	font-weight: 600;
	color: var(--muted);
	text-transform: uppercase;
	letter-spacing: 0.04em;
	background-color: var(--surface-variant);
	border-bottom: 1px solid var(--border);
}

.history-sidebar-row {
	padding: 3px 10px;
	min-height: 0;
	border-bottom: 1px solid alpha(var(--border), 0.4);
	background-color: var(--surface);
	transition: background-color 100ms ease;
}

.history-sidebar-row:hover {
	background-color: %s;
}

.history-sidebar-row:selected {
	background-color: %s;
}

.history-sidebar-row:focus {
	background-color: %s;
}

.history-sidebar-row-title {
	font-size: 0.82em;
	color: var(--text);
	font-weight: 500;
}

.history-sidebar-row-subtitle {
	font-size: 0.72em;
	color: var(--muted);
}

.history-sidebar-row-time {
	font-size: 0.68em;
	color: var(--muted);
	padding-left: 8px;
	opacity: 0.75;
}

.history-sidebar-empty {
	padding: 24px 12px;
	font-size: 0.82em;
	color: var(--muted);
	font-style: italic;
}

.history-sidebar-loading {
	padding: 24px 12px;
	font-size: 0.82em;
	color: var(--muted);
}
`, accentAlpha, accentAlpha, accentAlpha)
}
