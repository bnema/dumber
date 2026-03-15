package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/pelletier/go-toml/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const oldFormatConfig = `
[performance]
profile = "balanced"
zoom_cache_size = 50
webview_pool_prewarm_count = 2
skia_cpu_painting_threads = 4
skia_gpu_painting_threads = 2
skia_enable_cpu_rendering = false
web_process_memory_limit_mb = 512
web_process_memory_poll_interval_sec = 5.0
web_process_memory_conservative_threshold = 0.7
web_process_memory_strict_threshold = 0.9
network_process_memory_limit_mb = 256
network_process_memory_poll_interval_sec = 10.0
network_process_memory_conservative_threshold = 0.6
network_process_memory_strict_threshold = 0.85

[privacy]
cookie_policy = "accept_all"
itp_enabled = true

[rendering]
mode = "auto"
disable_dmabuf_renderer = false
force_compositing_mode = true
disable_compositing_mode = false
gsk_renderer = "gl"
disable_mipmaps = false
prefer_gl = true
draw_compositing_indicators = false
show_fps = false
sample_memory = false
debug_frames = false

[media]
hardware_decoding = true
prefer_av1 = false
show_diagnostics = false
force_vsync = true
gl_rendering_mode = "auto"
gstreamer_debug_level = 2

[runtime]
prefix = "/usr/local"
`

func TestMigrateToEngineConfig_OldFormat(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.toml")

	err := os.WriteFile(configFile, []byte(oldFormatConfig), 0o644)
	require.NoError(t, err)

	migrated, err := MigrateToEngineConfig(configFile)
	require.NoError(t, err)
	assert.True(t, migrated, "should report migration performed")

	// Read the migrated config
	data, err := os.ReadFile(configFile)
	require.NoError(t, err)

	var raw map[string]any
	err = toml.Unmarshal(data, &raw)
	require.NoError(t, err)

	// [engine] section must exist
	engineAny, ok := raw["engine"]
	require.True(t, ok, "engine section must exist")
	engine, ok := engineAny.(map[string]any)
	require.True(t, ok, "engine must be a map")

	// engine.type defaults to "webkit"
	assert.Equal(t, "webkit", engine["type"])

	// [performance] -> [engine] universal fields
	assert.Equal(t, "balanced", engine["profile"])
	assert.Equal(t, int64(50), engine["zoom_cache_size"])
	assert.Equal(t, int64(2), engine["pool_prewarm_count"])

	// [privacy] -> [engine] universal field
	assert.Equal(t, "accept_all", engine["cookie_policy"])

	// [engine.webkit] section
	webkitAny, ok := engine["webkit"]
	require.True(t, ok, "engine.webkit section must exist")
	webkit, ok := webkitAny.(map[string]any)
	require.True(t, ok, "engine.webkit must be a map")

	// [privacy] -> [engine.webkit]
	assert.Equal(t, true, webkit["itp_enabled"])

	// [rendering] -> [engine.webkit] (excluding mode which is dropped)
	assert.Equal(t, false, webkit["disable_dmabuf_renderer"])
	assert.Equal(t, true, webkit["force_compositing_mode"])
	assert.Equal(t, false, webkit["disable_compositing_mode"])
	assert.Equal(t, "gl", webkit["gsk_renderer"])
	assert.Equal(t, false, webkit["disable_mipmaps"])
	assert.Equal(t, true, webkit["prefer_gl"])
	assert.Equal(t, false, webkit["draw_compositing_indicators"])
	assert.Equal(t, false, webkit["show_fps"])
	assert.Equal(t, false, webkit["sample_memory"])
	assert.Equal(t, false, webkit["debug_frames"])
	// rendering.mode must be dropped (not present in webkit)
	assert.NotContains(t, webkit, "mode")

	// [performance] -> [engine.webkit] (webkit-specific)
	assert.Equal(t, int64(4), webkit["skia_cpu_painting_threads"])
	assert.Equal(t, int64(2), webkit["skia_gpu_painting_threads"])
	assert.Equal(t, false, webkit["skia_enable_cpu_rendering"])
	assert.Equal(t, int64(512), webkit["web_process_memory_limit_mb"])
	assert.InDelta(t, 5.0, webkit["web_process_memory_poll_interval_sec"], 0.001)
	assert.InDelta(t, 0.7, webkit["web_process_memory_conservative_threshold"], 0.001)
	assert.InDelta(t, 0.9, webkit["web_process_memory_strict_threshold"], 0.001)
	assert.Equal(t, int64(256), webkit["network_process_memory_limit_mb"])
	assert.InDelta(t, 10.0, webkit["network_process_memory_poll_interval_sec"], 0.001)
	assert.InDelta(t, 0.6, webkit["network_process_memory_conservative_threshold"], 0.001)
	assert.InDelta(t, 0.85, webkit["network_process_memory_strict_threshold"], 0.001)

	// [media] GStreamer fields -> [engine.webkit]
	assert.Equal(t, true, webkit["force_vsync"])
	assert.Equal(t, "auto", webkit["gl_rendering_mode"])
	assert.Equal(t, int64(2), webkit["gstreamer_debug_level"])

	// [runtime] -> [engine.webkit]
	assert.Equal(t, "/usr/local", webkit["prefix"])

	// Old top-level sections must be removed
	assert.NotContains(t, raw, "performance")
	assert.NotContains(t, raw, "rendering")
	assert.NotContains(t, raw, "runtime")

	// [privacy] should be removed (all fields migrated)
	assert.NotContains(t, raw, "privacy")
}

