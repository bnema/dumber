// Package gtkgl contains GtkGLArea context probing helpers shared by probes and renderers.
package gtkgl

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/bnema/purego-cef2gtk/internal/egl"
	"github.com/bnema/puregotk/v4/gdk"
	"github.com/bnema/puregotk/v4/gtk"
)

const (
	BackendWayland = "wayland"
	BackendX11     = "x11"
	BackendUnknown = "unknown"

	ContextStrategyGtkGLAreaCurrent = "gtk-gl-area-current"
)

var (
	ErrNilGLArea               = errors.New("nil GtkGLArea")
	ErrNonWaylandBackend       = errors.New("non-Wayland GTK/GDK backend")
	ErrMissingEGLDisplay       = errors.New("missing EGLDisplay")
	ErrMissingDMABUFImport     = errors.New("missing EGL_EXT_image_dma_buf_import")
	ErrMissingGLAreaContext    = errors.New("missing GtkGLArea context")
	ErrGLAreaNotRealized       = errors.New("GtkGLArea is not realized")
	ErrMissingGLAreaCurrent    = errors.New("GtkGLArea context is not current")
	ErrMissingGLAreaCapability = errors.New("missing GLArea capability")
)

// ContextProbeResult describes the realized/current GtkGLArea GL context and EGL display.
type ContextProbeResult struct {
	Backend             string   `json:"backend"`
	GLAPI               string   `json:"gl_api"`
	GLVersion           string   `json:"gl_version"`
	GLVendor            string   `json:"gl_vendor"`
	GLRenderer          string   `json:"gl_renderer"`
	EGLDisplay          string   `json:"egl_display"`
	EGLVendor           string   `json:"egl_vendor,omitempty"`
	EGLVersion          string   `json:"egl_version,omitempty"`
	EGLClientAPIs       string   `json:"egl_client_apis,omitempty"`
	EGLExtensions       []string `json:"egl_extensions"`
	EGLExtensionString  string   `json:"egl_extension_string,omitempty"`
	ContextStrategy     string   `json:"context_strategy"`
	DMABUFImportSupport bool     `json:"dmabuf_import_support"`
}

// Validate enforces the Phase 1 compatibility gate: Wayland + current EGL + DMABUF import.
func (r ContextProbeResult) Validate() error {
	if strings.ToLower(r.Backend) != BackendWayland {
		return fmt.Errorf("%w: %q", ErrNonWaylandBackend, r.Backend)
	}
	if r.EGLDisplay == "" || r.EGLDisplay == egl.NoDisplay.String() {
		return ErrMissingEGLDisplay
	}
	if !r.DMABUFImportSupport && !slices.Contains(r.EGLExtensions, egl.ExtensionDMABUFImport) {
		return ErrMissingDMABUFImport
	}
	if strings.TrimSpace(r.GLVersion) == "" || strings.TrimSpace(r.GLVendor) == "" || strings.TrimSpace(r.GLRenderer) == "" {
		return ErrMissingGLAreaCapability
	}
	return nil
}

// ProbeCurrentGLAreaContext makes area current and returns GL/EGL diagnostics.
// GTK/GDK callers must invoke this on the GTK main thread.
func ProbeCurrentGLAreaContext(area *gtk.GLArea) (ContextProbeResult, error) {
	if area == nil {
		return ContextProbeResult{}, ErrNilGLArea
	}

	result := ContextProbeResult{Backend: DetectBackendFromDisplay(area.GetDisplay())}
	if !area.GetRealized() {
		return result, ErrGLAreaNotRealized
	}
	if strings.ToLower(result.Backend) != BackendWayland {
		return result, result.Validate()
	}

	area.MakeCurrent()
	if gerr := area.GetError(); gerr != nil {
		return result, fmt.Errorf("gtk gl area error: %s", glibErrorMessage(gerr))
	}
	ctx := area.GetContext()
	if ctx == nil {
		return result, ErrMissingGLAreaContext
	}

	eglInfo, err := egl.CurrentDisplayInfo()
	if err != nil {
		return result, err
	}
	glInfo, err := currentGLInfo()
	if err != nil {
		// Return partial EGL diagnostics for troubleshooting only; callers must
		// still treat the non-nil currentGLInfo error as a failed probe.
		result.GLAPI = glAPIName(area.GetApi(), ctx.GetApi())
		result.EGLDisplay = eglInfo.Display.String()
		result.EGLVendor = eglInfo.Vendor
		result.EGLVersion = eglInfo.Version
		result.EGLClientAPIs = eglInfo.ClientAPIs
		result.EGLExtensions = eglInfo.Extensions.Names()
		result.EGLExtensionString = eglInfo.ExtensionString
		result.ContextStrategy = ContextStrategyGtkGLAreaCurrent
		result.DMABUFImportSupport = eglInfo.SupportsDMABUFImport()
		return result, err
	}
	result = ContextProbeResult{
		Backend:             result.Backend,
		GLAPI:               glAPIName(area.GetApi(), ctx.GetApi()),
		GLVersion:           glInfo.Version,
		GLVendor:            glInfo.Vendor,
		GLRenderer:          glInfo.Renderer,
		EGLDisplay:          eglInfo.Display.String(),
		EGLVendor:           eglInfo.Vendor,
		EGLVersion:          eglInfo.Version,
		EGLClientAPIs:       eglInfo.ClientAPIs,
		EGLExtensions:       eglInfo.Extensions.Names(),
		EGLExtensionString:  eglInfo.ExtensionString,
		ContextStrategy:     ContextStrategyGtkGLAreaCurrent,
		DMABUFImportSupport: eglInfo.SupportsDMABUFImport(),
	}
	return result, result.Validate()
}

// DetectBackendFromDisplay classifies the active GDK display for the Wayland-only gate.
func DetectBackendFromDisplay(display *gdk.Display) string {
	if display != nil {
		name := strings.ToLower(display.GetName())
		if strings.Contains(name, "wayland") {
			return BackendWayland
		}
		// X11 display names typically start with :N (or hostname:N).
		if strings.HasPrefix(name, ":") || strings.Contains(name, "x11") {
			return BackendX11
		}
	}
	return DetectRuntimeBackend()
}

// DetectRuntimeBackend returns the likely backend from environment when no GDK display is available.
func DetectRuntimeBackend() string {
	if os.Getenv("GDK_BACKEND") == BackendWayland {
		return BackendWayland
	}
	if os.Getenv("WAYLAND_DISPLAY") != "" && os.Getenv("GDK_BACKEND") != "x11" {
		return BackendWayland
	}
	if os.Getenv("DISPLAY") != "" || os.Getenv("GDK_BACKEND") == "x11" {
		return BackendX11
	}
	return BackendUnknown
}

func glAPIName(areaAPI, contextAPI gdk.GLAPI) string {
	api := contextAPI
	if api == 0 {
		api = areaAPI
	}
	switch api {
	case gdk.GlApiGlValue:
		return "opengl"
	case gdk.GlApiGlesValue:
		return "opengles"
	default:
		return "unknown"
	}
}
