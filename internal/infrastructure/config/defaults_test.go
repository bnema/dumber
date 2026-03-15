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
	assert.Equal(t, CookiePolicyNoThirdParty, cfg.Engine.CookiePolicy)
	assert.True(t, cfg.Engine.WebKit.ITPEnabled)

	// Old sections should be zero-valued
	assert.Empty(t, string(cfg.Performance.Profile))
	assert.Empty(t, string(cfg.Privacy.CookiePolicy))
	assert.Empty(t, string(cfg.Rendering.Mode))
}
