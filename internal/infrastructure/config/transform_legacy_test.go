package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTransformLegacyActions_SliceFormat(t *testing.T) {
	transformer := NewLegacyConfigTransformer()

	rawConfig := map[string]any{
		"workspace": map[string]any{
			"pane_mode": map[string]any{
				"actions": map[string]any{
					"focus-left": []any{"h", "arrowleft"},
				},
			},
		},
	}

	transformer.TransformLegacyActions(rawConfig)

	actions := rawConfig["workspace"].(map[string]any)["pane_mode"].(map[string]any)["actions"].(map[string]any)
	binding, ok := actions["focus-left"].(map[string]any)
	require.True(t, ok, "should be converted to map")

	keys, ok := binding["keys"].([]string)
	require.True(t, ok, "keys should be []string")
	assert.Equal(t, []string{"h", "arrowleft"}, keys)
	assert.NotEmpty(t, binding["desc"], "should have description from defaults")
}

func TestTransformLegacyActions_AllModes(t *testing.T) {
	transformer := NewLegacyConfigTransformer()

	rawConfig := map[string]any{
		"workspace": map[string]any{
			"pane_mode": map[string]any{
				"actions": map[string]any{
					"focus-left": []any{"h"},
				},
			},
			"tab_mode": map[string]any{
				"actions": map[string]any{
					"next-tab": []any{"l"},
				},
			},
			"resize_mode": map[string]any{
				"actions": map[string]any{
					"resize-increase": []any{"+"},
				},
			},
			"shortcuts": map[string]any{
				"actions": map[string]any{
					"close_pane": []any{"ctrl+w"},
				},
			},
		},
		"session": map[string]any{
			"session_mode": map[string]any{
				"actions": map[string]any{
					"session-manager": []any{"m"},
				},
			},
		},
	}

	transformer.TransformLegacyActions(rawConfig)

	// Check pane mode
	paneActions := rawConfig["workspace"].(map[string]any)["pane_mode"].(map[string]any)["actions"].(map[string]any)
	paneFocus, ok := paneActions["focus-left"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, []string{"h"}, paneFocus["keys"])

	// Check tab mode
	tabActions := rawConfig["workspace"].(map[string]any)["tab_mode"].(map[string]any)["actions"].(map[string]any)
	tabNext, ok := tabActions["next-tab"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, []string{"l"}, tabNext["keys"])

	// Check resize mode
	resizeActions := rawConfig["workspace"].(map[string]any)["resize_mode"].(map[string]any)["actions"].(map[string]any)
	resizeInc, ok := resizeActions["resize-increase"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, []string{"+"}, resizeInc["keys"])

	// Check shortcuts
	shortcutActions := rawConfig["workspace"].(map[string]any)["shortcuts"].(map[string]any)["actions"].(map[string]any)
	closePane, ok := shortcutActions["close_pane"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, []string{"ctrl+w"}, closePane["keys"])

	// Check session mode
	sessionActions := rawConfig["session"].(map[string]any)["session_mode"].(map[string]any)["actions"].(map[string]any)
	sessionMgr, ok := sessionActions["session-manager"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, []string{"m"}, sessionMgr["keys"])
}

func TestTransformLegacyActions_AlreadyNewFormat(t *testing.T) {
	transformer := NewLegacyConfigTransformer()

	rawConfig := map[string]any{
		"workspace": map[string]any{
			"pane_mode": map[string]any{
				"actions": map[string]any{
					"focus-left": map[string]any{
						"keys": []string{"h"},
						"desc": "Custom description",
					},
				},
			},
		},
	}

	transformer.TransformLegacyActions(rawConfig)

	actions := rawConfig["workspace"].(map[string]any)["pane_mode"].(map[string]any)["actions"].(map[string]any)
	binding := actions["focus-left"].(map[string]any)

	// Should remain unchanged
	assert.Equal(t, "Custom description", binding["desc"])
}

func TestTransformLegacyActions_MissingSection(t *testing.T) {
	transformer := NewLegacyConfigTransformer()

	rawConfig := map[string]any{
		"appearance": map[string]any{
			"theme": "dark",
		},
	}

	// Should not panic
	transformer.TransformLegacyActions(rawConfig)

	// Config should be unchanged
	assert.Equal(t, "dark", rawConfig["appearance"].(map[string]any)["theme"])
}

func TestTransformLegacyActions_EmptyConfig(t *testing.T) {
	transformer := NewLegacyConfigTransformer()

	rawConfig := map[string]any{}

	// Should not panic
	transformer.TransformLegacyActions(rawConfig)

	assert.Empty(t, rawConfig)
}

