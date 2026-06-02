package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultConfig_CoreDefaults(t *testing.T) {
	cfg := DefaultConfig()
	assert.Equal(t, "info", cfg.Logging.Level)
	assert.Equal(t, defaultMaxLogFiles, cfg.Logging.MaxFiles)
	assert.False(t, cfg.Logging.CaptureGTKLogs)
	assert.False(t, cfg.Media.ShowDiagnosticsOnStartup)

	// Engine defaults (replaces old Performance/Privacy sections)
	assert.Equal(t, EngineTypeCEF, cfg.Engine.Type)
	assert.Equal(t, ProfileDefault, cfg.Engine.Profile)
	assert.Equal(t, CookiePolicyAlways, cfg.Engine.CookiePolicy)
	assert.Equal(t, CEFRenderStackVulkan, cfg.Engine.CEF.CEFRenderStack())
	assert.True(t, cfg.Engine.CEF.CEFAdaptiveWindowlessFrameRate())
	assert.Equal(t, int32(defaultCEFWindowlessFrameRate), cfg.Engine.CEF.CEFWindowlessFrameRate())
	assert.Equal(t, int32(defaultCEFWindowlessFrameRateMax), cfg.Engine.CEF.CEFWindowlessFrameRateMax())
	assert.Empty(t, cfg.Engine.CEF.LogFile)
	assert.True(t, cfg.Engine.CEF.EnableAudioHandler)
	assert.False(t, cfg.Engine.CEF.TraceHandlers)
	assert.InDelta(t, defaultCEFScrollMultiplier, cfg.Engine.CEF.Input.ScrollWheelMultiplier, 0.001)
	assert.InDelta(t, defaultCEFScrollTouchpadMultiplier, cfg.Engine.CEF.Input.ScrollTouchpadMultiplier, 0.001)
	assert.InDelta(t, defaultCEFScrollMultiplier, cfg.Engine.CEF.Input.ScrollHorizontalMultiplier, 0.001)
	assert.InDelta(t, defaultCEFScrollMultiplier, cfg.Engine.CEF.Input.ScrollVerticalMultiplier, 0.001)
	assert.Equal(t, int32(defaultCEFScrollMaxDelta), cfg.Engine.CEF.Input.ScrollMaxDelta)
	assert.True(t, cfg.Engine.WebKit.ITPEnabled)

	// System view shortcuts are first-class global actions.
	requireActionBinding(t, cfg.Workspace.Shortcuts.Actions, "toggle-history-systemview", []string{"ctrl+h"})
	requireActionBinding(t, cfg.Workspace.Shortcuts.Actions, "toggle-favorites-systemview", []string{})
	requireActionBinding(t, cfg.Workspace.Shortcuts.Actions, "toggle-config-systemview", []string{})

	// Old sections (Rendering, Privacy, Performance, Runtime) have been removed from Config.
	// Their values now live under cfg.Engine / cfg.Engine.WebKit (validated above).
}

func requireActionBinding(t *testing.T, actions map[string]ActionBinding, action string, keys []string) {
	t.Helper()
	binding, ok := actions[action]
	require.True(t, ok, "missing action binding %q", action)
	if len(keys) == 0 {
		assert.Empty(t, binding.Keys)
		return
	}
	assert.Equal(t, keys, binding.Keys)
}
