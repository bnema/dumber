package theme

import (
	"context"
	"testing"

	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func resolvedThemeFixture(prefersDark bool) entity.ResolvedTheme {
	light := entity.ColorPalette{
		Background:     "#ffffff",
		Surface:        "#f8f8f8",
		SurfaceVariant: "#eeeeee",
		Text:           "#111111",
		Muted:          "#666666",
		Accent:         "#0055ff",
		Border:         "#dddddd",
	}
	dark := entity.ColorPalette{
		Background:     "#101010",
		Surface:        "#181818",
		SurfaceVariant: "#242424",
		Text:           "#f5f5f5",
		Muted:          "#999999",
		Accent:         "#66aaff",
		Border:         "#333333",
	}
	active := light
	if prefersDark {
		active = dark
	}
	return entity.ResolvedTheme{
		LightPalette:      light,
		DarkPalette:       dark,
		ActivePalette:     active,
		PrefersDark:       prefersDark,
		ColorSchemeSource: "test",
		ThemeSource:       entity.ThemeSourceMetadata{Kind: entity.ThemeSourceConfig},
		Fonts: entity.ThemeFonts{
			SansFont:      "Inter",
			SerifFont:     "Georgia",
			MonospaceFont: "JetBrains Mono",
			GtkFont:       "Adwaita Sans",
			DefaultSize:   16,
		},
		UIScale: 1.5,
		ModeColors: entity.ThemeModeColors{
			PaneMode:    "#ff0000",
			TabMode:     "#00ff00",
			SessionMode: "#0000ff",
			ResizeMode:  "#ffff00",
		},
	}
}

func TestNewManager_UsesResolvedThemeFields(t *testing.T) {
	ctx := context.Background()
	resolved := resolvedThemeFixture(true)

	manager := NewManager(ctx, resolved)

	require.NotNil(t, manager)
	assert.True(t, manager.PrefersDark())
	assert.Equal(t, "#ffffff", manager.GetLightPalette().Background)
	assert.Equal(t, "#101010", manager.GetDarkPalette().Background)
	assert.Equal(t, manager.GetDarkPalette(), manager.GetCurrentPalette())
	assert.Equal(t, "#ff0000", manager.GetModeColors().PaneMode)
	assert.Equal(t, FontConfig{SansFont: "Inter", MonospaceFont: "JetBrains Mono", GtkFont: "Adwaita Sans"}, manager.fonts)
	assert.Equal(t, "Adwaita Sans", manager.gtkFont)
	assert.InDelta(t, 1.5, manager.uiScale, 0.000001)
}

func TestNewManager_DefaultsInvalidScaleToOne(t *testing.T) {
	ctx := context.Background()
	resolved := resolvedThemeFixture(true)
	resolved.UIScale = 0

	manager := NewManager(ctx, resolved)

	require.NotNil(t, manager)
	assert.InDelta(t, 1.0, manager.uiScale, 0.000001)
}

func TestManager_UpdateFromResolvedReplacesResolvedValues(t *testing.T) {
	ctx := context.Background()
	manager := NewManager(ctx, resolvedThemeFixture(true))
	updated := resolvedThemeFixture(false)
	updated.LightPalette.Background = "#abcdef"
	updated.DarkPalette.Background = "#123456"
	updated.Fonts.SansFont = "Recursive"
	updated.UIScale = 2.0
	updated.ModeColors.PaneMode = "#654321"

	manager.UpdateFromResolved(ctx, updated, nil)

	assert.False(t, manager.PrefersDark())
	assert.Equal(t, "#abcdef", manager.GetLightPalette().Background)
	assert.Equal(t, "#123456", manager.GetDarkPalette().Background)
	assert.Equal(t, manager.GetLightPalette(), manager.GetCurrentPalette())
	assert.Equal(t, "Recursive", manager.fonts.SansFont)
	assert.InDelta(t, 2.0, manager.uiScale, 0.000001)
	assert.Equal(t, "#654321", manager.GetModeColors().PaneMode)
}

func TestManager_GetWebUIThemeCSS(t *testing.T) {
	ctx := context.Background()
	manager := NewManager(ctx, resolvedThemeFixture(true))

	css := manager.GetWebUIThemeCSS()

	assert.Contains(t, css, ":root{")
	assert.Contains(t, css, ".dark{")
	assert.Contains(t, css, "--background: #ffffff")
	assert.Contains(t, css, "--background: #101010")
}

func TestManager_GetBackgroundRGBAUsesCurrentPalette(t *testing.T) {
	ctx := context.Background()
	manager := NewManager(ctx, resolvedThemeFixture(true))

	r, g, b, a := manager.GetBackgroundRGBA()

	assert.InDelta(t, float32(0x10)/255, r, 0.0001)
	assert.InDelta(t, float32(0x10)/255, g, 0.0001)
	assert.InDelta(t, float32(0x10)/255, b, 0.0001)
	assert.InDelta(t, float32(1), a, 0.0001)
}

func TestPaletteFromEntityAddsSemanticDefaults(t *testing.T) {
	palette := PaletteFromEntity(entity.ColorPalette{Background: "#ffffff", Text: "#111111"}, false)

	assert.Equal(t, "#ffffff", palette.Background)
	assert.Equal(t, "#111111", palette.Text)
	assert.Equal(t, DefaultLightPalette().Success, palette.Success)
	assert.Equal(t, DefaultLightPalette().Warning, palette.Warning)
	assert.Equal(t, DefaultLightPalette().Destructive, palette.Destructive)
}

func TestModeColorsFromEntity(t *testing.T) {
	modeColors := ModeColorsFromEntity(entity.ThemeModeColors{
		PaneMode:    "#111111",
		TabMode:     "#222222",
		SessionMode: "#333333",
		ResizeMode:  "#444444",
	})

	assert.Equal(t, "#111111", modeColors.PaneMode)
	assert.Equal(t, "#222222", modeColors.TabMode)
	assert.Equal(t, "#333333", modeColors.SessionMode)
	assert.Equal(t, "#444444", modeColors.ResizeMode)
}

func TestFontConfigFromEntity(t *testing.T) {
	fonts := FontConfigFromEntity(entity.ThemeFonts{
		SansFont:      "Inter",
		MonospaceFont: "JetBrains Mono",
		GtkFont:       "Adwaita Sans",
	})

	assert.Equal(t, FontConfig{SansFont: "Inter", MonospaceFont: "JetBrains Mono", GtkFont: "Adwaita Sans"}, fonts)
}

func TestDefaultGTKFont(t *testing.T) {
	assert.Equal(t, "Adwaita Sans", DefaultGTKFont())
}

func TestFormatGTKFontName_UsesScaledConfiguredGTKFont(t *testing.T) {
	assert.Equal(t, "Adwaita Sans 14", formatGTKFontName("Adwaita Sans", 1.3))
}

func TestFormatGTKFontName_DefaultsScaleAndRoundsToNearestPoint(t *testing.T) {
	assert.Equal(t, "Inter 11", formatGTKFontName("Inter", 0))
	assert.Equal(t, "Inter 12", formatGTKFontName("Inter", 1.05))
	assert.Equal(t, "Adwaita Sans 11", formatGTKFontName("", 1.0))
}

func TestShouldApplyGTKFontName(t *testing.T) {
	assert.True(t, shouldApplyGTKFontName("", "Fira Sans 14"))
	assert.False(t, shouldApplyGTKFontName("Fira Sans 14", "Fira Sans 14"))
	assert.False(t, shouldApplyGTKFontName("Fira Sans 14", ""))
}
