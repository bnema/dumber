package theme

// generateSidebarCSS creates the shared GTK4 foundation used by native sidebars.
// The history sidebar remains the visual reference; feature-specific rules only
// add the controls unique to each sidebar.
func generateSidebarCSS(_ Palette) string {
	return `/* ===== Native Sidebar Foundation ===== */

.sidebar-outer {
	background-color: var(--surface);
	border-left: 0.0625em solid var(--border);
}

.sidebar-search-box {
	padding: 0.5em 0.5em 0.375em;
	border-bottom: 0.0625em solid var(--border);
	background-color: var(--surface);
}

.sidebar-search {
	min-height: 0;
	padding: 0.125em 0.375em;
	font-size: 0.85em;
}

.sidebar-list {
	background-color: var(--surface);
}

.sidebar-row {
	padding: 0.1875em 0.625em;
	min-height: 0;
	border-bottom: 0.0625em solid alpha(var(--border), 0.4);
	background-color: var(--surface);
	transition: background-color 100ms ease;
}

.sidebar-row:hover {
	background-color: alpha(var(--accent), 0.18);
}

.sidebar-row:selected {
	background-color: alpha(var(--accent), 0.18);
}

.sidebar-row:focus {
	background-color: alpha(var(--accent), 0.18);
}

.sidebar-row-title {
	font-size: 0.82em;
	color: var(--text);
	font-weight: 500;
}

.sidebar-row-subtitle {
	font-size: 0.72em;
	color: var(--muted);
}

.sidebar-empty {
	padding: 1.5em 0.75em;
	font-size: 0.82em;
	color: var(--muted);
	font-style: italic;
}
`
}
