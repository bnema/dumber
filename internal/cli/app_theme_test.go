package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/bnema/dumber/internal/infrastructure/config"
)

func TestResolveCLIThemePreservesDarkDefault(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Appearance.ColorScheme = "default"

	resolved := resolveCLITheme(cfg)

	require.True(t, resolved.PrefersDark)
	require.Equal(t, cfg.Appearance.DarkPalette.Background, resolved.ActivePalette.Background)
}

func TestResolveCLIThemeHonorsExplicitLightPreference(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Appearance.ColorScheme = "prefer-light"

	resolved := resolveCLITheme(cfg)

	require.False(t, resolved.PrefersDark)
	require.Equal(t, cfg.Appearance.LightPalette.Background, resolved.ActivePalette.Background)
}

func TestResolveCLIThemeUsesEnabledExternalTheme(t *testing.T) {
	path := filepath.Join(t.TempDir(), "theme.json")
	require.NoError(t, os.WriteFile(path, []byte(`{
		"light":{"background":"#ffffff","accent":"#112233"},
		"dark":{"background":"#000000","accent":"#445566"}
	}`), 0o600))
	cfg := config.DefaultConfig()
	cfg.Appearance.ExternalTheme.Enabled = true
	cfg.Appearance.ExternalTheme.Provider = "noctalia"
	cfg.Appearance.ExternalTheme.Format = "dumber-json"
	cfg.Appearance.ExternalTheme.Path = path

	resolved := resolveCLITheme(cfg)

	require.True(t, resolved.PrefersDark)
	require.Equal(t, "#000000", resolved.ActivePalette.Background)
	require.Equal(t, "#445566", resolved.ActivePalette.Accent)
	require.Equal(t, "external", string(resolved.ThemeSource.Kind))
	require.Equal(t, "noctalia", resolved.ThemeSource.Provider)
}
