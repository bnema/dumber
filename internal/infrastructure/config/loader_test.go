package config

import (
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func TestSetEngineDefaults(t *testing.T) {
	mgr := &Manager{viper: viper.New()}
	mgr.setDefaults()

	// Universal engine fields
	assert.Equal(t, "webkit", mgr.viper.GetString("engine.type"))
	assert.Equal(t, "default", mgr.viper.GetString("engine.profile"))
	assert.Equal(t, "no_third_party", mgr.viper.GetString("engine.cookie_policy"))
	assert.Equal(t, 4, mgr.viper.GetInt("engine.pool_prewarm_count"))
	assert.Equal(t, 256, mgr.viper.GetInt("engine.zoom_cache_size"))

	// WebKit-specific engine fields
	assert.True(t, mgr.viper.GetBool("engine.webkit.itp_enabled"))
	assert.Equal(t, "auto", mgr.viper.GetString("engine.webkit.gsk_renderer"))
	assert.Equal(t, "auto", mgr.viper.GetString("engine.webkit.gl_rendering_mode"))
	assert.False(t, mgr.viper.GetBool("engine.webkit.disable_dmabuf_renderer"))

	// Old sections should NOT be registered as defaults
	assert.Empty(t, mgr.viper.GetString("privacy.cookie_policy"))
	assert.Empty(t, mgr.viper.GetString("rendering.mode"))
	assert.Empty(t, mgr.viper.GetString("performance.profile"))
	assert.Empty(t, mgr.viper.GetString("runtime.prefix"))
}

func TestNormalizeConfig_PrivacyCookiePolicy(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Privacy.CookiePolicy = CookiePolicy("INVALID")

	normalizeConfig(cfg)

	assert.Equal(t, CookiePolicyNoThirdParty, cfg.Privacy.CookiePolicy)
}
