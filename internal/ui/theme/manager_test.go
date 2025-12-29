package theme

import (
	"context"
	"testing"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/application/port/mocks"
	"github.com/bnema/dumber/internal/infrastructure/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewManager_DarkMode(t *testing.T) {
	ctx := context.Background()
	cfg := &config.Config{
		Appearance: config.AppearanceConfig{
			ColorScheme: "default",
		},
	}

	mockResolver := mocks.NewMockColorSchemeResolver(t)
	mockResolver.EXPECT().Resolve().Return(port.ColorSchemePreference{
		PrefersDark: true,
		Source:      "test",
	})

	manager := NewManager(ctx, cfg, mockResolver)

	assert.True(t, manager.PrefersDark())
	assert.Equal(t, "default", manager.scheme)
}

func TestNewManager_LightMode(t *testing.T) {
	ctx := context.Background()
	cfg := &config.Config{
		Appearance: config.AppearanceConfig{
			ColorScheme: "default",
		},
	}

	mockResolver := mocks.NewMockColorSchemeResolver(t)
	mockResolver.EXPECT().Resolve().Return(port.ColorSchemePreference{
		PrefersDark: false,
		Source:      "test",
	})

	manager := NewManager(ctx, cfg, mockResolver)

	assert.False(t, manager.PrefersDark())
}

func TestNewManager_WithNilConfig(t *testing.T) {
	ctx := context.Background()

	mockResolver := mocks.NewMockColorSchemeResolver(t)
	mockResolver.EXPECT().Resolve().Return(port.ColorSchemePreference{
		PrefersDark: true,
		Source:      "fallback",
	})

	manager := NewManager(ctx, nil, mockResolver)

	require.NotNil(t, manager)
	assert.True(t, manager.PrefersDark())
	assert.Equal(t, "system", manager.scheme)
}

func TestNewManager_UsesConfigScheme(t *testing.T) {
	ctx := context.Background()
	cfg := &config.Config{
		Appearance: config.AppearanceConfig{
			ColorScheme: "prefer-dark",
		},
	}

	mockResolver := mocks.NewMockColorSchemeResolver(t)
	mockResolver.EXPECT().Resolve().Return(port.ColorSchemePreference{
		PrefersDark: true,
		Source:      "config",
	})

	manager := NewManager(ctx, cfg, mockResolver)

	assert.Equal(t, "prefer-dark", manager.scheme)
}

func TestManager_GetCurrentPalette_Dark(t *testing.T) {
	ctx := context.Background()
	cfg := &config.Config{}

	mockResolver := mocks.NewMockColorSchemeResolver(t)
	mockResolver.EXPECT().Resolve().Return(port.ColorSchemePreference{
		PrefersDark: true,
		Source:      "test",
	})

	manager := NewManager(ctx, cfg, mockResolver)
	palette := manager.GetCurrentPalette()

	// Dark palette should be returned
	assert.Equal(t, manager.GetDarkPalette(), palette)
}

func TestManager_GetCurrentPalette_Light(t *testing.T) {
	ctx := context.Background()
	cfg := &config.Config{}

	mockResolver := mocks.NewMockColorSchemeResolver(t)
	mockResolver.EXPECT().Resolve().Return(port.ColorSchemePreference{
		PrefersDark: false,
		Source:      "test",
	})

	manager := NewManager(ctx, cfg, mockResolver)
	palette := manager.GetCurrentPalette()

	// Light palette should be returned
	assert.Equal(t, manager.GetLightPalette(), palette)
}

func TestManager_SetColorScheme(t *testing.T) {
	ctx := context.Background()
	cfg := &config.Config{}

	mockResolver := mocks.NewMockColorSchemeResolver(t)
	// Initial resolve call
	mockResolver.EXPECT().Resolve().Return(port.ColorSchemePreference{
		PrefersDark: true,
		Source:      "test",
	})
	// Refresh call when SetColorScheme is called
	mockResolver.EXPECT().Refresh().Return(port.ColorSchemePreference{
		PrefersDark: false,
		Source:      "config",
	})

	manager := NewManager(ctx, cfg, mockResolver)
	assert.True(t, manager.PrefersDark())

	// Change scheme - this calls Refresh on resolver
	manager.SetColorScheme(ctx, "prefer-light", nil)

	assert.False(t, manager.PrefersDark())
	assert.Equal(t, "prefer-light", manager.scheme)
}

func TestManager_UpdateFromConfig(t *testing.T) {
	ctx := context.Background()
	cfg := &config.Config{
		Appearance: config.AppearanceConfig{
			ColorScheme: "default",
		},
	}

	mockResolver := mocks.NewMockColorSchemeResolver(t)
	// Initial resolve
	mockResolver.EXPECT().Resolve().Return(port.ColorSchemePreference{
		PrefersDark: true,
		Source:      "test",
	})
	// Refresh when UpdateFromConfig is called
	mockResolver.EXPECT().Refresh().Return(port.ColorSchemePreference{
		PrefersDark: false,
		Source:      "config",
	})

	manager := NewManager(ctx, cfg, mockResolver)
	assert.True(t, manager.PrefersDark())

	// Update with new config
	newCfg := &config.Config{
		Appearance: config.AppearanceConfig{
			ColorScheme: "prefer-light",
		},
	}
	manager.UpdateFromConfig(ctx, newCfg, nil)

	assert.False(t, manager.PrefersDark())
	assert.Equal(t, "prefer-light", manager.scheme)
}

