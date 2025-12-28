package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMigrator_NewMigrator(t *testing.T) {
	m := NewMigrator()
	require.NotNil(t, m)
	require.NotNil(t, m.defaultViper)
	require.NotNil(t, m.defaultConfig)
}

func TestMigrator_GetAllDefaultKeys(t *testing.T) {
	m := NewMigrator()
	keys := m.getAllDefaultKeys()

	// Should have many default keys
	assert.Greater(t, len(keys), 50, "should have many default keys")

	// Should not include database.path (it's dynamically set)
	for _, key := range keys {
		assert.NotEqual(t, "database.path", key, "should not include database.path")
	}

	// Should include some known keys
	keySet := make(map[string]bool)
	for _, k := range keys {
		keySet[k] = true
	}

	assert.True(t, keySet["history.max_entries"], "should include history.max_entries")
	assert.True(t, keySet["logging.level"], "should include logging.level")
	assert.True(t, keySet["appearance.sans_font"], "should include appearance.sans_font")
}

func TestMigrator_GetUserConfigKeys(t *testing.T) {
	m := NewMigrator()

	// Create a temp config file
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.toml")

	content := `
[history]
max_entries = 5000
retention_period_days = 180

[logging]
level = "debug"

[appearance]
sans_font = "Arial"
`
	err := os.WriteFile(configFile, []byte(content), 0o644)
	require.NoError(t, err)

	keys, err := m.getUserConfigKeys(configFile)
	require.NoError(t, err)

	// Should have the keys we defined
	assert.True(t, keys["history.max_entries"])
	assert.True(t, keys["history.retention_period_days"])
	assert.True(t, keys["logging.level"])
	assert.True(t, keys["appearance.sans_font"])

	// Should not have keys we didn't define
	assert.False(t, keys["history.cleanup_interval_days"])
	assert.False(t, keys["logging.format"])
}

func TestMigrator_FlattenMap(t *testing.T) {
	m := NewMigrator()

	tests := []struct {
		name     string
		input    map[string]any
		prefix   string
		expected []string
	}{
		{
			name: "simple flat map",
			input: map[string]any{
				"key1": "value1",
				"key2": 123,
			},
			prefix:   "",
			expected: []string{"key1", "key2"},
		},
		{
			name: "nested map",
			input: map[string]any{
				"parent": map[string]any{
					"child1": "value1",
					"child2": "value2",
				},
			},
			prefix:   "",
			expected: []string{"parent.child1", "parent.child2"},
		},
		{
			name: "with prefix",
			input: map[string]any{
				"key": "value",
			},
			prefix:   "section",
			expected: []string{"section.key"},
		},
		{
			name: "search_shortcuts treated as single key",
			input: map[string]any{
				"search_shortcuts": map[string]any{
					"g":   "https://google.com/?q=%s",
					"ddg": "https://duckduckgo.com/?q=%s",
				},
			},
			prefix:   "",
			expected: []string{"search_shortcuts"}, // Should be treated as user data
		},
		{
			name: "actions map treated as single key",
			input: map[string]any{
				"pane_mode": map[string]any{
					"actions": map[string]any{
						"split-right": []string{"r"},
					},
				},
			},
			prefix:   "",
			expected: []string{"pane_mode.actions"}, // Actions treated as user data
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keys := make(map[string]bool)
			m.flattenMap(tt.input, tt.prefix, keys)

			for _, expectedKey := range tt.expected {
				assert.True(t, keys[expectedKey], "should contain key: %s", expectedKey)
			}
		})
	}
}

