package config

import "math"

const (
	sectionWorkspace = "workspace"
	sectionSession   = "session"
)

// LegacyConfigTransformer implements port.ConfigTransformer.
// It transforms old-format action bindings (slices) to new ActionBinding format.
type LegacyConfigTransformer struct {
	defaults *Config
}

// NewLegacyConfigTransformer creates a new transformer with defaults for description lookup.
func NewLegacyConfigTransformer() *LegacyConfigTransformer {
	return &LegacyConfigTransformer{defaults: DefaultConfig()}
}

// TransformLegacyActions converts old-format action bindings to new format.
// This handles migration from:
//
//	[workspace.pane_mode.actions]
//	"focus-left" = ["h", "arrowleft"]
//
// To:
//
//	[workspace.pane_mode.actions.focus-left]
//	keys = ["h", "arrowleft"]
//	desc = "Focus pane to the left"
func (t *LegacyConfigTransformer) TransformLegacyActions(rawConfig map[string]any) {
	actionPaths := [][]string{
		{"workspace", "pane_mode", "actions"},
		{"workspace", "tab_mode", "actions"},
		{"workspace", "resize_mode", "actions"},
		{"workspace", "shortcuts", "actions"},
		{"session", "session_mode", "actions"},
	}

	for _, path := range actionPaths {
		t.transformActionSection(rawConfig, path)
	}
}

// TransformLegacyEngineConfig removes deprecated engine config keys.
func (t *LegacyConfigTransformer) TransformLegacyEngineConfig(rawConfig map[string]any) {
	adaptiveEnabled, hasAdaptive := nestedConfigBoolValue(rawConfig, "engine", "cef", "adaptive_windowless_frame_rate")
	t.transformLegacyEngineConfig(rawConfig, hasAdaptive, adaptiveEnabled)
}

func (t *LegacyConfigTransformer) TransformLegacyEngineConfigWithExplicitAdaptive(
	rawConfig map[string]any,
	hasAdaptive, adaptiveEnabled bool,
) {
	t.transformLegacyEngineConfig(rawConfig, hasAdaptive, adaptiveEnabled)
}

func (*LegacyConfigTransformer) transformLegacyEngineConfig(rawConfig map[string]any, hasAdaptive, adaptiveEnabled bool) {
	engine, ok := rawConfig["engine"].(map[string]any)
	if !ok {
		return
	}

	cef, ok := engine["cef"].(map[string]any)
	if !ok {
		return
	}

	delete(cef, "enable_context_menu_handler")
	migrateLegacyCEFWindowlessFrameRateDefault(cef, hasAdaptive, adaptiveEnabled)
	migrateLegacyCEFInputScrollDefaults(cef)
}

const legacyCEFScrollTouchpadMultiplierDefault = 0.35

func migrateLegacyCEFInputScrollDefaults(cef map[string]any) {
	input, ok := cef["input"].(map[string]any)
	if !ok {
		return
	}

	if legacyTouchpad, hasLegacyTouchpad := input["scroll_touchpad_multiplier"]; hasLegacyTouchpad {
		if precise, hasPrecise := input["scroll_precise_multiplier"]; !hasPrecise {
			input["scroll_precise_multiplier"] = migratedCEFPreciseScrollMultiplier(legacyTouchpad)
		} else if shouldPreferLegacyTouchpadScrollMultiplier(legacyTouchpad, precise) {
			input["scroll_precise_multiplier"] = legacyTouchpad
		}
		delete(input, "scroll_touchpad_multiplier")
	}

	if current, hasPrecise := input["scroll_precise_multiplier"]; hasPrecise {
		input["scroll_precise_multiplier"] = migratedCEFPreciseScrollMultiplier(current)
	}
}

func shouldPreferLegacyTouchpadScrollMultiplier(legacyTouchpad, precise any) bool {
	legacyMultiplier, legacyOK := float64ConfigValue(legacyTouchpad)
	preciseMultiplier, preciseOK := float64ConfigValue(precise)
	if !legacyOK || !preciseOK {
		return false
	}
	return !sameConfigFloat(legacyMultiplier, legacyCEFScrollTouchpadMultiplierDefault) &&
		(sameConfigFloat(preciseMultiplier, defaultCEFScrollPreciseMultiplier) ||
			sameConfigFloat(preciseMultiplier, legacyCEFScrollTouchpadMultiplierDefault))
}

func migratedCEFPreciseScrollMultiplier(value any) any {
	multiplier, ok := float64ConfigValue(value)
	if !ok || !sameConfigFloat(multiplier, legacyCEFScrollTouchpadMultiplierDefault) {
		return value
	}
	return defaultCEFScrollPreciseMultiplier
}

func sameConfigFloat(a, b float64) bool {
	return math.Abs(a-b) <= 0.000001
}

func migrateLegacyCEFWindowlessFrameRateDefault(cef map[string]any, hasAdaptive, adaptiveEnabled bool) {
	if hasAdaptive && !adaptiveEnabled {
		return
	}
	frameRate, ok := int64ConfigValue(cef["windowless_frame_rate"])
	if !ok || frameRate != defaultCEFWindowlessFrameRate {
		return
	}
	cef["adaptive_windowless_frame_rate"] = true
	cef["windowless_frame_rate"] = 0
	if _, hasMax := cef["windowless_frame_rate_max"]; !hasMax {
		cef["windowless_frame_rate_max"] = defaultCEFWindowlessFrameRateMax
	}
}

