package systemviews

import (
	"testing"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/stretchr/testify/assert"
)

func TestResolveShellTheme(t *testing.T) {
	orig := currentPrefersDarkImpl
	t.Cleanup(func() {
		currentPrefersDarkImpl = orig
	})

	appearance := port.WebUIAppearanceConfig{
		SansFont:        "Inter",
		SerifFont:       "Georgia",
		MonospaceFont:   "JetBrains Mono",
		DefaultFontSize: 16,
		ColorScheme:     "default",
		LightPalette: port.ColorPalette{
			Background:     "#ffffff",
			Surface:        "#f8f8f8",
			SurfaceVariant: "#eeeeee",
			Text:           "#111111",
			Muted:          "#666666",
			Accent:         "#0055ff",
			Border:         "#dddddd",
		},
		DarkPalette: port.ColorPalette{
			Background:     "#111111",
			Surface:        "#1a1a1a",
			SurfaceVariant: "#2a2a2a",
			Text:           "#f5f5f5",
			Muted:          "#a0a0a0",
			Accent:         "#66aaff",
			Border:         "#333333",
		},
	}

	tests := []struct {
		name               string
		colorScheme        string
		prefersDark        bool
		wantClass          string
		wantBackground     string
		wantSurfaceVariant string
	}{
		{name: "prefer-dark", colorScheme: "prefer-dark", prefersDark: false, wantClass: "sv-dark", wantBackground: "#111111", wantSurfaceVariant: "#2a2a2a"},
		{name: "prefer-light", colorScheme: "prefer-light", prefersDark: true, wantClass: "sv-light", wantBackground: "#ffffff", wantSurfaceVariant: "#eeeeee"},
		{name: "default prefers dark", colorScheme: "default", prefersDark: true, wantClass: "sv-dark", wantBackground: "#111111", wantSurfaceVariant: "#2a2a2a"},
		{name: "default prefers light", colorScheme: "default", prefersDark: false, wantClass: "sv-light", wantBackground: "#ffffff", wantSurfaceVariant: "#eeeeee"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			currentPrefersDarkImpl = func() bool { return tt.prefersDark }
			ttAppearance := appearance
			ttAppearance.ColorScheme = tt.colorScheme

			got := resolveShellTheme(ttAppearance)

			assert.Equal(t, tt.wantClass, got.RootClass)
			assert.Contains(t, got.InlineVars, "--sv-background")
			assert.Contains(t, got.InlineVars, "--sv-font-sans")
			assert.Contains(t, got.InlineVars, tt.wantBackground)
			assert.Contains(t, got.InlineVars, tt.wantSurfaceVariant)
		})
	}
}
