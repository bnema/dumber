package config

import (
	"os"

	"github.com/pelletier/go-toml/v2"
)

// engineMigrationMapping defines old key -> new key mappings for the engine migration.
// Keys with empty new key ("") are dropped (not migrated).
// The source section is the first part of the old key.
type engineFieldMapping struct {
	oldSection string
	oldField   string
	newSection string // "engine" or "engine.webkit"
	newField   string // empty means drop
}

// engineUniversalMappings are fields that go into [engine] (not [engine.webkit]).
var engineUniversalMappings = []engineFieldMapping{
	// [performance] -> [engine]
	{oldSection: "performance", oldField: "profile", newSection: "engine", newField: "profile"},
	{oldSection: "performance", oldField: "zoom_cache_size", newSection: "engine", newField: "zoom_cache_size"},
	{oldSection: "performance", oldField: "webview_pool_prewarm_count", newSection: "engine", newField: "pool_prewarm_count"},
	// [privacy] -> [engine]
	{oldSection: "privacy", oldField: "cookie_policy", newSection: "engine", newField: "cookie_policy"},
}

// engineWebkitMappings are fields that go into [engine.webkit].
var engineWebkitMappings = []engineFieldMapping{
	// [privacy] -> [engine.webkit]
	{oldSection: "privacy", oldField: "itp_enabled", newSection: "engine.webkit", newField: "itp_enabled"},

	// [rendering] -> [engine.webkit] (rendering.mode is dropped)
	{oldSection: "rendering", oldField: "disable_dmabuf_renderer", newSection: "engine.webkit", newField: "disable_dmabuf_renderer"},
	{oldSection: "rendering", oldField: "force_compositing_mode", newSection: "engine.webkit", newField: "force_compositing_mode"},
	{oldSection: "rendering", oldField: "disable_compositing_mode", newSection: "engine.webkit", newField: "disable_compositing_mode"},
	{oldSection: "rendering", oldField: "gsk_renderer", newSection: "engine.webkit", newField: "gsk_renderer"},
	{oldSection: "rendering", oldField: "disable_mipmaps", newSection: "engine.webkit", newField: "disable_mipmaps"},
	{oldSection: "rendering", oldField: "prefer_gl", newSection: "engine.webkit", newField: "prefer_gl"},
	{oldSection: "rendering", oldField: "draw_compositing_indicators", newSection: "engine.webkit", newField: "draw_compositing_indicators"},
	{oldSection: "rendering", oldField: "show_fps", newSection: "engine.webkit", newField: "show_fps"},
	{oldSection: "rendering", oldField: "sample_memory", newSection: "engine.webkit", newField: "sample_memory"},
	{oldSection: "rendering", oldField: "debug_frames", newSection: "engine.webkit", newField: "debug_frames"},

	// [performance] -> [engine.webkit] (webkit-specific)
	{oldSection: "performance", oldField: "skia_cpu_painting_threads", newSection: "engine.webkit", newField: "skia_cpu_painting_threads"},
	{oldSection: "performance", oldField: "skia_gpu_painting_threads", newSection: "engine.webkit", newField: "skia_gpu_painting_threads"},
	{oldSection: "performance", oldField: "skia_enable_cpu_rendering", newSection: "engine.webkit", newField: "skia_enable_cpu_rendering"},
	{oldSection: "performance", oldField: "web_process_memory_limit_mb", newSection: "engine.webkit", newField: "web_process_memory_limit_mb"},
	{oldSection: "performance", oldField: "web_process_memory_poll_interval_sec", newSection: "engine.webkit", newField: "web_process_memory_poll_interval_sec"},
	{oldSection: "performance", oldField: "web_process_memory_conservative_threshold", newSection: "engine.webkit", newField: "web_process_memory_conservative_threshold"},
	{oldSection: "performance", oldField: "web_process_memory_strict_threshold", newSection: "engine.webkit", newField: "web_process_memory_strict_threshold"},
	{oldSection: "performance", oldField: "network_process_memory_limit_mb", newSection: "engine.webkit", newField: "network_process_memory_limit_mb"},
	{oldSection: "performance", oldField: "network_process_memory_poll_interval_sec", newSection: "engine.webkit", newField: "network_process_memory_poll_interval_sec"},
	{oldSection: "performance", oldField: "network_process_memory_conservative_threshold", newSection: "engine.webkit", newField: "network_process_memory_conservative_threshold"},
	{oldSection: "performance", oldField: "network_process_memory_strict_threshold", newSection: "engine.webkit", newField: "network_process_memory_strict_threshold"},

	// [media] GStreamer fields -> [engine.webkit]
	{oldSection: "media", oldField: "force_vsync", newSection: "engine.webkit", newField: "force_vsync"},
	{oldSection: "media", oldField: "gl_rendering_mode", newSection: "engine.webkit", newField: "gl_rendering_mode"},
	{oldSection: "media", oldField: "gstreamer_debug_level", newSection: "engine.webkit", newField: "gstreamer_debug_level"},

	// [runtime] -> [engine.webkit]
	{oldSection: "runtime", oldField: "prefix", newSection: "engine.webkit", newField: "prefix"},
}

