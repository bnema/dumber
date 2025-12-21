package assets

import (
	"embed"
)

// WebUIAssets contains the embedded webui build output (homepage, error, and config pages).
// Used by the scheme handler to serve dumb://home, dumb://error, and dumb://config.
//
//go:embed webui/*
var WebUIAssets embed.FS

// LogoSVG contains the dumber logo as SVG for desktop icon installation.
//
//go:embed logo.svg
var LogoSVG []byte
