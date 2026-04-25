package systemviews

import (
	"testing"

	"github.com/bnema/dumber/internal/application/dto"
	"github.com/stretchr/testify/assert"
)

func TestBuildInlineVarsRejectsUnsafeCSSValues(t *testing.T) {
	t.Parallel()

	vars := buildInlineVars(dto.WebUIAppearanceConfig{
		SansFont:      "Inter; color:red",
		SerifFont:     "Georgia",
		MonospaceFont: "JetBrains Mono",
		LightPalette: dto.ColorPalette{
			Background: "#ffffff; color:red",
			Surface:    "rgb(1 2 3 / 50%)",
			Text:       "transparent",
			Accent:     "definitelynotacolor",
			Border:     "#ABCDEF80",
		},
	}, dto.ColorPalette{
		Background: "#ffffff; color:red",
		Surface:    "rgb(1 2 3 / 50%)",
		Text:       "transparent",
		Accent:     "definitelynotacolor",
		Border:     "#ABCDEF80",
	})

	assert.NotContains(t, vars, "color:red")
	assert.NotContains(t, vars, "--sv-background")
	assert.NotContains(t, vars, "--sv-font-sans")
	assert.Contains(t, vars, "--sv-surface: rgb(1 2 3 / 50%);")
	assert.Contains(t, vars, "--sv-text: transparent;")
	assert.NotContains(t, vars, "definitelynotacolor")
	assert.Contains(t, vars, "--sv-border: #ABCDEF80;")
	assert.Contains(t, vars, "--sv-font-serif: Georgia;")
	assert.Contains(t, vars, "--sv-font-mono: JetBrains Mono;")
}

func TestSanitizeCSSFontFamilyQuotes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{name: "unquoted spaces", value: "JetBrains Mono", want: true},
		{name: "single quoted token", value: "'JetBrains Mono'", want: true},
		{name: "double quoted token", value: "\"JetBrains Mono\"", want: true},
		{name: "unquoted single quote", value: "JetBrains' Mono", want: false},
		{name: "unquoted double quote", value: "JetBrains\" Mono", want: false},
		{name: "internal quote", value: "'JetBrains' Mono'", want: false},
		{name: "breakout", value: "'JetBrains Mono'; color:red", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, got := sanitizeCSSFontFamily(tt.value)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestResolveShellTheme(t *testing.T) {
	orig := currentPrefersDarkImpl
	t.Cleanup(func() {
		currentPrefersDarkImpl = orig
	})

	appearance := dto.WebUIAppearanceConfig{
		SansFont:        "Inter",
		SerifFont:       "Georgia",
		MonospaceFont:   "JetBrains Mono",
		DefaultFontSize: 16,
		ColorScheme:     "default",
		LightPalette: dto.ColorPalette{
			Background:     "#ffffff",
			Surface:        "#f8f8f8",
			SurfaceVariant: "#eeeeee",
			Text:           "#111111",
			Muted:          "#666666",
			Accent:         "#0055ff",
			Border:         "#dddddd",
		},
		DarkPalette: dto.ColorPalette{
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
