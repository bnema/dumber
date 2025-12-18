package assets

import (
	"embed"
	_ "embed"
)

// WebUIAssets contains the embedded webui build output (homepage and blocked pages).
// Used by the scheme handler to serve dumb://home and dumb://blocked.
//
//go:embed webui/*
var WebUIAssets embed.FS

// LogoSVG contains the dumber logo as SVG for desktop icon installation.
//
//go:embed logo.svg
var LogoSVG []byte
