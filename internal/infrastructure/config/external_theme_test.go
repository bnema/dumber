package config

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/bnema/dumber/internal/application/dto"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func TestDefaultConfig_ExternalThemeDefaults(t *testing.T) {
	cfg := DefaultConfig()

	require.False(t, cfg.Appearance.ExternalTheme.Enabled)
	require.Equal(t, "noctalia", cfg.Appearance.ExternalTheme.Provider)
	require.Equal(t, "colors-json", cfg.Appearance.ExternalTheme.Format)
	require.Equal(t, filepath.Join("noctalia", "colors.json"), lastPathElements(cfg.Appearance.ExternalTheme.Path, 2))
}

func TestManagerSetDefaults_ExternalThemeViperDefaults(t *testing.T) {
	mgr := &Manager{viper: viper.New()}
	mgr.setDefaults()

	require.False(t, mgr.viper.GetBool("appearance.external_theme.enabled"))
	require.Equal(t, "noctalia", mgr.viper.GetString("appearance.external_theme.provider"))
	require.Equal(t, "colors-json", mgr.viper.GetString("appearance.external_theme.format"))
	require.Equal(t, filepath.Join("noctalia", "colors.json"), lastPathElements(
		mgr.viper.GetString("appearance.external_theme.path"),
		2,
	))
}

func TestManagerLoad_ExternalThemeEnabledEmptyPathUsesDefaultColorsJSON(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("ENV", "")
	t.Setenv("XDG_CONFIG_HOME", configHome)

	configDir := filepath.Join(configHome, appName)
	require.NoError(t, os.MkdirAll(configDir, dirPerm))
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "config.toml"), []byte(`
[appearance.external_theme]
enabled = true
provider = 'noctalia'
path = ''
`), filePerm))

	mgr, err := NewManager()
	require.NoError(t, err)
	require.NoError(t, mgr.Load())

	externalTheme := mgr.Get().Appearance.ExternalTheme
	require.True(t, externalTheme.Enabled)
	require.Equal(t, "noctalia", externalTheme.Provider)
	require.Equal(t, "colors-json", externalTheme.Format)
	require.Equal(t, filepath.Join(configHome, "noctalia", "colors.json"), externalTheme.Path)
}

func TestManagerLoad_ExternalThemeDumberJSONEmptyPathUsesTemplateDefault(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("ENV", "")
	t.Setenv("XDG_CONFIG_HOME", configHome)

	configDir := filepath.Join(configHome, appName)
	require.NoError(t, os.MkdirAll(configDir, dirPerm))
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "config.toml"), []byte(`
[appearance.external_theme]
enabled = true
provider = 'noctalia'
format = 'dumber-json'
path = ''
`), filePerm))

	mgr, err := NewManager()
	require.NoError(t, err)
	require.NoError(t, mgr.Load())

	externalTheme := mgr.Get().Appearance.ExternalTheme
	require.True(t, externalTheme.Enabled)
	require.Equal(t, "noctalia", externalTheme.Provider)
	require.Equal(t, "dumber-json", externalTheme.Format)
	require.Equal(t, filepath.Join(configDir, defaultExternalThemeTemplateFilename), externalTheme.Path)
}

