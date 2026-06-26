package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func TestCheckLegacyFormat_OldSectionsNoEngine_ReturnsError(t *testing.T) {
	m := &Manager{viper: viper.New()}
	m.viper.Set("rendering.mode", "gpu")

	err := m.checkLegacyFormat()

	require.Error(t, err)
	require.Contains(t, err.Error(), "dumber config migrate")
}

func TestCheckLegacyFormat_OldSectionsWithEngine_ReturnsError(t *testing.T) {
	m := &Manager{viper: viper.New()}
	m.viper.Set("rendering.mode", "gpu")
	m.viper.Set("engine.type", "webkit")

	err := m.checkLegacyFormat()

	require.Error(t, err)
	require.Contains(t, err.Error(), "dumber config migrate")
}
func TestCheckLegacyFormat_MixedEngineSectionReturnsMixedConfigError(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	require.NoError(t, os.WriteFile(configPath, []byte("[engine]\nprofile = \"balanced\"\n\n[rendering]\ngsk_renderer = \"auto\"\n"), 0o600))

	m := &Manager{viper: viper.New()}
	m.viper.SetConfigFile(configPath)
	m.viper.SetConfigType("toml")
	require.NoError(t, m.viper.ReadInConfig())

	err := m.checkLegacyFormat()

	require.Error(t, err)
	require.Contains(t, err.Error(), "mixed old/new config")
	require.NotContains(t, err.Error(), "dumber config migrate")
}

func TestCheckLegacyFormat_FreshConfig_ReturnsNoError(t *testing.T) {
	m := &Manager{viper: viper.New()}

	err := m.checkLegacyFormat()

	require.NoError(t, err)
}

func TestCheckLegacyFormat_DisableDmabuf_ReturnsError(t *testing.T) {
	m := &Manager{viper: viper.New()}
	m.viper.Set("rendering.disable_dmabuf_renderer", true)

	err := m.checkLegacyFormat()

	require.Error(t, err)
	require.Contains(t, err.Error(), "dumber config migrate")
}

func TestCheckLegacyFormat_PerformanceProfile_ReturnsError(t *testing.T) {
	m := &Manager{viper: viper.New()}
	m.viper.Set("performance.profile", "lite")

	err := m.checkLegacyFormat()

	require.Error(t, err)
	require.Contains(t, err.Error(), "dumber config migrate")
}

func TestCheckLegacyFormat_PrivacyCookiePolicy_ReturnsError(t *testing.T) {
	m := &Manager{viper: viper.New()}
	m.viper.Set("privacy.cookie_policy", "always")

	err := m.checkLegacyFormat()

	require.Error(t, err)
	require.Contains(t, err.Error(), "dumber config migrate")
}

func TestCheckLegacyFormat_RuntimePrefix_ReturnsError(t *testing.T) {
	m := &Manager{viper: viper.New()}
	m.viper.Set("runtime.prefix", "/usr")

	err := m.checkLegacyFormat()

	require.Error(t, err)
	require.Contains(t, err.Error(), "dumber config migrate")
}

func TestCheckLegacyFormat_MetadataBackedAliasesReturnError(t *testing.T) {
	for _, alias := range legacyEngineAliases {
		key := alias.legacyKey()
		t.Run(key, func(t *testing.T) {
			m := &Manager{viper: viper.New()}
			m.viper.Set(key, sampleLegacyAliasValue(alias))

			err := m.checkLegacyFormat()

			require.Error(t, err)
			require.Contains(t, err.Error(), "dumber config migrate")
		})
	}
}

func sampleLegacyAliasValue(alias legacyEngineAlias) any {
	switch alias.legacyKey() {
	case "performance.profile":
		return "balanced"
	case "privacy.cookie_policy":
		return "always"
	case "rendering.mode":
		return "gpu"
	case "rendering.gsk_renderer", "media.gl_rendering_mode":
		return "auto"
	case "runtime.prefix":
		return "/usr"
	}

	switch alias.legacyField {
	case "zoom_cache_size", "webview_pool_prewarm_count",
		"skia_cpu_painting_threads", "skia_gpu_painting_threads",
		"web_process_memory_limit_mb", "network_process_memory_limit_mb",
		"gstreamer_debug_level":
		return 1
	case "web_process_memory_poll_interval_sec", "network_process_memory_poll_interval_sec",
		"web_process_memory_conservative_threshold", "web_process_memory_strict_threshold",
		"network_process_memory_conservative_threshold", "network_process_memory_strict_threshold":
		return 0.5
	default:
		return true
	}
}

