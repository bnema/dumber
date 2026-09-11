package entity

import (
	"math"
	"time"
)

// ZoomLevel represents the zoom factor for a specific domain.
// Allows users to set persistent zoom levels per-site.
type ZoomLevel struct {
	Domain     string  // Domain name (e.g., "github.com")
	ZoomFactor float64 // Zoom factor (1.0 = 100%, 1.5 = 150%)
	UpdatedAt  time.Time
}

// Default zoom constants
const (
	ZoomDefault = 1.0
	ZoomMin     = 0.25 // 25%
	ZoomMax     = 5.0  // 500%
	ZoomStep    = 0.1  // 10% increments
)

// NewZoomLevel creates a new zoom level for a domain.
func NewZoomLevel(domain string, factor float64) *ZoomLevel {
	return &ZoomLevel{
		Domain:     domain,
		ZoomFactor: clampZoom(factor),
		UpdatedAt:  time.Now(),
	}
}

// SetFactor updates the zoom factor, clamping to valid range.
func (z *ZoomLevel) SetFactor(factor float64) {
	z.ZoomFactor = clampZoom(factor)
	z.UpdatedAt = time.Now()
}

// ZoomIn increases the zoom factor by one step.
func (z *ZoomLevel) ZoomIn() {
	z.SetFactor(z.ZoomFactor + ZoomStep)
}

// ZoomOut decreases the zoom factor by one step.
func (z *ZoomLevel) ZoomOut() {
	z.SetFactor(z.ZoomFactor - ZoomStep)
}

// Reset restores the zoom factor to default.
func (z *ZoomLevel) Reset() {
	z.SetFactor(ZoomDefault)
}

// IsDefault returns true if the zoom is at default level.
func (z *ZoomLevel) IsDefault() bool {
	return z.ZoomFactor == ZoomDefault
}

// Percentage returns the zoom factor as a whole percentage (e.g., 150 for 1.5).
// Accumulated floating-point error is rounded away so a factor that equals the
// default zoom in intent (0.9999999999999997) displays as 100, not 99.
func (z *ZoomLevel) Percentage() int {
	return int(math.Round(z.ZoomFactor * 100))
}

// clampZoom constrains a zoom factor to the valid range.
func clampZoom(factor float64) float64 {
	if factor < ZoomMin {
		return ZoomMin
	}
	if factor > ZoomMax {
		return ZoomMax
	}
	return factor
}
