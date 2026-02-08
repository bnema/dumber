package theme

// generateWebRTCPermissionIndicatorCSS creates styles for the top-right
// WebRTC permission activity indicator.
// The dot anchors top-right; on hover the card expands left and down
// using a GtkRevealer (slide-down) for the details panel.
func generateWebRTCPermissionIndicatorCSS(p Palette) string {
	_ = p
	return `/* ===== WebRTC Permission Indicator ===== */

.webrtc-indicator-outer {
	margin-top: 0.125em;
	margin-right: 0.125em;
}

/* Card container: transparent when collapsed, styled when expanded.
   Right-aligned so the dot stays top-right and content grows leftward. */
.webrtc-indicator-card {
	background-color: transparent;
	border: 0.0625em solid transparent;
	border-radius: 999px;
	padding: 0;
	transition: background-color 180ms ease-in-out,
	            border-color 180ms ease-in-out,
	            box-shadow 180ms ease-in-out;
}

.webrtc-indicator-card.expanded {
	background-color: alpha(var(--surface-variant), 0.96);
	border-color: alpha(var(--border), 0.8);
	border-radius: 0.375em;
	box-shadow: 0 0.25em 1em alpha(black, 0.28),
	            0 0.0625em 0.25em alpha(black, 0.12);
	padding: 0.4em 0.5em;
}

/* Summary dot - small colored circle, right-aligned */
.webrtc-indicator-dot {
	min-width: 0.5em;
	min-height: 0.5em;
	border-radius: 999px;
	background-color: var(--muted);
	border: 0.0625em solid alpha(var(--border), 0.6);
	transition: background-color 200ms ease-in-out,
	            border-color 200ms ease-in-out,
	            box-shadow 200ms ease-in-out;
}

/* Origin label shown next to the dot. */
.webrtc-indicator-origin {
	font-size: 0.6875em;
	font-weight: 600;
	letter-spacing: 0.02em;
	padding: 0 0.4em 0 0;
	color: var(--muted);
}

/* Details section inside the revealer (permission rows only) */
.webrtc-indicator-details {
	margin-top: 0.3em;
	padding-top: 0.3em;
	border-top: 0.0625em solid alpha(var(--border), 0.5);
}

/* Permission row buttons */
.webrtc-indicator-row {
	font-size: 0.6875em;
	font-weight: 500;
	padding: 0.25em 0.4em;
	margin-top: 0.15em;
	border-radius: 0.25em;
	border-left: 0.1875em solid transparent;
	border-top: none;
	border-right: none;
	border-bottom: none;
	background-color: alpha(var(--bg), 0.35);
	background-image: none;
	color: var(--muted);
	text-align: left;
	transition: background-color 150ms ease-in-out,
	            border-color 150ms ease-in-out,
	            color 150ms ease-in-out;
}

.webrtc-indicator-row:hover {
	background-color: alpha(var(--bg), 0.55);
	background-image: none;
}

/* ---- State: idle ---- */
.webrtc-indicator-dot.state-idle {
	background-color: var(--muted);
	border-color: alpha(var(--border), 0.6);
}

.webrtc-indicator-row.state-idle {
	color: var(--muted);
	border-left-color: alpha(var(--muted), 0.4);
}

/* ---- State: requesting (amber) ---- */
.webrtc-indicator-dot.state-requesting {
	background-color: var(--warning);
	border-color: alpha(var(--warning), 0.6);
	box-shadow: 0 0 0.4em alpha(var(--warning), 0.5);
}

.webrtc-indicator-row.state-requesting {
	color: var(--warning);
	border-left-color: var(--warning);
	background-color: alpha(var(--warning), 0.08);
}

.webrtc-indicator-row.state-requesting:hover {
	background-color: alpha(var(--warning), 0.14);
	background-image: none;
}

/* ---- State: allowed (green) ---- */
.webrtc-indicator-dot.state-allowed {
	background-color: var(--success);
	border-color: alpha(var(--success), 0.6);
	box-shadow: 0 0 0.35em alpha(var(--success), 0.35);
}

.webrtc-indicator-row.state-allowed {
	color: var(--success);
	border-left-color: var(--success);
	background-color: alpha(var(--success), 0.06);
}

.webrtc-indicator-row.state-allowed:hover {
	background-color: alpha(var(--success), 0.12);
	background-image: none;
}

/* ---- State: blocked (red) ---- */
.webrtc-indicator-dot.state-blocked {
	background-color: var(--destructive);
	border-color: alpha(var(--destructive), 0.6);
	box-shadow: 0 0 0.35em alpha(var(--destructive), 0.35);
}

.webrtc-indicator-row.state-blocked {
	color: var(--destructive);
	border-left-color: var(--destructive);
	background-color: alpha(var(--destructive), 0.06);
}

.webrtc-indicator-row.state-blocked:hover {
	background-color: alpha(var(--destructive), 0.12);
	background-image: none;
}

/* ---- Locked row modifier ---- */
.webrtc-indicator-row.row-locked {
	font-weight: 600;
}
`
}
