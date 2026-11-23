package dto

import (
	"testing"

	"github.com/stretchr/testify/require"

	"bnema.dev/dumber/internal/domain/entity"
)

func TestWebUIAppearanceWithResolvedThemeUsesResolvedValues(t *testing.T) {
	base := WebUIAppearanceConfig{
		ColorScheme: "prefer-dark",
		ExternalTheme: WebUIExternalThemeConfig{
			Enabled:  true,
			Provider: "noctalia",
			Format:   "dumber-json",
			Path:     "/tmp/theme.json",
		},
	}
	resolved := entity.ResolvedTheme{
		Fonts: entity.ThemeFonts{
			SansFont:      "Inter",
			SerifFont:     "Georgia",
			MonospaceFont: "JetBrains Mono",
			GtkFont:       "Adwaita Sans",
			DefaultSize:   18,
		},
		LightPalette: entity.ColorPalette{Background: "#ffffff", Accent: "#0055ff"},
		DarkPalette:  entity.ColorPalette{Background: "#111111", Accent: "#66aaff"},
		PrefersDark:  true,
	}

	got := WebUIAppearanceWithResolvedTheme(base, resolved)

	require.Equal(t, "prefer-dark", got.ColorScheme)
	require.Equal(t, base.ExternalTheme, got.ExternalTheme)
	require.Equal(t, "Inter", got.SansFont)
	require.Equal(t, "Georgia", got.SerifFont)
	require.Equal(t, "JetBrains Mono", got.MonospaceFont)
	require.Equal(t, "Adwaita Sans", got.GtkFont)
	require.Equal(t, 18, got.DefaultFontSize)
	require.Equal(t, "#ffffff", got.LightPalette.Background)
	require.Equal(t, "#0055ff", got.LightPalette.Accent)
	require.Equal(t, "#111111", got.DarkPalette.Background)
	require.Equal(t, "#66aaff", got.DarkPalette.Accent)
}

func TestWebUIAppearanceWithResolvedThemeResolvesDefaultColorScheme(t *testing.T) {
	base := WebUIAppearanceConfig{ColorScheme: "default"}

	require.Equal(t, "prefer-dark", WebUIAppearanceWithResolvedTheme(base, entity.ResolvedTheme{PrefersDark: true}).ColorScheme)
	require.Equal(t, "prefer-light", WebUIAppearanceWithResolvedTheme(base, entity.ResolvedTheme{PrefersDark: false}).ColorScheme)
	require.Equal(t, "default", base.ColorScheme, "the editable config keeps its value")
}
