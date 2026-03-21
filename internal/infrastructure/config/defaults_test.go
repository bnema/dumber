package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultConfig_CoreDefaults(t *testing.T) {
	cfg := DefaultConfig()
	assert.Equal(t, "info", cfg.Logging.Level)
	assert.False(t, cfg.Logging.CaptureGTKLogs)
	assert.False(t, cfg.Media.ShowDiagnosticsOnStartup)

	// Engine defaults (replaces old Performance/Privacy sections)
	assert.Equal(t, "webkit", cfg.Engine.Type)
	assert.Equal(t, ProfileDefault, cfg.Engine.Profile)
	assert.Equal(t, CookiePolicyAlways, cfg.Engine.CookiePolicy)
	assert.Equal(t, int32(defaultCEFWindowlessFrameRate), cfg.Engine.CEF.CEFWindowlessFrameRate())
	assert.True(t, cfg.Engine.CEF.CEFMultiThreadedMessageLoop())
	assert.Equal(t, int64(defaultCEFManualPumpIntervalMs), cfg.Engine.CEF.CEFManualPumpIntervalMs())
	assert.Empty(t, cfg.Engine.CEF.LogFile)
	assert.False(t, cfg.Engine.CEF.EnableAudioHandler)
	assert.False(t, cfg.Engine.CEF.EnableContextMenuHandler)
	assert.False(t, cfg.Engine.CEF.TraceHandlers)
	assert.True(t, cfg.Engine.WebKit.ITPEnabled)

	// Old sections (Rendering, Privacy, Performance, Runtime) have been removed from Config.
	// Their values now live under cfg.Engine / cfg.Engine.WebKit (validated above).
}
