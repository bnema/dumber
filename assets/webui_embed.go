package assets

import _ "embed"

// WebUI scripts embedded at compile time

//go:embed webui/main-world.min.js
var MainWorldScript string

//go:embed webui/gui.min.js
var GUIScript string

//go:embed webui/color-scheme.js
var ColorSchemeScript string

// GUI component styles extracted from Svelte components
// These are injected into the isolated world using UserStyleSheet
//
//go:embed webui/gui.min.css
var ComponentStyles string
