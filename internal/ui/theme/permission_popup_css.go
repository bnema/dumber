package theme

// generatePermissionPopupCSS creates permission popup overlay styles.
// Uses em units for scalable UI, matches omnibox/session manager styling patterns.
func generatePermissionPopupCSS(p Palette) string {
	_ = p
	return `/* ===== Permission Popup Styling ===== */

.permission-popup-outer {
	/* Positioning handled via SetHalign/SetValign in Go */
}

.permission-popup-container {
	background-color: var(--surface-variant);
	border: 0.0625em solid var(--border);
	border-radius: 0.1875em;
	padding: 0;
	min-width: 20em;
}

.permission-popup-heading {
	font-size: 0.9375em;
	font-weight: 600;
	color: var(--text);
	padding: 0.75em 1em 0.25em 1em;
}

.permission-popup-body {
	font-size: 0.8125em;
	font-weight: 400;
	color: var(--muted);
	padding: 0.25em 1em 0.75em 1em;
}

.permission-popup-btn-row {
	border-top: 0.0625em solid var(--border);
	padding: 0.5em 0.75em;
}

/* Shared button base */
.permission-popup-btn {
	background-image: none;
	border: 0.0625em solid var(--border);
	border-radius: 0.1875em;
	padding: 0.375em 0.75em;
	font-size: 0.8125em;
	font-weight: 500;
	transition: background-color 120ms ease-in-out, border-color 120ms ease-in-out;
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
`
}
