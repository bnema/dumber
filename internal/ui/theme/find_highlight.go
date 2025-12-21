package theme

import (
	"fmt"
	"strconv"
	"strings"
)

// GenerateFindHighlightCSS builds CSS to style find-in-page matches using the accent color.
func GenerateFindHighlightCSS(p Palette) string {
	accent := strings.TrimSpace(p.Accent)
	dim := rgbaFromHex(accent, 0.50)
	active := accent

	return fmt.Sprintf(`/* Find highlight styling */
::-webkit-search-results {
	background-color: %s !important;
}

::-webkit-search-results-decoration {
	background-color: %s !important;
}

::selection {
	background-color: %s !important;
}
`, dim, active, active)
}

func rgbaFromHex(hex string, alpha float64) string {
	if len(hex) != 7 || !strings.HasPrefix(hex, "#") {
		return hex
	}

	r, rOK := parseHexByte(hex[1:3])
	g, gOK := parseHexByte(hex[3:5])
	b, bOK := parseHexByte(hex[5:7])
	if !rOK || !gOK || !bOK {
		return hex
	}

	if alpha < 0 {
		alpha = 0
	}
	if alpha > 1 {
		alpha = 1
	}

	return fmt.Sprintf("rgba(%d, %d, %d, %.2f)", r, g, b, alpha)
}

func parseHexByte(value string) (uint64, bool) {
	parsed, err := strconv.ParseUint(value, 16, 8)
	if err != nil {
		return 0, false
	}
	return parsed, true
}
