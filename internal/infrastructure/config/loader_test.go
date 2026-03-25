package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetEngineDefaults(t *testing.T) {
	mgr := &Manager{viper: viper.New()}
	mgr.setDefaults()

	// Universal engine fields
	assert.Equal(t, "webkit", mgr.viper.GetString("engine.type"))
	assert.Equal(t, "default", mgr.viper.GetString("engine.profile"))
	assert.Equal(t, "always", mgr.viper.GetString("engine.cookie_policy"))
	assert.Equal(t, 4, mgr.viper.GetInt("engine.pool_prewarm_count"))
	assert.Equal(t, 256, mgr.viper.GetInt("engine.zoom_cache_size"))

	// WebKit-specific engine fields
	assert.True(t, mgr.viper.GetBool("engine.webkit.itp_enabled"))
	assert.Equal(t, "auto", mgr.viper.GetString("engine.webkit.gsk_renderer"))
	assert.Equal(t, "auto", mgr.viper.GetString("engine.webkit.gl_rendering_mode"))
	assert.False(t, mgr.viper.GetBool("engine.webkit.disable_dmabuf_renderer"))

	// Transcoding defaults
	assert.True(t, mgr.viper.GetBool("transcoding.enabled"))
	assert.Equal(t, "auto", mgr.viper.GetString("transcoding.hwaccel"))
	assert.Equal(t, 3, mgr.viper.GetInt("transcoding.max_concurrent"))
	assert.Equal(t, "medium", mgr.viper.GetString("transcoding.quality"))

	// Old sections should NOT be registered as defaults
	assert.Empty(t, mgr.viper.GetString("privacy.cookie_policy"))
	assert.Empty(t, mgr.viper.GetString("rendering.mode"))
	assert.Empty(t, mgr.viper.GetString("performance.profile"))
	assert.Empty(t, mgr.viper.GetString("runtime.prefix"))
}

func TestNormalizeConfig_EngineCookiePolicy(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Engine.CookiePolicy = CookiePolicy("INVALID")

	normalizeConfig(cfg)

	assert.Equal(t, CookiePolicyAlways, cfg.Engine.CookiePolicy)
}

func TestLoad_AppliesTranscodingDefaultsWhenSectionMissing(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	configDir, err := GetConfigDir()
	require.NoError(t, err)

	err = os.MkdirAll(configDir, dirPerm)
	require.NoError(t, err)

	configPath := filepath.Join(configDir, "config.toml")
	err = os.WriteFile(configPath, []byte(`
[engine]
type = "cef"
`), filePerm)
	require.NoError(t, err)

	mgr, err := NewManager()
	require.NoError(t, err)
	require.NoError(t, mgr.Load())

	cfg := mgr.Get()
	require.NotNil(t, cfg)
	assert.True(t, cfg.Transcoding.Enabled)
	assert.Equal(t, "auto", cfg.Transcoding.HWAccel)
	assert.Equal(t, 3, cfg.Transcoding.MaxConcurrent)
	assert.Equal(t, "medium", cfg.Transcoding.Quality)
}
