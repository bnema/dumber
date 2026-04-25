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
	assert.False(t, cfg.Transcoding.Enabled)
	assert.Equal(t, "auto", cfg.Transcoding.HWAccel)
	assert.Equal(t, 3, cfg.Transcoding.MaxConcurrent)
	assert.Equal(t, "medium", cfg.Transcoding.Quality)

	// Engine defaults (replaces old Performance/Privacy sections)
	assert.Equal(t, "webkit", cfg.Engine.Type)
	assert.Equal(t, ProfileDefault, cfg.Engine.Profile)
	assert.Equal(t, CookiePolicyAlways, cfg.Engine.CookiePolicy)
	assert.Equal(t, int32(defaultCEFWindowlessFrameRate), cfg.Engine.CEF.CEFWindowlessFrameRate())
	assert.Empty(t, cfg.Engine.CEF.LogFile)
	assert.True(t, cfg.Engine.CEF.EnableAudioHandler)
	assert.False(t, cfg.Engine.CEF.TraceHandlers)
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
	assert.Equal(t, keys, binding.Keys)
}