func TestValidateConfig_ExternalTheme(t *testing.T) {
	tests := []struct {
		name     string
		mutate   func(*Config)
		wantErr  bool
		wantText string
	}{
		{
			name: "disabled empty path validates",
			mutate: func(cfg *Config) {
				cfg.Appearance.ExternalTheme.Path = ""
			},
		},
		{
			name: "enabled colors-json with path validates",
			mutate: func(cfg *Config) {
				cfg.Appearance.ExternalTheme.Enabled = true
				cfg.Appearance.ExternalTheme.Format = "colors-json"
				cfg.Appearance.ExternalTheme.Path = "/tmp/colors.json"
			},
		},
		{
			name: "enabled dumber-json with path validates",
			mutate: func(cfg *Config) {
				cfg.Appearance.ExternalTheme.Enabled = true
				cfg.Appearance.ExternalTheme.Format = "dumber-json"
				cfg.Appearance.ExternalTheme.Path = "/tmp/theme.json"
			},
		},
		{
			name: "enabled empty path uses format-specific default",
			mutate: func(cfg *Config) {
				cfg.Appearance.ExternalTheme.Enabled = true
				cfg.Appearance.ExternalTheme.Path = ""
			},
		},
		{
			name: "enabled rejects unsupported provider",
			mutate: func(cfg *Config) {
				cfg.Appearance.ExternalTheme.Enabled = true
				cfg.Appearance.ExternalTheme.Provider = "other"
				cfg.Appearance.ExternalTheme.Path = "/tmp/theme.json"
			},
			wantErr:  true,
			wantText: "appearance.external_theme.provider",
		},
		{
			name: "enabled rejects unsupported format",
			mutate: func(cfg *Config) {
				cfg.Appearance.ExternalTheme.Enabled = true
				cfg.Appearance.ExternalTheme.Format = "toml"
				cfg.Appearance.ExternalTheme.Path = "/tmp/theme.json"
			},
			wantErr:  true,
			wantText: "appearance.external_theme.format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			tt.mutate(cfg)
			normalizeConfig(cfg)

			err := validateConfig(cfg)
			if tt.wantErr {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.wantText)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestNormalizeConfig_ExternalTheme(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Appearance.ExternalTheme.Provider = "  NOCTALIA  "
	cfg.Appearance.ExternalTheme.Format = "  COLORS-JSON  "
	cfg.Appearance.ExternalTheme.Path = "  /tmp/colors.json  "

	normalizeConfig(cfg)

	require.Equal(t, "noctalia", cfg.Appearance.ExternalTheme.Provider)
	require.Equal(t, "colors-json", cfg.Appearance.ExternalTheme.Format)
	require.Equal(t, "/tmp/colors.json", cfg.Appearance.ExternalTheme.Path)

	cfg.Appearance.ExternalTheme.Provider = ""
	cfg.Appearance.ExternalTheme.Format = ""
	normalizeConfig(cfg)
	require.Equal(t, "noctalia", cfg.Appearance.ExternalTheme.Provider)
	require.Equal(t, "colors-json", cfg.Appearance.ExternalTheme.Format)
}

func TestSchemaProvider_ExternalThemeKeys(t *testing.T) {
	keys := NewSchemaProvider().GetSchema()
	byKey := map[string]bool{}
	for _, key := range keys {
		if key.Key == "appearance.external_theme.provider" {
			require.Equal(t, SectionAppearance, key.Section)
			require.Equal(t, "noctalia", key.Default)
			require.Equal(t, []string{"noctalia"}, key.Values)
		}
		if key.Key == "appearance.external_theme.format" {
			require.Equal(t, SectionAppearance, key.Section)
			require.Equal(t, "colors-json", key.Default)
			require.Equal(t, []string{"colors-json", "dumber-json"}, key.Values)
		}
		if key.Key == "appearance.external_theme.path" {
			require.Equal(t, filepath.Join("noctalia", "colors.json"), lastPathElements(key.Default, 2))
		}
		byKey[key.Key] = true
	}

	for _, key := range []string{
		"appearance.external_theme.enabled",
		"appearance.external_theme.provider",
		"appearance.external_theme.format",
		"appearance.external_theme.path",
	} {
		require.True(t, byKey[key], "missing schema key %s", key)
	}
}

func TestWriteConfigOrdered_IncludesExternalThemeTable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	cfg := DefaultConfig()

	require.NoError(t, WriteConfigOrdered(cfg, path))
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Contains(t, string(content), "[appearance.external_theme]")
	require.Contains(t, string(content), "enabled = false")
	require.Contains(t, string(content), "provider = 'noctalia'")
	require.Contains(t, string(content), "format = 'colors-json'")
	require.Contains(t, string(content), "colors.json")
}

func TestBuildSystemviewConfigPayload_ProjectsExternalTheme(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Appearance.ExternalTheme.Enabled = true
	cfg.Appearance.ExternalTheme.Path = "/tmp/colors.json"

	payload := BuildSystemviewConfigPayload(cfg, nil)

	require.True(t, payload.Appearance.ExternalTheme.Enabled)
	require.Equal(t, "noctalia", payload.Appearance.ExternalTheme.Provider)
	require.Equal(t, "colors-json", payload.Appearance.ExternalTheme.Format)
	require.Equal(t, "/tmp/colors.json", payload.Appearance.ExternalTheme.Path)
}

func lastPathElements(path string, count int) string {
	cleaned := filepath.Clean(path)
	parts := make([]string, 0, count)
	for len(parts) < count && cleaned != "." && cleaned != string(filepath.Separator) {
		parts = append([]string{filepath.Base(cleaned)}, parts...)
		cleaned = filepath.Dir(cleaned)
	}
	return filepath.Join(parts...)
}

func TestWebUIConfigGatewaySaveWebUIConfig_ExternalThemeRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	require.NoError(t, os.WriteFile(path, []byte(""), 0o644))

	v := viper.New()
	v.SetConfigFile(path)
	require.NoError(t, v.ReadInConfig())

	previousGlobal := globalManager
	defer func() { globalManager = previousGlobal }()

	mgr := &Manager{config: DefaultConfig(), viper: v}
	globalManager = mgr

	webCfg := dto.WebUIConfig{
		Appearance: buildSystemviewAppearancePayload(DefaultConfig().Appearance),
	}
	webCfg.Appearance.ExternalTheme = dto.WebUIExternalThemeConfig{
		Enabled:  true,
		Provider: " NOCTALIA ",
		Format:   " COLORS-JSON ",
		Path:     " /tmp/colors.json ",
	}
	webCfg.DefaultUIScale = DefaultConfig().DefaultUIScale
	webCfg.DefaultSearchEngine = DefaultConfig().DefaultSearchEngine
	webCfg.SearchShortcuts = map[string]dto.SearchShortcut{}
	for key, shortcut := range DefaultConfig().SearchShortcuts {
		webCfg.SearchShortcuts[key] = dto.SearchShortcut{URL: shortcut.URL, Description: shortcut.Description}
	}

	require.NoError(t, NewWebUIConfigGateway(mgr).SaveWebUIConfig(context.Background(), webCfg))

	saved := mgr.Get()
	require.True(t, saved.Appearance.ExternalTheme.Enabled)
	require.Equal(t, "noctalia", saved.Appearance.ExternalTheme.Provider)
	require.Equal(t, "colors-json", saved.Appearance.ExternalTheme.Format)
	require.Equal(t, "/tmp/colors.json", saved.Appearance.ExternalTheme.Path)
}
