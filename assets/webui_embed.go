package assets

import _ "embed"

// ColorSchemeScript handles dark mode detection in web pages.
// It patches window.matchMedia for prefers-color-scheme queries
// based on the __dumber_gtk_prefers_dark flag injected by Go.
//
//go:embed webui/color-scheme.js
var ColorSchemeScript string

// Deprecated: These variables are kept for backwards compatibility during migration.
// They will be removed once all JS handlers are deprecated.
// The actual files may not exist - check before using.
var MainWorldScript string
var GUIScript string
var ComponentStyles string
