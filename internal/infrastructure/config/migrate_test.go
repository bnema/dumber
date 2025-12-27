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
