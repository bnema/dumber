package systemviews

import (
	"fmt"
	"strings"

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
	appendInlineVar := func(name, value string) {
		if strings.TrimSpace(value) == "" {
			return
		}
		parts = append(parts, fmt.Sprintf("%s: %s;", name, value))
	}

	appendInlineVar("--sv-background", palette.Background)
	appendInlineVar("--sv-surface", palette.Surface)
	appendInlineVar("--sv-surface-variant", palette.SurfaceVariant)
	appendInlineVar("--sv-text", palette.Text)
	appendInlineVar("--sv-muted", palette.Muted)
	appendInlineVar("--sv-accent", palette.Accent)
	appendInlineVar("--sv-border", palette.Border)
	appendInlineVar("--sv-font-sans", appearance.SansFont)
	appendInlineVar("--sv-font-serif", appearance.SerifFont)
	appendInlineVar("--sv-font-mono", appearance.MonospaceFont)
	if appearance.DefaultFontSize > 0 {
		parts = append(parts, fmt.Sprintf("--sv-font-size: %dpx;", appearance.DefaultFontSize))
	}

	return strings.Join(parts, " ")
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