func TestMigrator_IsUserDataSection(t *testing.T) {
	m := NewMigrator()

	tests := []struct {
		name     string
		keyPath  string
		expected bool
	}{
		{
			name:     "search_shortcuts is user data",
			keyPath:  "search_shortcuts",
			expected: true,
		},
		{
			name:     "pane_mode.actions is user data",
			keyPath:  "pane_mode.actions",
			expected: true,
		},
		{
			name:     "tab_mode.actions is user data",
			keyPath:  "tab_mode.actions",
			expected: true,
		},
		{
			name:     "history is not user data",
			keyPath:  "history",
			expected: false,
		},
		{
			name:     "history.max_entries is not user data",
			keyPath:  "history.max_entries",
			expected: false,
		},
		{
			name:     "appearance.sans_font is not user data",
			keyPath:  "appearance.sans_font",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := m.isUserDataSection(tt.keyPath)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMigrator_FindMissingKeys(t *testing.T) {
	m := NewMigrator()

	tests := []struct {
		name        string
		defaultKeys []string
		userKeys    map[string]bool
		expected    []string
	}{
		{
			name:        "all keys present",
			defaultKeys: []string{"key1", "key2"},
			userKeys:    map[string]bool{"key1": true, "key2": true},
			expected:    []string{},
		},
		{
			name:        "some keys missing",
			defaultKeys: []string{"key1", "key2", "key3"},
			userKeys:    map[string]bool{"key1": true},
			expected:    []string{"key2", "key3"},
		},
		{
			name:        "all keys missing",
			defaultKeys: []string{"key1", "key2"},
			userKeys:    map[string]bool{},
			expected:    []string{"key1", "key2"},
		},
		{
			name:        "parent key covers children",
			defaultKeys: []string{"parent.child1", "parent.child2"},
			userKeys:    map[string]bool{"parent": true},
			expected:    []string{}, // Parent covers children
		},
		{
			name:        "child keys cover parent (struct defaults like palettes)",
			defaultKeys: []string{"appearance.dark_palette"},
			userKeys:    map[string]bool{"appearance.dark_palette.background": true, "appearance.dark_palette.accent": true},
			expected:    []string{}, // Child keys mean parent is defined
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := m.findMissingKeys(tt.defaultKeys, tt.userKeys)
			assert.ElementsMatch(t, tt.expected, result)
		})
	}
}

func TestMigrator_GetKeyInfo(t *testing.T) {
	m := NewMigrator()

	tests := []struct {
		name         string
		key          string
		expectedType string
	}{
		{
			name:         "bool key",
			key:          "debug.enable_devtools",
			expectedType: "bool",
		},
		{
			name:         "int key",
			key:          "history.max_entries",
			expectedType: "int",
		},
		{
			name:         "string key",
			key:          "logging.level",
			expectedType: "string",
		},
		{
			name:         "float key",
			key:          "default_webpage_zoom",
			expectedType: "float",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := m.GetKeyInfo(tt.key)
			assert.Equal(t, tt.key, info.Key)
			assert.Equal(t, tt.expectedType, info.Type)
			assert.NotEmpty(t, info.DefaultValue)
		})
	}
}

func TestMigrator_GetTypeName(t *testing.T) {
	m := NewMigrator()

	tests := []struct {
		name     string
		value    any
		expected string
	}{
		{name: "bool", value: true, expected: "bool"},
		{name: "int", value: 42, expected: "int"},
		{name: "int64", value: int64(42), expected: "int"},
		{name: "float64", value: 3.14, expected: "float"},
		{name: "string", value: "hello", expected: "string"},
		{name: "slice", value: []any{"a", "b"}, expected: "list"},
		{name: "map", value: map[string]any{"a": 1}, expected: "map"},
		{name: "nil", value: nil, expected: "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := m.getTypeName(tt.value)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMigrator_FormatValue(t *testing.T) {
	m := NewMigrator()

	tests := []struct {
		name     string
		value    any
		expected string
	}{
		{name: "true", value: true, expected: "true"},
		{name: "false", value: false, expected: "false"},
		{name: "int", value: 42, expected: "42"},
		{name: "float", value: 3.14, expected: "3.14"},
		{name: "string", value: "hello", expected: `"hello"`},
		{name: "empty string", value: "", expected: `""`},
		{name: "empty slice", value: []any{}, expected: "[]"},
		{name: "slice with items", value: []any{"a", "b", "c"}, expected: "[3 items]"},
		{name: "empty map", value: map[string]any{}, expected: "{}"},
		{name: "map with entries", value: map[string]any{"a": 1, "b": 2}, expected: "{2 entries}"},
		{name: "nil", value: nil, expected: "null"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := m.formatValue(tt.value)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestKeysAreSimilar(t *testing.T) {
	tests := []struct {
		name     string
		oldKey   string
		newKey   string
		expected bool
	}{
		{
			name:     "identical keys",
			oldKey:   "section.pane_mode_color",
			newKey:   "section.pane_mode_color",
			expected: true,
		},
		{
			name:     "substring match - old contains new",
			oldKey:   "section.pane_mode_border_color",
			newKey:   "section.pane_mode_color",
			expected: true,
		},
		{
			name:     "substring match - new contains old",
			oldKey:   "section.mode_color",
			newKey:   "section.pane_mode_color",
			expected: true,
		},
		{
			name:     "token matching - border removed",
			oldKey:   "section.tab_mode_border_color",
			newKey:   "section.tab_mode_color",
			expected: true,
		},
		{
			name:     "different parent paths",
			oldKey:   "section_a.pane_mode_color",
			newKey:   "section_b.pane_mode_color",
			expected: false,
		},
		{
			name:     "different depth",
			oldKey:   "pane_mode_color",
			newKey:   "section.pane_mode_color",
			expected: false,
		},
		{
			name:     "completely different keys",
			oldKey:   "section.foo_bar",
			newKey:   "section.baz_qux",
			expected: false,
		},
		{
			name:     "top level keys rejected",
			oldKey:   "foo",
			newKey:   "bar",
			expected: false,
		},
		{
			name:     "no common tokens",
			oldKey:   "section.aaa_bbb",
			newKey:   "section.ccc_ddd",
			expected: false,
		},
		{
			name:     "single token difference allowed",
			oldKey:   "section.pane_mode_border_width",
			newKey:   "section.mode_border_width",
			expected: true,
		},
		{
			name:     "duplicate tokens in old key - no double counting",
			oldKey:   "section.mode_mode_mode",
			newKey:   "section.mode_other_another",
			expected: false, // oldTokens=[mode,mode,mode], newTokens=[mode,other,another], matches=1 (only one mode can match), minTokens=3, needs 1>=2 which is false
		},
		{
			name:     "enough common tokens with duplicates",
			oldKey:   "section.pane_mode_border",
			newKey:   "section.pane_mode_color",
			expected: true, // oldTokens=[pane,mode,border], newTokens=[pane,mode,color], matches=2, minTokens=3, 2>=2 && 2>0 = true
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := keysAreSimilar(tt.oldKey, tt.newKey)
			assert.Equal(t, tt.expected, result, "keysAreSimilar(%q, %q)", tt.oldKey, tt.newKey)
		})
	}
}

func TestMigrator_TypesAreCompatible(t *testing.T) {
	m := NewMigrator()

	tests := []struct {
		name     string
		oldKey   string
		newKey   string
		expected bool
	}{
		{
			name:     "color to color - compatible (using real key)",
			oldKey:   "workspace.styling.pane_mode_border_color",
			newKey:   "workspace.styling.pane_mode_color",
			expected: true,
		},
		{
			name:     "width to width - compatible (using real key)",
			oldKey:   "workspace.styling.pane_mode_border_width",
			newKey:   "workspace.styling.mode_border_width",
			expected: true,
		},
		{
			name:     "width suffix to color type - incompatible",
			oldKey:   "workspace.styling.some_width",
			newKey:   "workspace.styling.pane_mode_color", // real key with string type
			expected: false,
		},
		{
			name:     "color suffix to int type - incompatible",
			oldKey:   "workspace.styling.some_color",
			newKey:   "workspace.styling.mode_border_width", // real key with int type
			expected: false,
		},
		{
			name:     "generic key to string type - compatible",
			oldKey:   "workspace.styling.some_setting",
			newKey:   "workspace.styling.pane_mode_color", // real key with string type
			expected: true,
		},
		{
			name:     "non-existent new key - incompatible",
			oldKey:   "section.old_key",
			newKey:   "section.nonexistent_key_xyz",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := m.typesAreCompatible(tt.oldKey, tt.newKey)
			assert.Equal(t, tt.expected, result, "typesAreCompatible(%q, %q)", tt.oldKey, tt.newKey)
		})
	}
}

func TestMigrator_FindDeprecatedKeys(t *testing.T) {
	m := NewMigrator()

	tests := []struct {
		name        string
		userKeys    map[string]any
		defaultKeys map[string]bool
		expected    []string
	}{
		{
			name: "no deprecated keys",
			userKeys: map[string]any{
				"history.max_entries": 1000,
				"logging.level":       "info",
			},
			defaultKeys: map[string]bool{
				"history.max_entries": true,
				"logging.level":       true,
			},
			expected: []string{},
		},
		{
			name: "one deprecated key",
			userKeys: map[string]any{
				"history.max_entries": 1000,
				"old.deprecated_key":  "value",
			},
			defaultKeys: map[string]bool{
				"history.max_entries": true,
			},
			expected: []string{"old.deprecated_key"},
		},
		{
			name: "multiple deprecated keys",
			userKeys: map[string]any{
				"old.key1": "a",
				"old.key2": "b",
				"old.key3": "c",
			},
			defaultKeys: map[string]bool{
				"new.key1": true,
			},
			expected: []string{"old.key1", "old.key2", "old.key3"},
		},
		{
			name: "parent key exists in defaults",
			userKeys: map[string]any{
				"parent.child": "value",
			},
			defaultKeys: map[string]bool{
				"parent.child":       true,
				"parent.other_child": true,
			},
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := m.findDeprecatedKeys(tt.userKeys, tt.defaultKeys)
			assert.ElementsMatch(t, tt.expected, result)
		})
	}
}

func TestMigrator_MatchRenamedKeys(t *testing.T) {
	m := NewMigrator()

	tests := []struct {
		name                 string
		deprecated           []string
		missing              []string
		expectedRenames      map[string]string
		expectedUnmatchedOld []string
		expectedUnmatchedNew []string
	}{
		{
			name:                 "no keys to match",
			deprecated:           []string{},
			missing:              []string{},
			expectedRenames:      map[string]string{},
			expectedUnmatchedOld: nil,
			expectedUnmatchedNew: nil,
		},
		{
			name:       "simple rename match (using real keys)",
			deprecated: []string{"workspace.styling.pane_mode_border_color"},
			missing:    []string{"workspace.styling.pane_mode_color"},
			expectedRenames: map[string]string{
				"workspace.styling.pane_mode_border_color": "workspace.styling.pane_mode_color",
			},
			expectedUnmatchedOld: nil,
			expectedUnmatchedNew: nil,
		},
		{
			name:                 "no match possible - different types (width vs color)",
			deprecated:           []string{"workspace.styling.pane_mode_border_width"},
			missing:              []string{"workspace.styling.pane_mode_color"},
			expectedRenames:      map[string]string{},
			expectedUnmatchedOld: []string{"workspace.styling.pane_mode_border_width"},
			expectedUnmatchedNew: []string{"workspace.styling.pane_mode_color"},
		},
		{
			name:                 "no match possible - different parents",
			deprecated:           []string{"old_section.some_key"},
			missing:              []string{"new_section.some_key"},
			expectedRenames:      map[string]string{},
			expectedUnmatchedOld: []string{"old_section.some_key"},
			expectedUnmatchedNew: []string{"new_section.some_key"},
		},
		{
			name:       "multiple renames (using real keys)",
			deprecated: []string{"workspace.styling.pane_mode_border_color", "workspace.styling.tab_mode_border_color"},
			missing:    []string{"workspace.styling.pane_mode_color", "workspace.styling.tab_mode_color"},
			expectedRenames: map[string]string{
				"workspace.styling.pane_mode_border_color": "workspace.styling.pane_mode_color",
				"workspace.styling.tab_mode_border_color":  "workspace.styling.tab_mode_color",
			},
			expectedUnmatchedOld: nil,
			expectedUnmatchedNew: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			renames, unmatchedOld, unmatchedNew := m.matchRenamedKeys(tt.deprecated, tt.missing)
			assert.Equal(t, tt.expectedRenames, renames, "renames mismatch")
			assert.ElementsMatch(t, tt.expectedUnmatchedOld, unmatchedOld, "unmatched old mismatch")
			assert.ElementsMatch(t, tt.expectedUnmatchedNew, unmatchedNew, "unmatched new mismatch")
		})
	}
}

func TestMigrator_CheckMigration_NoConfigFile(t *testing.T) {
	// This test would need to temporarily modify the config path
	// which is difficult without refactoring. Skip for now.
	t.Skip("requires config path mocking")
}

func TestMigrator_Migrate_NoConfigFile(t *testing.T) {
	// This test would need to temporarily modify the config path
	// which is difficult without refactoring. Skip for now.
	t.Skip("requires config path mocking")
}

func TestMigrator_GetConfigType(t *testing.T) {
	m := NewMigrator()

	tests := []struct {
		name       string
		configFile string
		expected   string
	}{
		{
			name:       "toml file",
			configFile: "/path/to/config.toml",
			expected:   "toml",
		},
		{
			name:       "yaml file",
			configFile: "/path/to/config.yaml",
			expected:   "yaml",
		},
		{
			name:       "yml file",
			configFile: "/path/to/config.yml",
			expected:   "yaml",
		},
		{
			name:       "json file",
			configFile: "/path/to/config.json",
			expected:   "json",
		},
		{
			name:       "unknown extension defaults to toml",
			configFile: "/path/to/config.unknown",
			expected:   "toml",
		},
		{
			name:       "no extension defaults to toml",
			configFile: "/path/to/config",
			expected:   "toml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := m.getConfigType(tt.configFile)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMigrator_GetNestedValue(t *testing.T) {
	m := NewMigrator()

	config := map[string]any{
		"level1": map[string]any{
			"level2": map[string]any{
				"level3": "deep_value",
			},
			"simple": "simple_value",
		},
		"top": "top_value",
	}

	tests := []struct {
		name     string
		key      string
		expected any
	}{
		{
			name:     "top level key",
			key:      "top",
			expected: "top_value",
		},
		{
			name:     "nested key",
			key:      "level1.simple",
			expected: "simple_value",
		},
		{
			name:     "deeply nested key",
			key:      "level1.level2.level3",
			expected: "deep_value",
		},
		{
			name:     "non-existent key",
			key:      "nonexistent",
			expected: nil,
		},
		{
			name:     "non-existent nested key",
			key:      "level1.nonexistent",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := m.getNestedValue(config, tt.key)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMigrator_SetNestedValue(t *testing.T) {
	m := NewMigrator()

	t.Run("set top level value", func(t *testing.T) {
		config := make(map[string]any)
		m.setNestedValue(config, "top", "value")
		assert.Equal(t, "value", config["top"])
	})

	t.Run("set nested value with existing parent", func(t *testing.T) {
		config := map[string]any{
			"parent": map[string]any{},
		}
		m.setNestedValue(config, "parent.child", "value")
		parent := config["parent"].(map[string]any)
		assert.Equal(t, "value", parent["child"])
	})

	t.Run("set nested value creates intermediate maps", func(t *testing.T) {
		config := make(map[string]any)
		m.setNestedValue(config, "a.b.c", "deep_value")

		a := config["a"].(map[string]any)
		b := a["b"].(map[string]any)
		assert.Equal(t, "deep_value", b["c"])
	})
}

func TestMigrator_DeleteNestedKey(t *testing.T) {
	m := NewMigrator()

	t.Run("delete top level key", func(t *testing.T) {
		config := map[string]any{
			"keep":   "value",
			"delete": "value",
		}
		m.deleteNestedKey(config, "delete")
		assert.Contains(t, config, "keep")
		assert.NotContains(t, config, "delete")
	})

	t.Run("delete nested key", func(t *testing.T) {
		config := map[string]any{
			"parent": map[string]any{
				"keep":   "value",
				"delete": "value",
			},
		}
		m.deleteNestedKey(config, "parent.delete")
		parent := config["parent"].(map[string]any)
		assert.Contains(t, parent, "keep")
		assert.NotContains(t, parent, "delete")
	})

	t.Run("delete nested key cleans up empty parents", func(t *testing.T) {
		config := map[string]any{
			"keep": "value",
			"parent": map[string]any{
				"child": map[string]any{
					"leaf": "value",
				},
			},
		}
		m.deleteNestedKey(config, "parent.child.leaf")
		// Both parent and child should be removed since they're empty
		assert.NotContains(t, config, "parent")
		assert.Contains(t, config, "keep")
	})

	t.Run("delete non-existent key is no-op", func(t *testing.T) {
		config := map[string]any{
			"keep": "value",
		}
		m.deleteNestedKey(config, "nonexistent")
		assert.Contains(t, config, "keep")
	})
}

func TestMigrator_ReadRawConfig(t *testing.T) {
	m := NewMigrator()

	t.Run("read TOML config", func(t *testing.T) {
		tmpDir := t.TempDir()
		configFile := filepath.Join(tmpDir, "config.toml")

		content := `
[section]
key = "value"
number = 42
`
		err := os.WriteFile(configFile, []byte(content), 0o644)
		require.NoError(t, err)

		config, err := m.readRawConfig(configFile)
		require.NoError(t, err)

		section := config["section"].(map[string]any)
		assert.Equal(t, "value", section["key"])
		assert.Equal(t, int64(42), section["number"])
	})

	t.Run("read YAML config", func(t *testing.T) {
		tmpDir := t.TempDir()
		configFile := filepath.Join(tmpDir, "config.yaml")

		content := `
section:
  key: value
  number: 42
`
		err := os.WriteFile(configFile, []byte(content), 0o644)
		require.NoError(t, err)

		config, err := m.readRawConfig(configFile)
		require.NoError(t, err)

		section := config["section"].(map[string]any)
		assert.Equal(t, "value", section["key"])
		assert.Equal(t, 42, section["number"])
	})

	t.Run("read JSON config", func(t *testing.T) {
		tmpDir := t.TempDir()
		configFile := filepath.Join(tmpDir, "config.json")

		content := `{
  "section": {
    "key": "value",
    "number": 42
  }
}`
		err := os.WriteFile(configFile, []byte(content), 0o644)
		require.NoError(t, err)

		config, err := m.readRawConfig(configFile)
		require.NoError(t, err)

		section := config["section"].(map[string]any)
		assert.Equal(t, "value", section["key"])
		// JSON numbers are float64
		assert.InDelta(t, 42.0, section["number"].(float64), 0.001)
	})
}