func TestTransformLegacyConfig_DefaultAdaptiveDoesNotBlockLegacyCEFMigration(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	require.NoError(t, os.WriteFile(configPath, []byte("[engine.cef]\nwindowless_frame_rate = 60\n"), 0o600))

	m := &Manager{viper: viper.New()}
	m.viper.SetConfigFile(configPath)
	m.viper.SetConfigType("toml")
	m.setDefaults()
	require.NoError(t, m.viper.ReadInConfig())

	m.transformLegacyConfig()

	require.True(t, m.viper.GetBool("engine.cef.adaptive_windowless_frame_rate"))
	require.Equal(t, 0, m.viper.GetInt("engine.cef.windowless_frame_rate"))
	require.Equal(t, defaultCEFWindowlessFrameRateMax, m.viper.GetInt("engine.cef.windowless_frame_rate_max"))
}

func TestTransformLegacyConfig_ExplicitAdaptiveEnvOverrideBlocksLegacyCEFMigration(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	require.NoError(t, os.WriteFile(configPath, []byte("[engine.cef]\nwindowless_frame_rate = 60\n"), 0o600))
	t.Setenv("DUMBER_ENGINE_CEF_ADAPTIVE_WINDOWLESS_FRAME_RATE", "false")

	m := &Manager{viper: viper.New()}
	m.viper.SetConfigFile(configPath)
	m.viper.SetConfigType("toml")
	m.configureAutomaticEnv()
	m.setDefaults()
	require.NoError(t, m.viper.ReadInConfig())

	m.transformLegacyConfig()

	require.False(t, m.viper.GetBool("engine.cef.adaptive_windowless_frame_rate"))
	require.Equal(t, 60, m.viper.GetInt("engine.cef.windowless_frame_rate"))
}

func TestTransformLegacyConfig_UnconfiguredManagerIgnoresAmbientAdaptiveEnv(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	require.NoError(t, os.WriteFile(configPath, []byte("[engine.cef]\nwindowless_frame_rate = 60\n"), 0o600))
	t.Setenv("DUMBER_ENGINE_CEF_ADAPTIVE_WINDOWLESS_FRAME_RATE", "false")

	m := &Manager{viper: viper.New()}
	m.viper.SetConfigFile(configPath)
	m.viper.SetConfigType("toml")
	m.setDefaults()
	require.NoError(t, m.viper.ReadInConfig())

	m.transformLegacyConfig()

	require.True(t, m.viper.GetBool("engine.cef.adaptive_windowless_frame_rate"))
	require.Equal(t, 0, m.viper.GetInt("engine.cef.windowless_frame_rate"))
}

func TestTransformLegacyConfig_ExplicitAdaptiveEnvTrueMigratesLegacyCEFDefault(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	require.NoError(t, os.WriteFile(configPath, []byte("[engine.cef]\nwindowless_frame_rate = 60\n"), 0o600))
	t.Setenv("DUMBER_ENGINE_CEF_ADAPTIVE_WINDOWLESS_FRAME_RATE", "true")

	m := &Manager{viper: viper.New()}
	m.viper.SetConfigFile(configPath)
	m.viper.SetConfigType("toml")
	m.configureAutomaticEnv()
	m.setDefaults()
	require.NoError(t, m.viper.ReadInConfig())

	m.transformLegacyConfig()

	require.True(t, m.viper.GetBool("engine.cef.adaptive_windowless_frame_rate"))
	require.Equal(t, 0, m.viper.GetInt("engine.cef.windowless_frame_rate"))
	require.Equal(t, defaultCEFWindowlessFrameRateMax, m.viper.GetInt("engine.cef.windowless_frame_rate_max"))
}
