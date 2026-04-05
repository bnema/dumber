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
	assert.True(t, cfg.Transcoding.Enabled)
	assert.Equal(t, "auto", cfg.Transcoding.HWAccel)
	assert.Equal(t, 3, cfg.Transcoding.MaxConcurrent)
	assert.Equal(t, "medium", cfg.Transcoding.Quality)

	// Engine defaults (replaces old Performance/Privacy sections)
	assert.Equal(t, "webkit", cfg.Engine.Type)
	assert.Equal(t, ProfileDefault, cfg.Engine.Profile)
	assert.Equal(t, CookiePolicyAlways, cfg.Engine.CookiePolicy)
	assert.Equal(t, int32(defaultCEFWindowlessFrameRate), cfg.Engine.CEF.CEFWindowlessFrameRate())
	assert.Empty(t, cfg.Engine.CEF.LogFile)
	assert.False(t, cfg.Engine.CEF.EnableAudioHandler)
	assert.False(t, cfg.Engine.CEF.EnableContextMenuHandler)
	assert.False(t, cfg.Engine.CEF.TraceHandlers)
	assert.True(t, cfg.Engine.WebKit.ITPEnabled)

	// Old sections (Rendering, Privacy, Performance, Runtime) have been removed from Config.
	// Their values now live under cfg.Engine / cfg.Engine.WebKit (validated above).
}