func TestManager_UpdateFromConfig_NilConfig(t *testing.T) {
	ctx := context.Background()
	cfg := &config.Config{}

	mockResolver := mocks.NewMockColorSchemeResolver(t)
	mockResolver.EXPECT().Resolve().Return(port.ColorSchemePreference{
		PrefersDark: true,
		Source:      "test",
	})

	manager := NewManager(ctx, cfg, mockResolver)
	initialScheme := manager.scheme

	// UpdateFromConfig with nil should be a no-op
	manager.UpdateFromConfig(ctx, nil, nil)

	assert.Equal(t, initialScheme, manager.scheme)
}

func TestManager_GetWebUIThemeCSS(t *testing.T) {
	ctx := context.Background()
	cfg := &config.Config{}

	mockResolver := mocks.NewMockColorSchemeResolver(t)
	mockResolver.EXPECT().Resolve().Return(port.ColorSchemePreference{
		PrefersDark: true,
		Source:      "test",
	})

	manager := NewManager(ctx, cfg, mockResolver)
	css := manager.GetWebUIThemeCSS()

	// Should contain both light and dark variables
	assert.Contains(t, css, ":root{")
	assert.Contains(t, css, ".dark{")
}

func TestManager_GetBackgroundRGBA(t *testing.T) {
	ctx := context.Background()
	cfg := &config.Config{}

	mockResolver := mocks.NewMockColorSchemeResolver(t)
	mockResolver.EXPECT().Resolve().Return(port.ColorSchemePreference{
		PrefersDark: true,
		Source:      "test",
	})

	manager := NewManager(ctx, cfg, mockResolver)
	r, g, b, a := manager.GetBackgroundRGBA()

	// Should return valid RGBA values (0-1 range)
	assert.GreaterOrEqual(t, r, float32(0))
	assert.LessOrEqual(t, r, float32(1))
	assert.GreaterOrEqual(t, g, float32(0))
	assert.LessOrEqual(t, g, float32(1))
	assert.GreaterOrEqual(t, b, float32(0))
	assert.LessOrEqual(t, b, float32(1))
	assert.GreaterOrEqual(t, a, float32(0))
	assert.LessOrEqual(t, a, float32(1))
}

func TestManager_CustomPalettes(t *testing.T) {
	ctx := context.Background()
	cfg := &config.Config{
		Appearance: config.AppearanceConfig{
			LightPalette: config.ColorPalette{
				Background: "#ffffff",
				Text:       "#000000",
			},
			DarkPalette: config.ColorPalette{
				Background: "#000000",
				Text:       "#ffffff",
			},
		},
	}

	mockResolver := mocks.NewMockColorSchemeResolver(t)
	mockResolver.EXPECT().Resolve().Return(port.ColorSchemePreference{
		PrefersDark: true,
		Source:      "test",
	})

	manager := NewManager(ctx, cfg, mockResolver)

	lightPalette := manager.GetLightPalette()
	darkPalette := manager.GetDarkPalette()

	assert.Equal(t, "#ffffff", lightPalette.Background)
	assert.Equal(t, "#000000", lightPalette.Text)
	assert.Equal(t, "#000000", darkPalette.Background)
	assert.Equal(t, "#ffffff", darkPalette.Text)
}

func TestManager_UIScale(t *testing.T) {
	ctx := context.Background()
	cfg := &config.Config{
		DefaultUIScale: 1.5,
	}

	mockResolver := mocks.NewMockColorSchemeResolver(t)
	mockResolver.EXPECT().Resolve().Return(port.ColorSchemePreference{
		PrefersDark: true,
		Source:      "test",
	})

	manager := NewManager(ctx, cfg, mockResolver)

	// UI scale should be stored (we can't directly access it, but we can verify
	// the manager was created without error)
	require.NotNil(t, manager)
}

func TestManager_Fonts(t *testing.T) {
	ctx := context.Background()
	cfg := &config.Config{
		Appearance: config.AppearanceConfig{
			SansFont:      "Inter",
			MonospaceFont: "JetBrains Mono",
		},
	}

	mockResolver := mocks.NewMockColorSchemeResolver(t)
	mockResolver.EXPECT().Resolve().Return(port.ColorSchemePreference{
		PrefersDark: true,
		Source:      "test",
	})

	manager := NewManager(ctx, cfg, mockResolver)

	// Fonts should be stored (manager creation should succeed)
	require.NotNil(t, manager)
}

func TestManager_ModeColors(t *testing.T) {
	ctx := context.Background()
	cfg := &config.Config{
		Workspace: config.WorkspaceConfig{
			Styling: config.WorkspaceStylingConfig{
				PaneModeColor:    "#ff0000",
				TabModeColor:     "#00ff00",
				SessionModeColor: "#0000ff",
				ResizeModeColor:  "#ffff00",
			},
		},
	}

	mockResolver := mocks.NewMockColorSchemeResolver(t)
	mockResolver.EXPECT().Resolve().Return(port.ColorSchemePreference{
		PrefersDark: true,
		Source:      "test",
	})

	manager := NewManager(ctx, cfg, mockResolver)
	modeColors := manager.GetModeColors()

	assert.Equal(t, "#ff0000", modeColors.PaneMode)
	assert.Equal(t, "#00ff00", modeColors.TabMode)
	assert.Equal(t, "#0000ff", modeColors.SessionMode)
	assert.Equal(t, "#ffff00", modeColors.ResizeMode)
}
