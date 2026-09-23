package theme

func generateModeLegendCSS() string {
	return `/* Mode frame legend: shares omnibox palette and treatments. */
.mode-legend-anchor { background-color: transparent; }
.mode-legend-panel {
	--mode-legend-color: var(--pane-mode-color);
	border-color: var(--mode-legend-color);
	border-width: 0.25em;
	border-radius: 0.625em 0.625em 0 0;
	padding: 0.5em;
	min-width: 0;
	animation: mode-legend-appear 120ms ease-out;
}
.mode-legend-panel.mode-legend-no-motion { animation: none; }
.mode-legend-panel.mode-legend-tab { --mode-legend-color: var(--tab-mode-color); }
.mode-legend-panel.mode-legend-session { --mode-legend-color: var(--session-mode-color); }
.mode-legend-panel.mode-legend-resize { --mode-legend-color: var(--resize-mode-color); }
.mode-legend-panel.mode-legend-vim { border-width: 0.125em; --mode-legend-color: var(--pane-mode-color); }
.mode-legend-panel.omnibox-style-minimal { border-radius: 0; }
.mode-legend-panel.omnibox-style-multiplexer { border-radius: 0.25em 0.25em 0 0; }
.mode-legend-title {
	font-family: var(--font-mono);
	font-weight: bold;
	color: var(--mode-legend-color);
	margin: 0.25em 0.5em 0.5em;
}
.mode-legend-content { padding: 0.25em; }
.mode-legend-group { margin: 0 0.375em; min-width: 0; }
.mode-legend-group-title {
	font-family: var(--font-mono);
	font-size: 0.75em;
	font-weight: bold;
	color: var(--mode-legend-color);
	margin-bottom: 0.25em;
}
.mode-legend-row { padding: 0.125em 0; min-width: 0; }
.mode-legend-keycap {
	font-family: var(--font-mono);
	font-size: 0.75em;
	font-weight: bold;
	color: var(--text);
	background-color: var(--surface-variant);
	border: 0.0625em solid alpha(var(--mode-legend-color), 0.45);
	border-radius: 0.25em;
	padding: 0.125em 0.25em;
	margin-right: 0.25em;
}
.mode-legend-keycap.mode-legend-flash { background-color: var(--mode-legend-color); color: var(--bg); }
.mode-legend-description { font-size: 0.75em; color: var(--muted); }
.mode-legend-row.mode-legend-dim { opacity: 0.3; }
@keyframes mode-legend-appear {
	from { opacity: 0; transform: translateY(0.5em); }
	to { opacity: 1; transform: translateY(0); }
}
`
}
