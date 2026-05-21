package cef

import "math"

func normalizeScale(scale float64) float64 {
	if math.IsNaN(scale) || math.IsInf(scale, 0) || scale <= 0 {
		return 1
	}
	return scale
}

func deviceToLogicalCoord(value int32, scale float64) int32 {
	return int32(math.Floor(float64(value) / normalizeScale(scale)))
}

func deviceToLogicalSize(value int32, scale float64) int32 {
	if value <= 0 {
		return 0
	}
	scaled := int32(math.Ceil(float64(value) / normalizeScale(scale)))
	if scaled < 1 {
		return 1
	}
	return scaled
}
