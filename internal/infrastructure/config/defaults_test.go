package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultConfig_RuntimeLoggingProfile(t *testing.T) {
	cfg := DefaultConfig()
	assert.Equal(t, "info", cfg.Logging.Level)
	assert.False(t, cfg.Logging.CaptureGTKLogs)
	assert.Equal(t, ProfileLite, cfg.Performance.Profile)
	assert.Equal(t, CookiePolicyNoThirdParty, cfg.Privacy.CookiePolicy)
	assert.True(t, cfg.Privacy.ITPEnabled)
	assert.False(t, cfg.Media.ShowDiagnosticsOnStartup)
}
