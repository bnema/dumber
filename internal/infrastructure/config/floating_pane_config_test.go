package config

import (
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFloatingPaneConfig_Defaults(t *testing.T) {
	cfg := DefaultConfig()

	assert.InDelta(t, 0.82, cfg.Workspace.FloatingPane.WidthPct, 0.0001)
	assert.InDelta(t, 0.72, cfg.Workspace.FloatingPane.HeightPct, 0.0001)
	assert.Empty(t, cfg.Workspace.FloatingPane.Profiles)

	binding, ok := cfg.Workspace.Shortcuts.Actions["toggle_floating_pane"]
	require.True(t, ok)
	assert.Equal(t, []string{"alt+f"}, binding.Keys)
}

func TestFloatingPaneConfig_DefaultsLoadThroughViper(t *testing.T) {
	mgr := &Manager{viper: viper.New()}
	mgr.setDefaults()

	cfg, err := mgr.unmarshalConfig()
	require.NoError(t, err)

	assert.InDelta(t, 0.82, cfg.Workspace.FloatingPane.WidthPct, 0.0001)
	assert.InDelta(t, 0.72, cfg.Workspace.FloatingPane.HeightPct, 0.0001)
	assert.Empty(t, cfg.Workspace.FloatingPane.Profiles)

	binding, ok := cfg.Workspace.Shortcuts.Actions["toggle_floating_pane"]
	require.True(t, ok)
	assert.Equal(t, []string{"alt+f"}, binding.Keys)
}

func TestFloatingPaneConfig_ValidationRanges(t *testing.T) {
	tests := []struct {
		name       string
		mutate     func(cfg *Config)
		errorField string
	}{
		{
			name: "width must be > 0",
			mutate: func(cfg *Config) {
				cfg.Workspace.FloatingPane.WidthPct = 0
			},
			errorField: "workspace.floating_pane.width_pct",
		},
		{
			name: "width must be <= 1",
			mutate: func(cfg *Config) {
				cfg.Workspace.FloatingPane.WidthPct = 1.5
			},
			errorField: "workspace.floating_pane.width_pct",
		},
		{
			name: "height must be > 0",
			mutate: func(cfg *Config) {
				cfg.Workspace.FloatingPane.HeightPct = 0
			},
			errorField: "workspace.floating_pane.height_pct",
		},
		{
			name: "height must be <= 1",
			mutate: func(cfg *Config) {
				cfg.Workspace.FloatingPane.HeightPct = 1.1
			},
			errorField: "workspace.floating_pane.height_pct",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			tt.mutate(cfg)

			err := validateConfig(cfg)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.errorField)
		})
	}
}

func TestFloatingPaneConfig_ProfileValidation(t *testing.T) {
	t.Run("missing URL", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.Workspace.FloatingPane.Profiles = map[string]FloatingPaneProfile{
			"google": {
				URL:  "",
				Keys: []string{"alt+g"},
			},
		}

		err := validateConfig(cfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "workspace.floating_pane.profiles.google.url")
	})

	t.Run("missing keys", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.Workspace.FloatingPane.Profiles = map[string]FloatingPaneProfile{
			"google": {
				URL:  "https://google.com",
				Keys: nil,
			},
		}

		err := validateConfig(cfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "workspace.floating_pane.profiles.google.keys")
	})

	t.Run("whitespace-only key", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.Workspace.FloatingPane.Profiles = map[string]FloatingPaneProfile{
			"google": {
				URL:  "https://google.com",
				Keys: []string{"   ", "alt+g"},
			},
		}

		err := validateConfig(cfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "empty or whitespace-only key binding")
	})

	t.Run("duplicate key binding", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.Workspace.FloatingPane.Profiles = map[string]FloatingPaneProfile{
			"google": {
				URL:  "https://google.com",
				Keys: []string{"alt+g"},
			},
			"github": {
				URL:  "https://github.com",
				Keys: []string{"alt+g"},
			},
		}

		err := validateConfig(cfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "duplicate key binding")
	})

	t.Run("invalid URL scheme", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.Workspace.FloatingPane.Profiles = map[string]FloatingPaneProfile{
			"invalid-scheme": {
				URL:  "ftp://example.com",
				Keys: []string{"alt+i"},
			},
		}

		err := validateConfig(cfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "workspace.floating_pane.profiles.invalid-scheme.url")
		assert.Contains(t, err.Error(), "must use one of: http, https, dumb, file, about")
	})
}

func TestFloatingPaneConfig_ProfileMultiplicity(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Workspace.FloatingPane.Profiles = map[string]FloatingPaneProfile{
		"google": {
			URL:  "https://google.com",
			Keys: []string{"alt+g"},
		},
		"github": {
			URL:  "https://github.com",
			Keys: []string{"alt+h"},
		},
	}

	err := validateConfig(cfg)
	require.NoError(t, err)
	assert.Len(t, cfg.Workspace.FloatingPane.Profiles, 2)
}