func TestTransformLegacyActions_PartialPath(t *testing.T) {
	transformer := NewLegacyConfigTransformer()

	rawConfig := map[string]any{
		"workspace": map[string]any{
			"pane_mode": map[string]any{
				// actions section missing
			},
		},
	}

	// Should not panic
	transformer.TransformLegacyActions(rawConfig)

	// Verify config is unchanged
	require.NotNil(t, rawConfig["workspace"])
}

func TestTransformLegacyEngineConfig_RemovesDeprecatedContextMenuHandler(t *testing.T) {
	transformer := NewLegacyConfigTransformer()

	rawConfig := map[string]any{
		"engine": map[string]any{
			"cef": map[string]any{
				"enable_context_menu_handler": true,
			},
		},
	}

	transformer.TransformLegacyEngineConfig(rawConfig)

	cef := rawConfig["engine"].(map[string]any)["cef"].(map[string]any)
	_, exists := cef["enable_context_menu_handler"]
	assert.False(t, exists)
}

func TestTransformLegacyEngineConfig_MigratesLegacyCEFWindowlessFrameRateDefault(t *testing.T) {
	transformer := NewLegacyConfigTransformer()
	rawConfig := map[string]any{
		"engine": map[string]any{
			"cef": map[string]any{
				"windowless_frame_rate": 60,
			},
		},
	}

	transformer.TransformLegacyEngineConfig(rawConfig)

	cef := rawConfig["engine"].(map[string]any)["cef"].(map[string]any)
	assert.Equal(t, true, cef["adaptive_windowless_frame_rate"])
	assert.Equal(t, 0, cef["windowless_frame_rate"])
	assert.Equal(t, defaultCEFWindowlessFrameRateMax, cef["windowless_frame_rate_max"])
}

func TestTransformLegacyEngineConfig_UpgradesObsoletePreciseScrollDefault(t *testing.T) {
	transformer := NewLegacyConfigTransformer()
	rawConfig := map[string]any{
		"engine": map[string]any{
			"cef": map[string]any{
				"input": map[string]any{
					"scroll_precise_multiplier": 0.35,
				},
			},
		},
	}

	transformer.TransformLegacyEngineConfig(rawConfig)

	input := rawConfig["engine"].(map[string]any)["cef"].(map[string]any)["input"].(map[string]any)
	assert.InDelta(t, defaultCEFScrollPreciseMultiplier, input["scroll_precise_multiplier"], 0.001)
}

func TestTransformLegacyEngineConfig_MigratesLegacyTouchpadScrollKey(t *testing.T) {
	transformer := NewLegacyConfigTransformer()
	rawConfig := map[string]any{
		"engine": map[string]any{
			"cef": map[string]any{
				"input": map[string]any{
					"scroll_touchpad_multiplier": 0.35,
				},
			},
		},
	}

	transformer.TransformLegacyEngineConfig(rawConfig)

	input := rawConfig["engine"].(map[string]any)["cef"].(map[string]any)["input"].(map[string]any)
	assert.InDelta(t, defaultCEFScrollPreciseMultiplier, input["scroll_precise_multiplier"], 0.001)
	assert.NotContains(t, input, "scroll_touchpad_multiplier")
}

func TestTransformLegacyEngineConfig_PreservesCustomPreciseScrollMultiplier(t *testing.T) {
	transformer := NewLegacyConfigTransformer()
	rawConfig := map[string]any{
		"engine": map[string]any{
			"cef": map[string]any{
				"input": map[string]any{
					"scroll_precise_multiplier": 4.0,
				},
			},
		},
	}

	transformer.TransformLegacyEngineConfig(rawConfig)

	input := rawConfig["engine"].(map[string]any)["cef"].(map[string]any)["input"].(map[string]any)
	assert.InDelta(t, 4.0, input["scroll_precise_multiplier"], 0.001)
}

func TestTransformLegacyEngineConfig_PreservesCustomLegacyTouchpadScrollMultiplier(t *testing.T) {
	transformer := NewLegacyConfigTransformer()
	rawConfig := map[string]any{
		"engine": map[string]any{
			"cef": map[string]any{
				"input": map[string]any{
					"scroll_touchpad_multiplier": 4.0,
				},
			},
		},
	}

	transformer.TransformLegacyEngineConfig(rawConfig)

	input := rawConfig["engine"].(map[string]any)["cef"].(map[string]any)["input"].(map[string]any)
	assert.InDelta(t, 4.0, input["scroll_precise_multiplier"], 0.001)
	assert.NotContains(t, input, "scroll_touchpad_multiplier")
}

