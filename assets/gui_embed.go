package assets

import _ "embed"

// GUI scripts embedded at compile time

//go:embed gui/main-world.min.js
var MainWorldScript string

//go:embed gui/gui.min.js
var GUIScript string

//go:embed gui/color-scheme.js
var ColorSchemeScript string
