package styles_test

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/stretchr/testify/require"

	"github.com/bnema/dumber/internal/cli/styles"
	"github.com/bnema/dumber/internal/domain/entity"
)

func TestNewThemeFromResolvedUsesActivePalette(t *testing.T) {
	resolved := entity.ResolvedTheme{
		ActivePalette: entity.ColorPalette{
			Background:     "#010101",
			Surface:        "#020202",
			SurfaceVariant: "#030303",
			Text:           "#fefefe",
			Muted:          "#999999",
			Accent:         "#abcdef",
			Border:         "#444444",
		},
	}

	theme := styles.NewThemeFromResolved(resolved)

	require.Equal(t, lipgloss.Color("#010101"), theme.Background)
	require.Equal(t, lipgloss.Color("#abcdef"), theme.Accent)
	require.Equal(t, lipgloss.Color("#abcdef"), theme.Success)
}

func TestNewThemeFromResolvedFallsBackToDarkPalette(t *testing.T) {
	resolved := entity.ResolvedTheme{
		DarkPalette: entity.ColorPalette{
			Background:     "#111111",
			Surface:        "#222222",
			SurfaceVariant: "#333333",
			Text:           "#eeeeee",
			Muted:          "#888888",
			Accent:         "#fedcba",
			Border:         "#444444",
		},
	}

	theme := styles.NewThemeFromResolved(resolved)

	require.Equal(t, lipgloss.Color("#111111"), theme.Background)
	require.Equal(t, lipgloss.Color("#fedcba"), theme.Accent)
}

func TestNewThemeFromResolvedFallsBackToDefaultDarkPalette(t *testing.T) {
	resolved := entity.ResolvedTheme{}
	palette := styles.DefaultDarkPalette()

	theme := styles.NewThemeFromResolved(resolved)

	require.Equal(t, lipgloss.Color(palette.Background), theme.Background)
	require.Equal(t, lipgloss.Color(palette.Surface), theme.Surface)
	require.Equal(t, lipgloss.Color(palette.Text), theme.Text)
	require.Equal(t, lipgloss.Color(palette.Accent), theme.Accent)
	require.Equal(t, lipgloss.Color(palette.Accent), theme.Success)
}