func nestedConfigBoolValue(rawConfig map[string]any, path ...string) (bool, bool) {
	current := rawConfig
	for i, key := range path {
		value, ok := current[key]
		if !ok {
			return false, false
		}
		if i == len(path)-1 {
			parsed, parsedOK := value.(bool)
			return parsed, parsedOK
		}
		next, ok := value.(map[string]any)
		if !ok {
			return false, false
		}
		current = next
	}
	return false, false
}

func float64ConfigValue(value any) (float64, bool) {
	switch v := value.(type) {
	case float32:
		return float64(v), true
	case float64:
		return v, true
	case int:
		return float64(v), true
	case int8:
		return float64(v), true
	case int16:
		return float64(v), true
	case int32:
		return float64(v), true
	case int64:
		return float64(v), true
	case uint:
		return float64(v), true
	case uint8:
		return float64(v), true
	case uint16:
		return float64(v), true
	case uint32:
		return float64(v), true
	case uint64:
		return float64(v), true
	default:
		return 0, false
	}
}

func int64ConfigValue(value any) (int64, bool) {
	switch v := value.(type) {
	case int:
		return int64(v), true
	case int8:
		return int64(v), true
	case int16:
		return int64(v), true
	case int32:
		return int64(v), true
	case int64:
		return v, true
	case uint:
		return int64(v), true
	case uint8:
		return int64(v), true
	case uint16:
		return int64(v), true
	case uint32:
		return int64(v), true
	case uint64:
		if v > math.MaxInt64 {
			return 0, false
		}
		return int64(v), true
	default:
		return 0, false
	}
}

func (t *LegacyConfigTransformer) transformActionSection(rawConfig map[string]any, path []string) {
	// Navigate to the parent of the actions section
	current := rawConfig
	for _, key := range path[:len(path)-1] {
		next, ok := current[key].(map[string]any)
		if !ok {
			return // Path doesn't exist
		}
		current = next
	}

	actionsKey := path[len(path)-1]
	actionsRaw, ok := current[actionsKey]
	if !ok {
		return
	}

	actions, ok := actionsRaw.(map[string]any)
	if !ok {
		return
	}

	// Check each action and transform if needed
	for actionName, actionValue := range actions {
		// If value is a slice (old format), convert to ActionBinding format
		if slice, ok := actionValue.([]any); ok {
			keys := make([]string, 0, len(slice))
			for _, v := range slice {
				if s, ok := v.(string); ok {
					keys = append(keys, s)
				}
			}

			actions[actionName] = map[string]any{
				"keys": keys,
				"desc": t.getDefaultDesc(path, actionName),
			}
		}
	}
}

// TransformLegacyPopupsToBrowsingContexts maps old workspace.popups.* keys to
// workspace.browsing_contexts.* when the new section is absent. This allows
// existing user configs with [workspace.popups] to be silently migrated.
func (t *LegacyConfigTransformer) TransformLegacyPopupsToBrowsingContexts(rawConfig map[string]any) {
	workspace, ok := rawConfig["workspace"].(map[string]any)
	if !ok {
		return
	}

	// Only transform if the new key does not already exist.
	if _, hasNew := workspace["browsing_contexts"]; hasNew {
		return
	}

	t.ForceLegacyPopupsToBrowsingContexts(rawConfig)
}

// ForceLegacyPopupsToBrowsingContexts maps old workspace.popups.* keys to
// workspace.browsing_contexts.* even when the new section already exists in the
// raw map. This is used by the loader because Viper's AllSettings includes
// defaults for the new section that do not indicate user intent.
func (*LegacyConfigTransformer) ForceLegacyPopupsToBrowsingContexts(rawConfig map[string]any) {
	workspace, ok := rawConfig["workspace"].(map[string]any)
	if !ok {
		return
	}

	oldPopups, ok := workspace["popups"]
	if !ok {
		return
	}

	// Move the popups map to browsing_contexts and delete the old key.
	workspace["browsing_contexts"] = oldPopups
	delete(workspace, "popups")
}

func (t *LegacyConfigTransformer) getDefaultDesc(path []string, actionName string) string {
	switch {
	case path[0] == sectionWorkspace && path[1] == "pane_mode":
		if b, ok := t.defaults.Workspace.PaneMode.Actions[actionName]; ok {
			return b.Desc
		}
	case path[0] == sectionWorkspace && path[1] == "tab_mode":
		if b, ok := t.defaults.Workspace.TabMode.Actions[actionName]; ok {
			return b.Desc
		}
	case path[0] == sectionWorkspace && path[1] == "resize_mode":
		if b, ok := t.defaults.Workspace.ResizeMode.Actions[actionName]; ok {
			return b.Desc
		}
	case path[0] == sectionWorkspace && path[1] == "shortcuts":
		if b, ok := t.defaults.Workspace.Shortcuts.Actions[actionName]; ok {
			return b.Desc
		}
	case path[0] == sectionSession && path[1] == "session_mode":
		if b, ok := t.defaults.Session.SessionMode.Actions[actionName]; ok {
			return b.Desc
		}
	}
	return ""
}