func TestMigrateToEngineConfig_AlreadyMigrated(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.toml")

	alreadyMigrated := `
[engine]
type = "webkit"
profile = "balanced"

[engine.webkit]
itp_enabled = true
`
	err := os.WriteFile(configFile, []byte(alreadyMigrated), 0o644)
	require.NoError(t, err)

	migrated, err := MigrateToEngineConfig(configFile)
	require.NoError(t, err)
	assert.False(t, migrated, "should not migrate when already migrated")

	// Content should be unchanged
	data, err := os.ReadFile(configFile)
	require.NoError(t, err)
	assert.Equal(t, alreadyMigrated, string(data))
}

func TestMigrateToEngineConfig_NoOldSections(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.toml")

	noOldSections := `
[history]
max_entries = 1000

[logging]
level = "info"
`
	err := os.WriteFile(configFile, []byte(noOldSections), 0o644)
	require.NoError(t, err)

	migrated, err := MigrateToEngineConfig(configFile)
	require.NoError(t, err)
	assert.False(t, migrated, "should not migrate when no old sections exist")

	// Content should be unchanged
	data, err := os.ReadFile(configFile)
	require.NoError(t, err)
	assert.Equal(t, noOldSections, string(data))
}

func TestMigrateToEngineConfig_PreservesNonMigratedMediaFields(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.toml")

	content := `
[media]
hardware_decoding = true
prefer_av1 = false
show_diagnostics = true
force_vsync = true
gl_rendering_mode = "auto"
gstreamer_debug_level = 3

[rendering]
disable_dmabuf_renderer = false
`
	err := os.WriteFile(configFile, []byte(content), 0o644)
	require.NoError(t, err)

	migrated, err := MigrateToEngineConfig(configFile)
	require.NoError(t, err)
	assert.True(t, migrated)

	data, err := os.ReadFile(configFile)
	require.NoError(t, err)

	var raw map[string]any
	err = toml.Unmarshal(data, &raw)
	require.NoError(t, err)

	// media section must still exist with non-migrated fields
	mediaAny, ok := raw["media"]
	require.True(t, ok, "media section must still exist")
	media, ok := mediaAny.(map[string]any)
	require.True(t, ok)

	assert.Equal(t, true, media["hardware_decoding"])
	assert.Equal(t, false, media["prefer_av1"])
	assert.Equal(t, true, media["show_diagnostics"])

	// Migrated media fields should be removed from media
	assert.NotContains(t, media, "force_vsync")
	assert.NotContains(t, media, "gl_rendering_mode")
	assert.NotContains(t, media, "gstreamer_debug_level")

	// Migrated fields should be in engine.webkit
	engineAny, ok := raw["engine"]
	require.True(t, ok, "engine section must exist")
	engine, ok := engineAny.(map[string]any)
	require.True(t, ok, "engine section must be a map")
	webkitAny, ok := engine["webkit"]
	require.True(t, ok, "engine.webkit section must exist")
	webkit, ok := webkitAny.(map[string]any)
	require.True(t, ok, "engine.webkit section must be a map")
	assert.Equal(t, true, webkit["force_vsync"])
	assert.Equal(t, "auto", webkit["gl_rendering_mode"])
	assert.Equal(t, int64(3), webkit["gstreamer_debug_level"])
}

func TestMigrateToEngineConfig_EngineTypeDefault(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.toml")

	// Minimal old-format config to trigger migration
	content := `
[rendering]
show_fps = true
`
	err := os.WriteFile(configFile, []byte(content), 0o644)
	require.NoError(t, err)

	migrated, err := MigrateToEngineConfig(configFile)
	require.NoError(t, err)
	assert.True(t, migrated)

	data, err := os.ReadFile(configFile)
	require.NoError(t, err)

	var raw map[string]any
	err = toml.Unmarshal(data, &raw)
	require.NoError(t, err)

	engineAny, ok := raw["engine"]
	require.True(t, ok, "raw[\"engine\"] should exist")
	engine, ok := engineAny.(map[string]any)
	require.True(t, ok, "raw[\"engine\"] should be map[string]any, got %T", engineAny)
	assert.Equal(t, "webkit", engine["type"], "engine.type should default to webkit")
}
