package cef2gtk

import (
	"fmt"
	"os"
	"strings"
)

// Backend selects the GTK presentation backend used by a View.
type Backend string

const (
	// BackendAuto tries the GDK DMABUF backend first and falls back to GLArea
	// when GDK DMABUF construction is unavailable.
	BackendAuto Backend = "auto"
	// BackendGDKDMABUF presents CEF DMABUF frames as GDK textures.
	BackendGDKDMABUF Backend = "gdk-dmabuf"
	// BackendGLArea presents CEF DMABUF frames through GtkGLArea/EGL/OpenGL.
	BackendGLArea Backend = "glarea"
)

const backendEnvVar = "PUREGO_CEF2GTK_BACKEND"

// String returns the environment/API spelling for b.
func (b Backend) String() string {
	if b == "" {
		return string(BackendAuto)
	}
	return string(b)
}

// ViewOptions configures NewViewWithOptions.
type ViewOptions struct {
	// Backend selects the renderer backend. Empty defaults to the Vulkan stack's
	// GDK DMABUF backend unless RenderStackPlan is provided.
	Backend Backend
	// RenderStackPlan selects the renderer backend from a resolved render stack.
	RenderStackPlan RenderStackPlan
	// Profile optionally enables development render profiling during construction.
	Profile ProfileOptions
	// ScaleMultiplier is an application-level multiplier composed on top of the
	// GTK/GDK surface scale for CEF OSR screen, backing, and input coordinates.
	// Values <= 0, NaN, or Inf are treated as 1.
	ScaleMultiplier float64
}

// Validate verifies that the requested backend/render stack is supported by the option schema.
func (o ViewOptions) Validate() error {
	_, err := o.normalized()
	return err
}

func (o ViewOptions) normalized() (ViewOptions, error) {
	if o.RenderStackPlan.Backend != "" {
		o.Backend = o.RenderStackPlan.Backend
	}
	backend, err := normalizeBackend(o.Backend)
	if err != nil {
		return ViewOptions{}, err
	}
	if o.Backend == "" && backend == BackendAuto && o.RenderStackPlan.Backend == "" {
		backend = BackendGDKDMABUF
	}
	o.Backend = backend
	return o, nil
}

func resolveViewOptions(opts ViewOptions) (ViewOptions, error) {
	planProvided := opts.RenderStackPlan.Backend != ""
	normalized, err := opts.normalized()
	if err != nil {
		return ViewOptions{}, err
	}
	if planProvided {
		return normalized, nil
	}
	if envBackend, ok, err := backendFromEnv(); err != nil {
		return ViewOptions{}, err
	} else if ok {
		normalized.Backend = envBackend
	}
	return normalized, nil
}

func normalizeBackend(backend Backend) (Backend, error) {
	if backend == "" {
		return BackendAuto, nil
	}
	parsed, ok := parseBackend(string(backend))
	if !ok {
		return "", fmt.Errorf("unsupported backend %q", backend)
	}
	return parsed, nil
}

func parseBackend(value string) (Backend, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", string(BackendAuto):
		return BackendAuto, true
	case string(BackendGDKDMABUF):
		return BackendGDKDMABUF, true
	case string(BackendGLArea):
		return BackendGLArea, true
	default:
		return "", false
	}
}

func backendFromEnv() (Backend, bool, error) {
	value, ok := os.LookupEnv(backendEnvVar)
	if !ok {
		return "", false, nil
	}
	backend, valid := parseBackend(value)
	if !valid {
		return "", true, fmt.Errorf("unsupported %s %q", backendEnvVar, value)
	}
	return backend, true, nil
}

// RenderingMode selects the rendering pipeline used by the GTK bridge.
type RenderingMode string

const (
	// RenderingModeAcceleratedDMABUF uses CEF accelerated OSR shared textures
	// imported through Wayland DMABUF/EGL and rendered by GtkGLArea.
	RenderingModeAcceleratedDMABUF RenderingMode = "accelerated-dmabuf"
)

// Options configures a View.
type Options struct {
	// RenderingMode must be RenderingModeAcceleratedDMABUF. It defaults to that
	// mode when left empty.
	RenderingMode RenderingMode
}

// Validate verifies that the options request the supported accelerated path.
func (o Options) Validate() error {
	mode := o.RenderingMode
	if mode == "" {
		mode = RenderingModeAcceleratedDMABUF
	}
	if mode != RenderingModeAcceleratedDMABUF {
		return fmt.Errorf("unsupported rendering mode %q", o.RenderingMode)
	}
	return nil
}

// normalized returns options with defaults applied after validation.
func (o Options) normalized() (Options, error) {
	if o.RenderingMode == "" {
		o.RenderingMode = RenderingModeAcceleratedDMABUF
	}
	if err := o.Validate(); err != nil {
		return Options{}, err
	}
	return o, nil
}
