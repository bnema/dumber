package assets

import "embed"

// WebUIAssets contains the embedded webui build output (homepage and blocked pages).
// Used by the scheme handler to serve dumb://home and dumb://blocked.
//
//go:embed webui/*
var WebUIAssets embed.FS
