// Package color provides pure hex color parsing and WCAG contrast helpers
// shared by the GTK theme and the web system views.
package color

import "math"

const (
	hexLenShort     = 3   // #RGB
	hexLenStandard  = 6   // #RRGGBB
	hexLenWithAlpha = 8   // #RRGGBBAA
	maxColorValue   = 255 // Maximum value for 8-bit color channel

	// MinimumReadableContrast is the WCAG AA ratio for normal text.
	MinimumReadableContrast = 4.5

	contrastLuminanceOffset = 0.05
	sRGBLinearThreshold     = 0.04045
	sRGBLinearDivisor       = 12.92
	sRGBGammaOffset         = 0.055
	sRGBGammaScale          = 1.055
	sRGBGammaExponent       = 2.4
	redLuminanceWeight      = 0.2126
	greenLuminanceWeight    = 0.7152
	blueLuminanceWeight     = 0.0722
)

// HexToRGBA converts a hex color string to RGBA float32 values (0.0-1.0).
// Supports #RGB, #RRGGBB, and #RRGGBBAA formats.
// Returns opaque black (0,0,0,1) if parsing fails.
func HexToRGBA(hex string) (r, g, b, a float32) {
	if hex == "" || hex[0] != '#' {
		return 0, 0, 0, 1
	}
	hex = hex[1:]

	var ri, gi, bi, ai uint64
	switch len(hex) {
	case hexLenShort:
		ri = parseHex(hex[0:1]) * 17 // 0xF -> 0xFF
		gi = parseHex(hex[1:2]) * 17
		bi = parseHex(hex[2:3]) * 17
		ai = maxColorValue
	case hexLenStandard:
		ri = parseHex(hex[0:2])
		gi = parseHex(hex[2:4])
		bi = parseHex(hex[4:6])
		ai = maxColorValue
	case hexLenWithAlpha:
		ri = parseHex(hex[0:2])
		gi = parseHex(hex[2:4])
		bi = parseHex(hex[4:6])
		ai = parseHex(hex[6:8])
	default:
		return 0, 0, 0, 1
	}

	return float32(ri) / maxColorValue, float32(gi) / maxColorValue, float32(bi) / maxColorValue, float32(ai) / maxColorValue
}

// parseHex parses hex digits; any invalid digit makes the whole channel zero.
func parseHex(s string) uint64 {
	var result uint64
	for _, c := range s {
		result *= 16
		switch {
		case c >= '0' && c <= '9':
			result += uint64(c - '0')
		case c >= 'a' && c <= 'f':
			result += uint64(c - 'a' + 10)
		case c >= 'A' && c <= 'F':
			result += uint64(c - 'A' + 10)
		default:
			return 0
		}
	}
	return result
}

// ReadableTextColor returns preferred when it reaches MinimumReadableContrast
// on background, otherwise whichever of white or black contrasts more.
func ReadableTextColor(background, preferred string) string {
	if ContrastRatio(background, preferred) >= MinimumReadableContrast {
		return preferred
	}
	if ContrastRatio(background, "#ffffff") >= ContrastRatio(background, "#000000") {
		return "#ffffff"
	}
	return "#000000"
}

// ContrastRatio returns the WCAG contrast ratio between two hex colors.
func ContrastRatio(first, second string) float64 {
	firstLuminance := relativeLuminance(first)
	secondLuminance := relativeLuminance(second)
	if firstLuminance < secondLuminance {
		firstLuminance, secondLuminance = secondLuminance, firstLuminance
	}
	return (firstLuminance + contrastLuminanceOffset) / (secondLuminance + contrastLuminanceOffset)
}

func relativeLuminance(hex string) float64 {
	r, g, b, _ := HexToRGBA(hex)
	linearize := func(channel float32) float64 {
		value := float64(channel)
		if value <= sRGBLinearThreshold {
			return value / sRGBLinearDivisor
		}
		return math.Pow((value+sRGBGammaOffset)/sRGBGammaScale, sRGBGammaExponent)
	}
	return redLuminanceWeight*linearize(r) +
		greenLuminanceWeight*linearize(g) +
		blueLuminanceWeight*linearize(b)
}
