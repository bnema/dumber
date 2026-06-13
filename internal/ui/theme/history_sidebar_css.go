package theme

// generateHistorySidebarCSS creates GTK4 CSS for the history sidebar component.
func generateHistorySidebarCSS(_ Palette) string {
	return `/* ===== History Sidebar Styling ===== */

.history-sidebar-outer {
	background-color: var(--surface);
	border-left: 0.0625em solid var(--border);
}

.history-sidebar-search-box {
	padding: 0.375em 0.5em;
	border-bottom: 0.0625em solid var(--border);
	background-color: var(--surface);
}

.history-sidebar-search {
	padding: 0.125em 0.375em;
	font-size: 0.85em;
}

.history-sidebar-groups {
	background-color: var(--surface);
}

.history-sidebar-group-header {
	padding: 0.25em 0.625em;
	padding-top: 0.375em;
	font-size: 0.75em;
	font-weight: 600;
	color: var(--muted);
	text-transform: uppercase;
	letter-spacing: 0.04em;
	background-color: var(--surface-variant);
	border-bottom: 0.0625em solid var(--border);
}

.history-sidebar-row {
	padding: 0.1875em 0.625em;
	min-height: 0;
	border-bottom: 0.0625em solid alpha(var(--border), 0.4);
	background-color: var(--surface);
	transition: background-color 100ms ease;
}

.history-sidebar-row:hover {
	background-color: alpha(var(--accent), 0.18);
}

.history-sidebar-row:selected {
	background-color: alpha(var(--accent), 0.18);
}

.history-sidebar-row:focus {
	background-color: alpha(var(--accent), 0.18);
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
	padding-left: 0.5em;
	opacity: 0.75;
}

.history-sidebar-empty {
	padding: 1.5em 0.75em;
	font-size: 0.82em;
	color: var(--muted);
	font-style: italic;
}

.history-sidebar-loading {
	padding: 1.5em 0.75em;
	font-size: 0.82em;
	color: var(--muted);
}
`
}
