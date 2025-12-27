package theme

// generateTabPickerCSS creates tab picker modal styles.
// Uses similar styling conventions as Omnibox/SessionManager.
func generateTabPickerCSS(p Palette) string {
	_ = p
	return `/* ===== Tab Picker Styling ===== */

.tab-picker-outer {
	/* Positioning handled in Go */
}

.tab-picker-container {
	background-color: var(--surface-variant);
	border: 0.0625em solid var(--border);
	border-radius: 0.1875em;
	padding: 0;
	min-width: 28em;
}

.tab-picker-header {
	background-color: shade(var(--surface-variant), 1.1);
	border-bottom: 0.0625em solid var(--border);
	padding: 0.5em 0.75em;
}

.tab-picker-title {
	font-size: 0.9375em;
	font-weight: 600;
	color: var(--text);
}

.tab-picker-scrolled {
	background-color: shade(var(--surface-variant), 0.95);
}

.tab-picker-list {
	background-color: transparent;
}

.tab-picker-row {
	padding: 0.5em 0.75em;
	margin: 0;
	border-radius: 0;
	border-left: 0.1875em solid transparent;
	border-bottom: 0.0625em solid alpha(var(--border), 0.5);
	transition: background-color 100ms ease-in-out, border-left 100ms ease-in-out;
	min-height: 2.75em;
}

.tab-picker-row:last-child {
	border-bottom: none;
}

.tab-picker-row:hover {
	background-color: alpha(var(--accent), 0.12);
	border-left: 0.1875em solid var(--accent);
}

.tab-picker-row:selected,
row:selected .tab-picker-row {
	background-color: alpha(var(--accent), 0.2);
	border-left: 0.1875em solid var(--accent);
}

.tab-picker-row-title {
	color: var(--text);
	font-size: 0.875em;
	font-weight: 400;
}

.tab-picker-footer {
	background-color: shade(var(--surface-variant), 0.9);
	border-top: 0.0625em solid var(--border);
	padding: 0.375em 0.75em;
	font-size: 0.6875em;
	color: var(--muted);
	font-family: var(--font-mono);
}
`
}
