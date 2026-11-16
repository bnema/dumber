package theme

import "strings"

// generateModalChromeCSS shares only the popup shell and header vocabulary;
// content and controls remain owned by each modal's stylesheet.
func generateModalChromeCSS(prefix, header string) string {
	return strings.NewReplacer("$modal", prefix, "$header", header).Replace(`
.$modal-container {
	background-color: var(--surface);
	background-image: none;
	border: 0.09375em solid alpha(var(--border), 0.9);
	border-radius: 0.1875em;
	box-shadow: 0 0.875em 2.5em alpha(black, 0.24), 0 0.0625em 0 alpha(white, 0.05) inset;
	padding: 0;
}

.$header {
	background-color: var(--surface);
	background-image: none;
	border-bottom: 0.0625em solid alpha(var(--border), 0.75);
	padding: 0.375em 0.75em;
}

.omnibox-style-multiplexer.$modal-container {
	border-radius: 0;
	border-color: var(--border);
	box-shadow: 0 1.125em 3em alpha(black, 0.34);
}

.omnibox-style-multiplexer .$header {
	min-height: 2em;
	background-color: shade(var(--surface), 0.94);
	font-family: var(--font-mono);
}

.omnibox-style-command.$modal-container {
	border: 0.125em solid alpha(var(--border), 0.95);
	border-radius: 0.625em;
	box-shadow: 0 1.25em 3.5em alpha(black, 0.38), 0 0 0 0.0625em alpha(white, 0.04) inset;
	padding: 0.375em;
}

.omnibox-style-command .$header {
	border-bottom: none;
	padding: 0.25em 0.375em 0.125em;
}

.omnibox-style-minimal.$modal-container {
	border-radius: 0;
	box-shadow: 0 0.75em 2em alpha(black, 0.2);
}

.omnibox-style-minimal .$header {
	padding: 0.25em 0.75em;
	font-family: var(--font-mono);
}
`)
}
