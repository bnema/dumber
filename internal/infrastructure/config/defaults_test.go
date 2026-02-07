package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultConfig_RuntimeLoggingProfile(t *testing.T) {
	cfg := DefaultConfig()
	assert.Equal(t, "info", cfg.Logging.Level)
	assert.False(t, cfg.Logging.CaptureGTKLogs)
}
