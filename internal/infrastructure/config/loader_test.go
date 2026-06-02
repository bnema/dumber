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
	assert.Equal(t, EngineTypeCEF, mgr.viper.GetString("engine.type"))
	assert.Equal(t, "default", mgr.viper.GetString("engine.profile"))
	assert.Equal(t, "always", mgr.viper.GetString("engine.cookie_policy"))
	assert.Equal(t, 4, mgr.viper.GetInt("engine.pool_prewarm_count"))
	assert.Equal(t, 256, mgr.viper.GetInt("engine.zoom_cache_size"))
	assert.InDelta(t, defaultCEFScrollMultiplier, mgr.viper.GetFloat64("engine.cef.input.scroll_wheel_multiplier"), 0.001)
	assert.InDelta(t, defaultCEFScrollTouchpadMultiplier, mgr.viper.GetFloat64("engine.cef.input.scroll_touchpad_multiplier"), 0.001)
	assert.InDelta(t, defaultCEFScrollMultiplier, mgr.viper.GetFloat64("engine.cef.input.scroll_horizontal_multiplier"), 0.001)
	assert.InDelta(t, defaultCEFScrollMultiplier, mgr.viper.GetFloat64("engine.cef.input.scroll_vertical_multiplier"), 0.001)
	assert.Equal(t, defaultCEFScrollMaxDelta, mgr.viper.GetInt("engine.cef.input.scroll_max_delta"))
	assert.Equal(t, defaultCEFTouchpadNavigation, mgr.viper.GetBool("engine.cef.input.touchpad_navigation_enabled"))
	assert.InDelta(t, defaultCEFTouchpadNavigationDelta, mgr.viper.GetFloat64("engine.cef.input.touchpad_navigation_min_delta"), 0.001)
	assert.InDelta(t, defaultCEFTouchpadNavigationRatio, mgr.viper.GetFloat64("engine.cef.input.touchpad_navigation_max_vertical_ratio"), 0.001)

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

func TestNormalizeConfig_EngineCookiePolicy(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Engine.CookiePolicy = CookiePolicy("INVALID")

	normalizeConfig(cfg)

	assert.Equal(t, CookiePolicyAlways, cfg.Engine.CookiePolicy)
}
