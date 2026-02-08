package assets

import (
	"embed"
)

// WebUIAssets contains the embedded webui build output (homepage, error, config, and webrtc pages).
// Used by the scheme handler to serve dumb://home, dumb://error, dumb://config, and dumb://webrtc.
//
//go:embed webui/*
var WebUIAssets embed.FS

// LogoSVG contains the dumber logo as SVG for desktop icon installation.
//
//go:embed logo.svg
var LogoSVG []byte

// LogoPNG32 contains the dumber logo as 32x32 PNG for CLI tools like dmenu/rofi.
//
//go:embed logo-32.png
var LogoPNG32 []byte
