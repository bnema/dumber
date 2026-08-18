package theme

// generateHistorySidebarCSS creates GTK4 CSS for the history sidebar component.
func generateHistorySidebarCSS(_ Palette) string {
	return `/* ===== History Sidebar Styling ===== */

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

.history-sidebar-row-time {
	font-size: 0.68em;
	color: var(--muted);
	padding-left: 0.5em;
	opacity: 0.75;
}

.history-sidebar-loading {
	padding: 1.5em 0.75em;
	font-size: 0.82em;
	color: var(--muted);
}
`
}