func TestTransformLegacyEngineConfig_PrefersCustomLegacyTouchpadOverGeneratedPreciseDefault(t *testing.T) {
	transformer := NewLegacyConfigTransformer()
	rawConfig := map[string]any{
		"engine": map[string]any{
			"cef": map[string]any{
				"input": map[string]any{
					"scroll_touchpad_multiplier": 4.0,
					"scroll_precise_multiplier":  0.35,
				},
			},
		},
	}

	transformer.TransformLegacyEngineConfig(rawConfig)

	input := rawConfig["engine"].(map[string]any)["cef"].(map[string]any)["input"].(map[string]any)
	assert.InDelta(t, 4.0, input["scroll_precise_multiplier"], 0.001)
	assert.NotContains(t, input, "scroll_touchpad_multiplier")
}

func TestTransformLegacyEngineConfig_PreservesExplicitCEFWindowlessFrameRate(t *testing.T) {
	transformer := NewLegacyConfigTransformer()
	rawConfig := map[string]any{
		"engine": map[string]any{
			"cef": map[string]any{
				"adaptive_windowless_frame_rate": false,
				"windowless_frame_rate":          60,
			},
		},
	}

	transformer.TransformLegacyEngineConfig(rawConfig)

	cef := rawConfig["engine"].(map[string]any)["cef"].(map[string]any)
	assert.Equal(t, false, cef["adaptive_windowless_frame_rate"])
	assert.Equal(t, 60, cef["windowless_frame_rate"])
	_, hasMax := cef["windowless_frame_rate_max"]
	assert.False(t, hasMax)
}

func TestTransformLegacyEngineConfig_PreservesNonDefaultCEFWindowlessFrameRate(t *testing.T) {
	transformer := NewLegacyConfigTransformer()
	rawConfig := map[string]any{
		"engine": map[string]any{
			"cef": map[string]any{
				"windowless_frame_rate": 144,
			},
		},
	}

	transformer.TransformLegacyEngineConfig(rawConfig)

	cef := rawConfig["engine"].(map[string]any)["cef"].(map[string]any)
	_, hasAdaptive := cef["adaptive_windowless_frame_rate"]
	assert.False(t, hasAdaptive)
	assert.Equal(t, 144, cef["windowless_frame_rate"])
}

func TestTransformLegacyEngineConfig_MissingCefSection(t *testing.T) {
	transformer := NewLegacyConfigTransformer()
	rawConfig := map[string]any{
		"engine": map[string]any{
			"webkit": map[string]any{"enabled": true},
		},
	}

	transformer.TransformLegacyEngineConfig(rawConfig)

	engine, ok := rawConfig["engine"].(map[string]any)
	require.True(t, ok)
	assert.NotNil(t, engine)
	assert.Equal(t, true, engine["webkit"].(map[string]any)["enabled"])
}

func TestTransformLegacyEngineConfig_MissingContextMenuHandlerKey(t *testing.T) {
	transformer := NewLegacyConfigTransformer()
	rawConfig := map[string]any{
		"engine": map[string]any{
			"cef": map[string]any{
				"other_setting": true,
			},
		},
	}

	transformer.TransformLegacyEngineConfig(rawConfig)

	cef := rawConfig["engine"].(map[string]any)["cef"].(map[string]any)
	assert.True(t, cef["other_setting"].(bool))
	_, exists := cef["enable_context_menu_handler"]
	assert.False(t, exists)
}

func TestTransformLegacyEngineConfig_NonMapEngineSection(t *testing.T) {
	transformer := NewLegacyConfigTransformer()
	rawConfig := map[string]any{
		"engine": "invalid",
	}

	transformer.TransformLegacyEngineConfig(rawConfig)

	assert.Equal(t, "invalid", rawConfig["engine"])
}

func TestTransformLegacyActions_UnknownAction(t *testing.T) {
	transformer := NewLegacyConfigTransformer()

	rawConfig := map[string]any{
		"workspace": map[string]any{
			"pane_mode": map[string]any{
				"actions": map[string]any{
					"unknown-action": []any{"x"},
				},
			},
		},
	}

	transformer.TransformLegacyActions(rawConfig)

	actions := rawConfig["workspace"].(map[string]any)["pane_mode"].(map[string]any)["actions"].(map[string]any)
	binding, ok := actions["unknown-action"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, []string{"x"}, binding["keys"])
	assert.Empty(t, binding["desc"], "unknown action should have empty description")
}
