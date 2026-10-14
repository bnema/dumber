package theme

import (
	"fmt"
	"strings"
)

// modeLegendColors maps legend mode classes to the palette variables of their mode.
var modeLegendColors = []struct{ mode, color, text string }{
	{"pane", "--pane-mode-color", "--pane-mode-text"},
	{"vim", "--vim-mode-color", "--vim-mode-text"},
	{"resize", "--resize-mode-color", "--resize-mode-text"},
	{"tab", "--tab-mode-color", "--tab-mode-text"},
	{"session", "--session-mode-color", "--session-mode-text"},
	{"prefix", "--pane-mode-color", "--pane-mode-text"},
}

func generateModeLegendCSS() string {
	var sb strings.Builder
	sb.WriteString(`/* Mode toaster: a bottom-left pill whose legend expands above it. The legend
   is an omnibox surface tinted with the active mode color. */
.mode-legend-anchor { background-color: transparent; }
.mode-toaster-pill {
	border-radius: 0.25em;
	padding: 0.375em 0.625em;
	font-size: 0.875em;
	font-weight: 600;
	box-shadow: 0 2px 8px alpha(black, 0.3);
	min-width: 2.5em;
}
.mode-toaster-chevron { margin-left: 0.5em; font-size: 0.75em; opacity: 0.85; }
.mode-toaster-pill.mode-toaster-interactive:hover { box-shadow: 0 2px 10px alpha(black, 0.45); }
.mode-toaster-pill.mode-toaster-interactive:hover .mode-toaster-chevron { opacity: 1; }
.omnibox-container.mode-legend-panel {
	border-style: solid;
	border-width: 0.125em;
	padding: 0.625em 0.5em 0.375em 0.5em;
	min-width: 0;
	animation: mode-legend-appear 120ms ease-out;
}
.omnibox-container.mode-legend-panel.mode-legend-lingering { opacity: 0.85; }
.omnibox-container.mode-legend-panel.mode-legend-no-motion { animation: none; }
.mode-legend-hint {
	font-family: var(--font-mono);
	font-size: 0.6875em;
	color: var(--surface-muted);
	margin: 0.25em 0.5em 0 0;
}
.mode-legend-columns { padding: 0 0.25em; }
.mode-legend-column { min-width: 0; }
.mode-legend-group { margin: 0 0.375em 0.5em 0.375em; min-width: 0; }
.mode-legend-group-title {
	font-family: var(--font-mono);
	font-size: 0.6875em;
	font-weight: bold;
	margin-bottom: 0.25em;
}
.mode-legend-row { padding: 0.125em 0; min-width: 0; }
.mode-legend-keycap {
	font-family: var(--font-mono);
	font-size: 0.75em;
	font-weight: bold;
	color: var(--control-text);
	background-color: var(--surface-variant);
	border-style: solid;
	border-width: 0.0625em;
	border-radius: 0.25em;
	padding: 0.125em 0.375em;
	margin-right: 0.375em;
	transition: background-color 120ms ease-out, color 120ms ease-out;
}
.mode-legend-description { font-size: 0.75em; color: var(--surface-muted); }
.mode-legend-row.mode-legend-dim { opacity: 0.3; }
@keyframes mode-legend-appear {
	from { opacity: 0; }
	to { opacity: 1; }
}
`)
	for _, m := range modeLegendColors {
		fmt.Fprintf(&sb, `.omnibox-container.mode-legend-panel.mode-legend-%[1]s { border-color: var(%[2]s); }
.mode-toaster-pill.mode-toaster-%[1]s { background-color: var(%[2]s); color: var(%[3]s); }
.mode-legend-%[1]s .mode-legend-group-title { color: var(%[2]s); }
.mode-legend-%[1]s .mode-legend-keycap { border-color: alpha(var(%[2]s), 0.6); }
.mode-legend-%[1]s .mode-legend-keycap.mode-legend-flash { background-color: var(%[2]s); color: var(%[3]s); }
`, m.mode, m.color, m.text)
	}
	return sb.String()
}
