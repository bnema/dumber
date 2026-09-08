package assets

import (
	"embed"
)

// WebUIAssets contains the embedded systemviews build output.
// The legacy WebUIAssets name is kept for compatibility with existing callers.
// Used by the scheme handlers to serve dumb://error, dumb://config,
// dumb://history, and dumb://favorites.
//
//go:embed systemviews/*
var WebUIAssets embed.FS

// LogoSVG contains the dumber logo as SVG for desktop icon installation.
//
//go:embed logo.svg
var LogoSVG []byte

// LogoPNG32 contains the dumber logo as 32x32 PNG for CLI tools like dmenu/rofi.
//
//go:embed logo-32.png
var LogoPNG32 []byte

// LogoPNG512 contains the dumber logo as 512x512 PNG for the loading
// placeholder. The initial pane decodes this raster asset instead of SVG to
// avoid XML parsing and rasterization on the critical path. LogoSVG remains
// the source for desktop icon installation.
//
//go:embed logo-512.png
var LogoPNG512 []byte
