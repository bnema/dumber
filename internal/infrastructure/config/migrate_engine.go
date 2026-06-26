package config

import (
	"fmt"
	"io/fs"
	"os"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// configFilePermissions is the file permission used when writing migrated config files.
const configFilePermissions fs.FileMode = 0o644

// legacyEngineAlias defines the single metadata source for old engine config keys.
// Dropped aliases are detected as legacy input but are not written to a new target.
type legacyEngineAlias struct {
	legacySection string
	legacyField   string
	targetSection string // "engine" or "engine.webkit"; empty when dropped
	targetField   string // empty when dropped
	dropped       bool
}

func (a legacyEngineAlias) legacyKey() string {
	return a.legacySection + "." + a.legacyField
}

const (
	engineTargetSection       = "engine"
	engineWebKitTargetSection = "engine.webkit"
)

func legacyEngineTarget(legacySection, legacyField, targetSection, targetField string) legacyEngineAlias {
	return legacyEngineAlias{
		legacySection: legacySection,
		legacyField:   legacyField,
		targetSection: targetSection,
		targetField:   targetField,
	}
}

func legacyEngineSameTarget(legacySection, legacyField, targetSection string) legacyEngineAlias {
	return legacyEngineTarget(legacySection, legacyField, targetSection, legacyField)
}

func droppedLegacyEngineAlias(legacySection, legacyField string) legacyEngineAlias {
	return legacyEngineAlias{legacySection: legacySection, legacyField: legacyField, dropped: true}
}

// legacyEngineAliases is the canonical list of old-to-new engine config aliases.
var legacyEngineAliases = []legacyEngineAlias{
	// [performance] -> [engine]
	legacyEngineSameTarget("performance", "profile", engineTargetSection),
	legacyEngineSameTarget("performance", "zoom_cache_size", engineTargetSection),
	legacyEngineTarget("performance", "webview_pool_prewarm_count", engineTargetSection, "pool_prewarm_count"),
	// [privacy] -> [engine]
	legacyEngineSameTarget("privacy", "cookie_policy", engineTargetSection),

	// [privacy] -> [engine.webkit]
	legacyEngineSameTarget("privacy", "itp_enabled", engineWebKitTargetSection),

	// [rendering] -> [engine.webkit] (rendering.mode is dropped)
	droppedLegacyEngineAlias("rendering", "mode"),
	legacyEngineSameTarget("rendering", "disable_dmabuf_renderer", engineWebKitTargetSection),
	legacyEngineSameTarget("rendering", "force_compositing_mode", engineWebKitTargetSection),
	legacyEngineSameTarget("rendering", "disable_compositing_mode", engineWebKitTargetSection),
	legacyEngineSameTarget("rendering", "gsk_renderer", engineWebKitTargetSection),
	legacyEngineSameTarget("rendering", "disable_mipmaps", engineWebKitTargetSection),
	legacyEngineSameTarget("rendering", "prefer_gl", engineWebKitTargetSection),
	legacyEngineSameTarget("rendering", "draw_compositing_indicators", engineWebKitTargetSection),
	legacyEngineSameTarget("rendering", "show_fps", engineWebKitTargetSection),
	legacyEngineSameTarget("rendering", "sample_memory", engineWebKitTargetSection),
	legacyEngineSameTarget("rendering", "debug_frames", engineWebKitTargetSection),

	// [performance] -> [engine.webkit] (webkit-specific) — field names are identical
	legacyEngineSameTarget("performance", "skia_cpu_painting_threads", engineWebKitTargetSection),
	legacyEngineSameTarget("performance", "skia_gpu_painting_threads", engineWebKitTargetSection),
	legacyEngineSameTarget("performance", "skia_enable_cpu_rendering", engineWebKitTargetSection),
	legacyEngineSameTarget("performance", "web_process_memory_limit_mb", engineWebKitTargetSection),
	legacyEngineSameTarget("performance", "web_process_memory_poll_interval_sec", engineWebKitTargetSection),
	legacyEngineSameTarget("performance", "web_process_memory_conservative_threshold", engineWebKitTargetSection),
	legacyEngineSameTarget("performance", "web_process_memory_strict_threshold", engineWebKitTargetSection),
	legacyEngineSameTarget("performance", "network_process_memory_limit_mb", engineWebKitTargetSection),
	legacyEngineSameTarget("performance", "network_process_memory_poll_interval_sec", engineWebKitTargetSection),
	legacyEngineSameTarget("performance", "network_process_memory_conservative_threshold", engineWebKitTargetSection),
	legacyEngineSameTarget("performance", "network_process_memory_strict_threshold", engineWebKitTargetSection),

	// [media] GStreamer fields -> [engine.webkit]
	legacyEngineSameTarget("media", "force_vsync", engineWebKitTargetSection),
	legacyEngineSameTarget("media", "gl_rendering_mode", engineWebKitTargetSection),
	legacyEngineSameTarget("media", "gstreamer_debug_level", engineWebKitTargetSection),

	// [runtime] -> [engine.webkit]
	legacyEngineSameTarget("runtime", "prefix", engineWebKitTargetSection),
}

func legacyEngineAliasesForSection(section string) []legacyEngineAlias {
	aliases := make([]legacyEngineAlias, 0)
	for _, alias := range legacyEngineAliases {
		if alias.legacySection == section {
			aliases = append(aliases, alias)
		}
	}
	return aliases
}

func legacyEngineSectionsToRemove() []string {
	sections := make([]string, 0)
	seen := make(map[string]struct{})
	for _, alias := range legacyEngineAliases {
		if alias.legacySection == "media" {
			continue
		}
		if _, ok := seen[alias.legacySection]; ok {
			continue
		}
		seen[alias.legacySection] = struct{}{}
		sections = append(sections, alias.legacySection)
	}
	return sections
}

func legacyEngineInputSections() []string {
	sections := make([]string, 0)
	seen := make(map[string]struct{})
	for _, alias := range legacyEngineAliases {
		if _, ok := seen[alias.legacySection]; ok {
			continue
		}
		seen[alias.legacySection] = struct{}{}
		sections = append(sections, alias.legacySection)
	}
	return sections
}

func legacyEngineInputSectionsMessage() string {
	sections := legacyEngineInputSections()
	formatted := make([]string, 0, len(sections))
	for _, section := range sections {
		formatted = append(formatted, "["+section+"]")
	}
	return strings.Join(formatted, ", ")
}

func hasLegacyEngineAlias(raw map[string]any, alias legacyEngineAlias) bool {
	sectionAny, exists := raw[alias.legacySection]
	if !exists {
		return false
	}
	sectionMap, ok := sectionAny.(map[string]any)
	if !ok {
		return false
	}
	_, exists = sectionMap[alias.legacyField]
	return exists
}

func canonicalLegacyEngineValue(alias legacyEngineAlias, val any) any {
	if alias.targetSection == engineTargetSection && alias.targetField == "cookie_policy" {
		if cookiePolicy, ok := val.(string); ok && cookiePolicy == "accept_all" {
			return string(CookiePolicyAlways)
		}
	}
	return val
}

// MigrateToEngineConfig restructures old top-level config sections into [engine]/[engine.webkit].
// Returns true if migration was performed, false if already migrated or no old sections found.
func MigrateToEngineConfig(configFile string) (bool, error) {
	raw, err := readRawTOML(configFile)
	if err != nil {
		return false, err
	}

	if _, exists := raw["engine"]; exists {
		for _, section := range legacyEngineSectionsToRemove() {
			if _, hasLegacy := raw[section]; hasLegacy {
				return false, fmt.Errorf(
					"mixed old/new config: [engine] coexists with legacy [%s]; "+
						"remove [rendering], [performance], [privacy], [runtime]",
					section,
				)
			}
		}
		for _, alias := range legacyEngineAliasesForSection("media") {
			if hasLegacyEngineAlias(raw, alias) {
				return false, fmt.Errorf(
					"mixed old/new config: [engine] coexists with deprecated "+
						"media.%s; remove it from [media] (now in [engine.webkit])",
					alias.legacyField,
				)
			}
		}
		return false, nil // already migrated
	}

	if !hasOldEngineSections(raw) {
		return false, nil
	}

	applyEngineMappings(raw)

	out, marshalErr := toml.Marshal(raw)
	if marshalErr != nil {
		return false, marshalErr
	}

	if err := atomicWriteFile(configFile, string(out), configFilePermissions); err != nil {
		return false, err
	}

	return true, nil
}

// readRawTOML reads a TOML file into a generic map.
func readRawTOML(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var raw map[string]any
	if err := toml.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	return raw, nil
}

// hasOldEngineSections checks if the raw config has old sections that need migration.
func hasOldEngineSections(raw map[string]any) bool {
	for _, section := range legacyEngineSectionsToRemove() {
		if _, exists := raw[section]; exists {
			return true
		}
	}
	for _, alias := range legacyEngineAliasesForSection("media") {
		if hasLegacyEngineAlias(raw, alias) {
			return true
		}
	}
	return false
}

// applyEngineMappings builds [engine]/[engine.webkit] from old sections and cleans up.
func applyEngineMappings(raw map[string]any) {
	engineMap := make(map[string]any)
	webkitMap := make(map[string]any)

	for _, alias := range legacyEngineAliases {
		if alias.dropped {
			continue
		}
		val := getSectionField(raw, alias.legacySection, alias.legacyField)
		if val == nil {
			continue
		}
		val = canonicalLegacyEngineValue(alias, val)
		switch alias.targetSection {
		case engineTargetSection:
			engineMap[alias.targetField] = val
		case engineWebKitTargetSection:
			webkitMap[alias.targetField] = val
		}
	}

	engineMap["type"] = "webkit"
	if len(webkitMap) > 0 {
		engineMap["webkit"] = webkitMap
	}
	raw["engine"] = engineMap

	for _, section := range legacyEngineSectionsToRemove() {
		delete(raw, section)
	}

	stripMigratedMediaFields(raw)
}

// stripMigratedMediaFields removes migrated fields from [media] while keeping the rest.
func stripMigratedMediaFields(raw map[string]any) {
	mediaAny, exists := raw["media"]
	if !exists {
		return
	}
	media, ok := mediaAny.(map[string]any)
	if !ok {
		return
	}
	for _, alias := range legacyEngineAliasesForSection("media") {
		delete(media, alias.legacyField)
	}
	if len(media) == 0 {
		delete(raw, "media")
	}
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
