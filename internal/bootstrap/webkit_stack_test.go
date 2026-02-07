package bootstrap

import (
	"testing"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/infrastructure/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildWebKitContextOptions_MapsPrivacyAndMemory(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Privacy.CookiePolicy = config.CookiePolicyNever
	cfg.Privacy.ITPEnabled = false

	perf := &config.ResolvedPerformanceSettings{
		WebProcessMemoryLimitMB:               1024,
		WebProcessMemoryPollIntervalSec:       11,
		WebProcessMemoryConservativeThreshold: 0.25,
		WebProcessMemoryStrictThreshold:       0.5,
		NetworkProcessMemoryLimitMB:           512,
		NetworkProcessMemoryPollIntervalSec:   13,
	}

	opts := buildWebKitContextOptions(cfg, "/tmp/data", "/tmp/cache", perf)

	assert.Equal(t, "/tmp/data", opts.DataDir)
	assert.Equal(t, "/tmp/cache", opts.CacheDir)
	assert.Equal(t, port.WebKitCookiePolicyNever, opts.CookiePolicy)
	assert.False(t, opts.ITPEnabled)

	require.NotNil(t, opts.WebProcessMemory)
	assert.Equal(t, 1024, opts.WebProcessMemory.MemoryLimitMB)
	assert.InDelta(t, 11.0, opts.WebProcessMemory.PollIntervalSec, 1e-9)
	assert.InDelta(t, 0.25, opts.WebProcessMemory.ConservativeThreshold, 1e-9)
	assert.InDelta(t, 0.5, opts.WebProcessMemory.StrictThreshold, 1e-9)

	require.NotNil(t, opts.NetworkProcessMemory)
	assert.Equal(t, 512, opts.NetworkProcessMemory.MemoryLimitMB)
	assert.InDelta(t, 13.0, opts.NetworkProcessMemory.PollIntervalSec, 1e-9)
}

func TestBuildWebKitContextOptions_DefaultPrivacyWhenConfigNil(t *testing.T) {
	opts := buildWebKitContextOptions(nil, "/tmp/data", "/tmp/cache", &config.ResolvedPerformanceSettings{})

	assert.Equal(t, port.WebKitCookiePolicyNoThirdParty, opts.CookiePolicy)
	assert.True(t, opts.ITPEnabled)
}
