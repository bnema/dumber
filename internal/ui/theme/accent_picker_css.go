package theme

// generateAccentPickerCSS creates accent picker overlay styles.
// The picker displays a row of accented characters that the user can select.
func generateAccentPickerCSS(p Palette) string {
	_ = p
	return `/* ===== Accent Picker Styling ===== */

/* Main container - horizontal row of accent characters */
.accent-picker {
	background-color: alpha(var(--surface-variant), 0.95);
	border: 0.0625em solid var(--border);
	border-radius: 0.25em;
	padding: 0.375em 0.5em;
	margin-bottom: 1em;
	box-shadow: 0 2px 8px alpha(black, 0.2);
}

/* Individual accent character label */
.accent-picker-item {
	font-size: 1.5em;
	font-weight: 500;
	color: var(--text);
	padding: 0.25em 0.5em;
	border-radius: 0.125em;
	min-width: 1.5em;
	transition: background-color 100ms ease-in-out;
}

.accent-picker-item:hover {
	background-color: alpha(var(--accent), 0.15);
}

/* Selected accent character */
.accent-picker-selected {
	background-color: alpha(var(--accent), 0.25);
	color: var(--accent);
}

/* Numbered items (1-9) could show subtle number hints */
.accent-picker-numbered {
	/* Could add ::after pseudo-element for number hint if needed */
}
`
}