// oldSectionsToRemove are the sections that are fully removed after migration
// (media is not here because it has fields that stay).
var oldSectionsToRemove = []string{"rendering", "performance", "privacy", "runtime"}

// MigrateToEngineConfig restructures old top-level config sections into [engine]/[engine.webkit].
// Returns true if migration was performed, false if already migrated or no old sections found.
func MigrateToEngineConfig(configFile string) (bool, error) {
	data, err := os.ReadFile(configFile)
	if err != nil {
		return false, err
	}

	var raw map[string]any
	if err := toml.Unmarshal(data, &raw); err != nil {
		return false, err
	}

	// If "engine" key already exists, already migrated.
	if _, exists := raw["engine"]; exists {
		return false, nil
	}

	// Check if any old sections exist.
	hasOldSections := false
	for _, section := range []string{"rendering", "performance", "privacy", "runtime"} {
		if _, exists := raw[section]; exists {
			hasOldSections = true
			break
		}
	}
	// Also check media for GStreamer fields that migrate.
	if !hasOldSections {
		if mediaAny, exists := raw["media"]; exists {
			if media, ok := mediaAny.(map[string]any); ok {
				for _, f := range []string{"force_vsync", "gl_rendering_mode", "gstreamer_debug_level"} {
					if _, has := media[f]; has {
						hasOldSections = true
						break
					}
				}
			}
		}
	}

	if !hasOldSections {
		return false, nil
	}

	// Build engine and engine.webkit maps.
	engineMap := make(map[string]any)
	webkitMap := make(map[string]any)

	// Apply universal mappings -> engine.
	for _, m := range engineUniversalMappings {
		if val := getSectionField(raw, m.oldSection, m.oldField); val != nil {
			engineMap[m.newField] = val
		}
	}

	// Apply webkit mappings -> engine.webkit.
	for _, m := range engineWebkitMappings {
		if val := getSectionField(raw, m.oldSection, m.oldField); val != nil {
			webkitMap[m.newField] = val
		}
	}

	// Set engine.type default.
	engineMap["type"] = "webkit"

	// Attach webkit sub-map.
	if len(webkitMap) > 0 {
		engineMap["webkit"] = webkitMap
	}

	// Set engine in raw map.
	raw["engine"] = engineMap

	// Remove old sections entirely.
	for _, section := range oldSectionsToRemove {
		delete(raw, section)
	}

	// Remove migrated fields from media (but keep non-migrated ones).
	if mediaAny, exists := raw["media"]; exists {
		if media, ok := mediaAny.(map[string]any); ok {
			delete(media, "force_vsync")
			delete(media, "gl_rendering_mode")
			delete(media, "gstreamer_debug_level")
			if len(media) == 0 {
				delete(raw, "media")
			} else {
				raw["media"] = media
			}
		}
	}

	// Write back.
	out, err := toml.Marshal(raw)
	if err != nil {
		return false, err
	}

	if err := os.WriteFile(configFile, out, 0o644); err != nil {
		return false, err
	}

	return true, nil
}

// getSectionField retrieves a field value from a named section in the raw config map.
func getSectionField(raw map[string]any, section, field string) any {
	sectionAny, exists := raw[section]
	if !exists {
		return nil
	}
	sectionMap, ok := sectionAny.(map[string]any)
	if !ok {
		return nil
	}
	val, exists := sectionMap[field]
	if !exists {
		return nil
	}
	return val
}
