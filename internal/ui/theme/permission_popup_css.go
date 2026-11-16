package theme

// generatePermissionPopupCSS creates permission popup overlay styles.
// Uses em units for scalable UI, matches omnibox/session manager styling patterns.
func generatePermissionPopupCSS(p Palette) string {
	_ = p
	return generateModalChromeCSS("permission-popup", "permission-popup-heading") + `/* ===== Permission Popup Styling ===== */

.permission-popup-container {
	min-width: 20em;
}

.permission-popup-heading {
	font-size: 0.9375em;
	font-weight: 600;
	color: var(--text);
}

.permission-popup-body {
	font-size: 0.8125em;
	font-weight: 400;
	color: var(--muted);
	padding: 0.25em 1em 0.75em 1em;
}

.permission-popup-btn-row {
	border-top: 0.0625em solid alpha(var(--border), 0.65);
	font-family: var(--font-mono);
	padding: 0.5em 0.75em;
}

/* JS dialog (alert/confirm/prompt/beforeunload): per-pane scrim + prompt entry */
.jsdialog-scrim {
	background-color: alpha(black, 0.35);
}

.jsdialog-entry {
	margin: 0 1em 0.75em 1em;
	font-size: 0.8125em;
}

/* Shared button base */
.permission-popup-btn {
	background-image: none;
	border: 0.0625em solid var(--border);
	border-radius: 0;
	padding: 0.375em 0.75em;
	font-size: 0.8125em;
	font-weight: 500;
	transition: background-color 120ms ease-in-out, border-color 120ms ease-in-out;
}

.omnibox-style-command .permission-popup-btn {
	border-radius: 0.375em;
	padding: 0.625em 1em;
	border-color: transparent;
	box-shadow: none;
}

/* Allow buttons - accent styled */
.permission-popup-btn-allow {
	background-color: alpha(var(--accent), 0.15);
	color: var(--accent);
	border-color: alpha(var(--accent), 0.3);
}

.permission-popup-btn-allow:hover {
	background-color: alpha(var(--accent), 0.25);
	border-color: var(--accent);
}

/* Deny button - subtle default */
.permission-popup-btn-deny {
	background-color: var(--surface);
	color: var(--text);
}

.permission-popup-btn-deny:hover {
	background-color: shade(var(--surface), 1.15);
}

/* Always Deny - destructive */
.permission-popup-btn-destructive {
	background-color: alpha(var(--destructive), 0.15);
	color: var(--destructive);
	border-color: alpha(var(--destructive), 0.3);
}

.permission-popup-btn-destructive:hover {
	background-color: alpha(var(--destructive), 0.25);
	border-color: var(--destructive);
}

/* Command dialogs use quiet UI typography and spacious content. */
.omnibox-style-command.permission-popup-container {
	font-family: var(--font-gtk);
}

.omnibox-style-command .permission-popup-heading {
	padding: 1em 1.25em 0.5em;
}

.omnibox-style-command .permission-popup-body {
	padding: 0.5em 1.25em 1em;
}

.omnibox-style-command .permission-popup-btn-row {
	font-family: var(--font-gtk);
	padding: 0.75em 1.25em;
}

.omnibox-style-command .permission-popup-btn-allow {
	background-color: mix(var(--surface), var(--accent), 0.7);
	color: var(--accent-text);
}

.omnibox-style-command .permission-popup-btn-allow:hover {
	background-color: mix(var(--surface), var(--accent), 0.85);
	border-color: transparent;
}

.omnibox-style-command .permission-popup-btn-deny,
.omnibox-style-command .permission-popup-btn-destructive {
	background-color: alpha(var(--text), 0.04);
	border-color: transparent;
}

.omnibox-style-command .permission-popup-btn-deny:hover,
.omnibox-style-command .permission-popup-btn-destructive:hover {
	background-color: alpha(var(--text), 0.08);
	border-color: transparent;
}

.omnibox-style-command .permission-popup-btn-destructive {
	background-color: transparent;
}

.omnibox-style-command .jsdialog-entry {
	margin: 0 1.25em 1em;
}
`
}
