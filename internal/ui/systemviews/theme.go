package systemviews

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/bnema/dumber/internal/application/port"
)

type shellTheme struct {
	RootClass  string
	InlineVars string
}

const (
	shellDarkClass  = "sv-dark"
	shellLightClass = "sv-light"
)

var cssNamedColors = func() map[string]struct{} {
	names := strings.Fields(`aliceblue antiquewhite aqua aquamarine azure beige bisque black blanchedalmond blue blueviolet brown burlywood cadetblue chartreuse chocolate coral cornflowerblue cornsilk crimson cyan darkblue darkcyan darkgoldenrod darkgray darkgreen darkgrey darkkhaki darkmagenta darkolivegreen darkorange darkorchid darkred darksalmon darkseagreen darkslateblue darkslategray darkslategrey darkturquoise darkviolet deeppink deepskyblue dimgray dimgrey dodgerblue firebrick floralwhite forestgreen fuchsia gainsboro ghostwhite gold goldenrod gray green greenyellow grey honeydew hotpink indianred indigo ivory khaki lavender lavenderblush lawngreen lemonchiffon lightblue lightcoral lightcyan lightgoldenrodyellow lightgray lightgreen lightgrey lightpink lightsalmon lightseagreen lightskyblue lightslategray lightslategrey lightsteelblue lightyellow lime limegreen linen magenta maroon mediumaquamarine mediumblue mediumorchid mediumpurple mediumseagreen mediumslateblue mediumspringgreen mediumturquoise mediumvioletred midnightblue mintcream mistyrose moccasin navajowhite navy oldlace olive olivedrab orange orangered orchid palegoldenrod palegreen paleturquoise palevioletred papayawhip peachpuff peru pink plum powderblue purple rebeccapurple red rosybrown royalblue saddlebrown salmon sandybrown seagreen seashell sienna silver skyblue slateblue slategray slategrey snow springgreen steelblue tan teal thistle tomato turquoise violet wheat white whitesmoke yellow yellowgreen`)
	colors := make(map[string]struct{}, len(names))
	for _, name := range names {
		colors[name] = struct{}{}
	}
	return colors
}()

func currentPrefersDark() bool {
	return currentPrefersDarkImpl()
}

func resolveShellTheme(appearance port.WebUIAppearanceConfig) shellTheme {
	if isZeroAppearance(appearance) {
		return shellTheme{}
	}

	var palette port.ColorPalette
	var rootClass string

	switch strings.ToLower(strings.TrimSpace(appearance.ColorScheme)) {
	case "prefer-dark":
		palette = appearance.DarkPalette
		rootClass = shellDarkClass
	case "prefer-light":
		palette = appearance.LightPalette
		rootClass = shellLightClass
	default:
		if currentPrefersDark() {
			palette = appearance.DarkPalette
			rootClass = shellDarkClass
		} else {
			palette = appearance.LightPalette
			rootClass = shellLightClass
		}
	}

	return shellTheme{
		RootClass:  rootClass,
		InlineVars: buildInlineVars(appearance, palette),
	}
}

func buildInlineVars(appearance port.WebUIAppearanceConfig, palette port.ColorPalette) string {
	parts := make([]string, 0, 11)
	appendInlineVar := func(name, value string, sanitize func(string) (string, bool)) {
		value, ok := sanitize(value)
		if !ok {
			return
		}
		parts = append(parts, fmt.Sprintf("%s: %s;", name, value))
	}

	appendInlineVar("--sv-background", palette.Background, sanitizeCSSColor)
	appendInlineVar("--sv-surface", palette.Surface, sanitizeCSSColor)
	appendInlineVar("--sv-surface-variant", palette.SurfaceVariant, sanitizeCSSColor)
	appendInlineVar("--sv-text", palette.Text, sanitizeCSSColor)
	appendInlineVar("--sv-muted", palette.Muted, sanitizeCSSColor)
	appendInlineVar("--sv-accent", palette.Accent, sanitizeCSSColor)
	appendInlineVar("--sv-border", palette.Border, sanitizeCSSColor)
	appendInlineVar("--sv-font-sans", appearance.SansFont, sanitizeCSSFontFamily)
	appendInlineVar("--sv-font-serif", appearance.SerifFont, sanitizeCSSFontFamily)
	appendInlineVar("--sv-font-mono", appearance.MonospaceFont, sanitizeCSSFontFamily)
	if appearance.DefaultFontSize > 0 {
		parts = append(parts, fmt.Sprintf("--sv-font-size: %dpx;", appearance.DefaultFontSize))
	}

	return strings.Join(parts, " ")
}

func sanitizeCSSColor(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" || hasCSSBreakout(value) {
		return "", false
	}
	lower := strings.ToLower(value)
	if isHexColor(lower) || isFunctionalColor(lower) || isNamedColor(lower) {
		return value, true
	}
	return "", false
}

func sanitizeCSSFontFamily(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" || hasCSSBreakout(value) {
		return "", false
	}

	validated := value
	if quote := value[0]; quote == '\'' || quote == '"' {
		if len(value) < 2 || value[len(value)-1] != quote {
			return "", false
		}
		validated = value[1 : len(value)-1]
		if validated == "" || strings.ContainsAny(validated, "'\"") {
			return "", false
		}
	} else if strings.ContainsAny(value, "'\"") {
		return "", false
	}

	for _, r := range validated {
		switch {
		case unicode.IsLetter(r), unicode.IsMark(r), unicode.IsDigit(r):
		case strings.ContainsRune(" ,._-", r):
		default:
			return "", false
		}
	}
	return value, true
}

func hasCSSBreakout(value string) bool {
	return strings.ContainsAny(value, ";{}\n\r") || strings.Contains(value, "/*") || strings.Contains(value, "*/")
}

func isHexColor(value string) bool {
	if len(value) != 4 && len(value) != 5 && len(value) != 7 && len(value) != 9 {
		return false
	}
	if value[0] != '#' {
		return false
	}
	for _, r := range value[1:] {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
			return false
		}
	}
	return true
}

func isFunctionalColor(value string) bool {
	prefixes := []string{"rgb(", "rgba(", "hsl(", "hsla(", "oklch(", "oklab(", "color(", "lab(", "lch(", "hwb("}
	matched := false
	for _, prefix := range prefixes {
		if strings.HasPrefix(value, prefix) {
			matched = true
			break
		}
	}
	if !matched || !strings.HasSuffix(value, ")") {
		return false
	}
	for _, r := range strings.TrimSuffix(value[strings.IndexByte(value, '(')+1:], ")") {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || strings.ContainsRune(" .,%/-", r)) {
			return false
		}
	}
	return true
}

func isNamedColor(value string) bool {
	if value == "transparent" || value == "currentcolor" {
		return true
	}
	_, ok := cssNamedColors[value]
	return ok
}

func isZeroAppearance(appearance port.WebUIAppearanceConfig) bool {
	return appearance.ColorScheme == "" &&
		appearance.SansFont == "" &&
		appearance.SerifFont == "" &&
		appearance.MonospaceFont == "" &&
		appearance.DefaultFontSize == 0 &&
		appearance.LightPalette == (port.ColorPalette{}) &&
		appearance.DarkPalette == (port.ColorPalette{})
}
