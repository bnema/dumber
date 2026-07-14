// Package cef2gtk provides a GTK4 widget bridge for purego-cef accelerated
// off-screen rendering.
//
// The project is Wayland-only and GPU-first. It is intended to use CEF
// accelerated OSR shared textures/DMABUFs; software OnPaint rendering is a
// diagnostic/error path, not a product renderer.
package cef2gtk

import "github.com/bnema/purego-cef/cef"

// cefSettingsAnchor keeps the Phase 0 module tied to the purego-cef API that
// later phases build on, without exposing premature public surface area.
var _ = cef.Settings{}
