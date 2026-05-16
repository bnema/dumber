package gtkutil

import (
	"math"

	"github.com/bnema/puregotk/v4/gtk"
)

// NormalizeScale clamps invalid GTK/GDK scale values to 1.
func NormalizeScale(scale float64) float64 {
	if math.IsNaN(scale) || math.IsInf(scale, 0) || scale <= 0 {
		return 1
	}
	return scale
}

// WidgetFromNativePointer wraps a native GTK widget pointer.
func WidgetFromNativePointer(ptr uintptr) *gtk.Widget {
	if ptr == 0 {
		return nil
	}
	return gtk.WidgetNewFromInternalPtr(ptr)
}

// DeviceToLogical converts a device-pixel coordinate into a logical coordinate.
func DeviceToLogical(value int32, scale float64) int32 {
	return int32(math.Floor(float64(value) / NormalizeScale(scale)))
}
