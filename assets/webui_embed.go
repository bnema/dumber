package assets

import (
	"embed"
)

// WebUIAssets contains the embedded systemviews build output.
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
