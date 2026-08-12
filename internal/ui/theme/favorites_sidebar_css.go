package theme

// generateFavoritesSidebarCSS creates GTK4 CSS for favorites-specific controls.
func generateFavoritesSidebarCSS(_ Palette) string {
	return `/* ===== Favorites Sidebar Styling ===== */

.favorites-sidebar-tags {
	padding: 0.25em 0.5em 0.375em;
	border-bottom: 0.0625em solid var(--border);
	background-color: var(--surface-variant);
}

.favorites-sidebar-tag-filter {
	min-height: 0;
	min-width: 0;
	padding: 0.125em 0.375em;
	font-size: 0.72em;
	font-weight: 500;
	color: var(--muted);
	background-color: transparent;
	border: 0.0625em solid alpha(var(--border), 0.7);
	border-radius: 0.25em;
	box-shadow: none;
}

.favorites-sidebar-tag-filter:hover {
	color: var(--text);
	background-color: alpha(var(--accent), 0.10);
}

.favorites-sidebar-tag-filter.suggested-action {
	color: var(--accent);
	background-color: alpha(var(--accent), 0.16);
	border-color: alpha(var(--accent), 0.5);
}

.favorites-sidebar-tag-add {
	min-width: 1.4em;
	font-weight: 700;
}

.favorites-sidebar-tag-prompt {
	padding: 0.375em 0.5em;
	border-bottom: 0.0625em solid var(--border);
	background-color: var(--surface-variant);
}

.favorites-sidebar-tag-prompt entry,
.favorites-sidebar-tag-prompt button {
	min-height: 0;
	padding: 0.1875em 0.375em;
	font-size: 0.78em;
}

.favorites-sidebar-tag-binding {
	min-height: 0;
	padding: 0.125em 0.375em;
	font-size: 0.72em;
}

.favorites-sidebar-shortcut-badge,
.favorites-sidebar-tag-chip {
	padding: 0.0625em 0.25em;
	font-size: 0.66em;
	font-weight: 600;
	color: var(--muted);
	background-color: alpha(var(--surface-variant), 0.9);
	border: 0.0625em solid alpha(var(--border), 0.65);
	border-radius: 0.2em;
}

.favorites-sidebar-row-tags {
	margin-top: 0.0625em;
}

.favorites-sidebar-shortcut-badge {
	color: var(--accent);
	border-color: alpha(var(--accent), 0.45);
	background-color: alpha(var(--accent), 0.10);
}

.favorites-sidebar-form {
	padding: 0.5em;
	border-top: 0.0625em solid var(--border);
	background-color: var(--surface-variant);
}

.favorites-sidebar-form entry {
	min-height: 0;
	padding: 0.1875em 0.375em;
	font-size: 0.8em;
	background-color: var(--surface);
}

.favorites-sidebar-form-tag-matches {
	padding: 0.125em 0;
}

.favorites-sidebar-form-tag-match {
	min-height: 0;
	min-width: 0;
	padding: 0.0625em 0.25em;
	font-size: 0.7em;
	color: var(--muted);
	background-color: alpha(var(--surface), 0.8);
	border: 0.0625em solid alpha(var(--border), 0.65);
	border-radius: 0.2em;
	box-shadow: none;
}

.favorites-sidebar-form-tag-match:hover {
	color: var(--text);
	background-color: alpha(var(--accent), 0.12);
}

.favorites-sidebar-form button {
	min-height: 0;
	padding: 0.1875em 0.5em;
	font-size: 0.78em;
}
`
}
