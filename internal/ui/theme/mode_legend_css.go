package theme

import (
	"fmt"
	"strings"
)

// modeLegendColors maps legend mode classes to the palette variable of their mode.
var modeLegendColors = []struct{ mode, color string }{
	{"pane", "--pane-mode-color"},
	{"vim", "--pane-mode-color"},
	{"resize", "--resize-mode-color"},
	{"tab", "--tab-mode-color"},
	{"session", "--session-mode-color"},
}

func generateModeLegendCSS() string {
	var sb strings.Builder
	sb.WriteString(`/* Mode frame legend: omnibox surface, tinted with the active mode color. */
.mode-legend-anchor { background-color: transparent; }
.omnibox-container.mode-legend-panel {
	border-style: solid;
	border-width: 0.125em;
	padding: 0 0 0.375em 0;
	min-width: 0;
	animation: mode-legend-appear 120ms ease-out;
}
.omnibox-container.mode-legend-panel.mode-legend-no-motion { animation: none; }
.mode-legend-title {
	font-family: var(--font-mono);
	font-size: 0.8125em;
	font-weight: bold;
	color: #ffffff;
	padding: 0.375em 0.75em;
	margin-bottom: 0.375em;
}
.mode-legend-content { padding: 0 0.25em; }
.mode-legend-group { margin: 0 0.375em; min-width: 0; }
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
	color: var(--text);
	background-color: var(--surface-variant);
	border-style: solid;
	border-width: 0.0625em;
	border-radius: 0.25em;
	padding: 0.125em 0.375em;
	margin-right: 0.375em;
	transition: background-color 120ms ease-out, color 120ms ease-out;
}
.mode-legend-description { font-size: 0.75em; color: var(--muted); }
.mode-legend-row.mode-legend-dim { opacity: 0.3; }
@keyframes mode-legend-appear {
	from { opacity: 0; }
	to { opacity: 1; }
}
`)
	for _, m := range modeLegendColors {
		fmt.Fprintf(&sb, `.omnibox-container.mode-legend-panel.mode-legend-%[1]s { border-color: var(%[2]s); }
.mode-legend-%[1]s .mode-legend-title { background-color: var(%[2]s); }
.mode-legend-%[1]s .mode-legend-group-title { color: var(%[2]s); }
.mode-legend-%[1]s .mode-legend-keycap { border-color: alpha(var(%[2]s), 0.6); }
.mode-legend-%[1]s .mode-legend-keycap.mode-legend-flash { background-color: var(%[2]s); color: #ffffff; }
`, m.mode, m.color)
	}
	return sb.String()
}
