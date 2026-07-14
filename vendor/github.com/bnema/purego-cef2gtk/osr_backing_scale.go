package cef2gtk

import (
	"math"
	"os"
	"strings"
	"sync"
)

const osrBackingScaleEnvVar = "PUREGO_CEF2GTK_OSR_BACKING_SCALE"

var (
	cachedOsrBackingScaleMode osrBackingScaleMode
	cachedOsrBackingScaleOnce sync.Once
)

type osrBackingScaleMode uint8

const (
	osrBackingScaleOff osrBackingScaleMode = iota
	osrBackingScaleOn
	osrBackingScaleAuto
)

func osrBackingScaleModeFromEnv() osrBackingScaleMode {
	cachedOsrBackingScaleOnce.Do(func() {
		cachedOsrBackingScaleMode = parseOSRBackingScaleMode(os.Getenv(osrBackingScaleEnvVar))
	})
	return cachedOsrBackingScaleMode
}

func parseOSRBackingScaleMode(value string) osrBackingScaleMode {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on", "device":
		return osrBackingScaleOn
	case "auto":
		return osrBackingScaleAuto
	default:
		return osrBackingScaleOff
	}
}

// OSRBackingScaleEnabledForScale reports whether the Linux accelerated OSR
// HiDPI compatibility path is active for the provided GTK surface scale.
//
// Current CEF shared-texture OSR builds can accept a fractional
// CefScreenInfo.device_scale_factor while still emitting 1x/logical DMABUF
// frames. When this compatibility path is active, purego-cef2gtk asks CEF for a
// device-sized OSR view rect and reports a 1x screen scale. Applications that
// expose page zoom should compensate their internal CEF zoom level by multiplying
// the user-facing page zoom by OSRBackingScaleFactorForScale, then divide CEF
// zoom readback by the same factor. This keeps the page's CSS viewport at the
// GTK logical size while the OSR backing is device-sized.
func OSRBackingScaleEnabledForScale(scale float64) bool {
	return osrBackingScaleEnabledForScale(scale)
}

func osrBackingScaleEnabledForScale(scale float64) bool {
	scale = normalizeDeviceScale(scale)
	switch osrBackingScaleModeFromEnv() {
	case osrBackingScaleOn:
		return true
	case osrBackingScaleAuto:
		return scale > 1
	default:
		return false
	}
}

// OSRBackingScaleFactorForScale returns the backing scale that should be used
// for CEF OSR view/input coordinates. It returns 1 when compatibility scaling
// is disabled for the provided surface scale.
func OSRBackingScaleFactorForScale(scale float64) float64 {
	scale = normalizeDeviceScale(scale)
	if !osrBackingScaleEnabledForScale(scale) {
		return 1
	}
	return scale
}

func (v *View) osrBackingScaleEnabled() bool {
	if v == nil {
		return false
	}
	return osrBackingScaleEnabledForScale(float64(v.DeviceScaleFactor()))
}

func (v *View) osrBackingScale() float64 {
	if v == nil {
		return 1
	}
	return OSRBackingScaleFactorForScale(float64(v.DeviceScaleFactor()))
}

func (v *View) osrViewRectSize() (int32, int32) {
	width, height := v.cachedSize()
	scale := v.osrBackingScale()
	return scaleDimension(width, scale), scaleDimension(height, scale)
}

func (v *View) osrScreenInfoScale() float32 {
	if v.osrBackingScaleEnabled() {
		return 1
	}
	return v.DeviceScaleFactor()
}

func (v *View) osrScreenPoint(viewX, viewY int32) (int32, int32) {
	if v.osrBackingScaleEnabled() {
		// With device-sized OSR backing CEF view coordinates are already device
		// pixels because GetScreenInfo reports a 1x scale.
		return viewX, viewY
	}
	scale := normalizeDeviceScale(float64(v.DeviceScaleFactor()))
	return scaleCoordinate(viewX, scale), scaleCoordinate(viewY, scale)
}

func inputScaleForOSRBacking(fallback float64) float64 {
	return OSRBackingScaleFactorForScale(fallback)
}

func scaleDimension(value int32, scale float64) int32 {
	if value <= 0 {
		return 1
	}
	scale = normalizeDeviceScale(scale)
	if scale <= 1 {
		return value
	}
	return int32(math.Ceil(float64(value) * scale))
}

func scaleCoordinate(value int32, scale float64) int32 {
	return int32(math.Floor(float64(value) * normalizeDeviceScale(scale)))
}
