package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildSystemviewConfigPayload_EmptySearchShortcutsUsesEmptyMap(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SearchShortcuts = map[string]SearchShortcut{}
	payload := BuildSystemviewConfigPayload(cfg, nil)
	require.NotNil(t, payload.SearchShortcuts)
	require.Empty(t, payload.SearchShortcuts)
}

func TestBuildSystemviewConfigPayload_FillsMissingPaletteValuesFromDefaults(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Appearance.LightPalette.Background = ""
	cfg.Appearance.DarkPalette.Accent = ""

	payload := BuildSystemviewConfigPayload(cfg, nil)
	defaults := DefaultConfig()

	require.Equal(t, defaults.Appearance.LightPalette.Background, payload.Appearance.LightPalette.Background)
	require.Equal(t, defaults.Appearance.DarkPalette.Accent, payload.Appearance.DarkPalette.Accent)
}
