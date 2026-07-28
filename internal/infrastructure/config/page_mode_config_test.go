package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultConfig_PageModeDefaults(t *testing.T) {
	cfg := DefaultConfig()

	// PageMode must exist in workspace config.
	pageMode := cfg.Workspace.PageMode
	assert.Equal(t, "ctrl+y", pageMode.ActivationShortcut)
	assert.Equal(t, 0, pageMode.TimeoutMilliseconds)

	// All required actions must exist with exact key bindings.
	require.NotNil(t, pageMode.Actions)
	requireActionBinding(t, pageMode.Actions, "page-scroll-left", []string{"h"})
	requireActionBinding(t, pageMode.Actions, "page-scroll-down", []string{"j"})
	requireActionBinding(t, pageMode.Actions, "page-scroll-up", []string{"k"})
	requireActionBinding(t, pageMode.Actions, "page-scroll-right", []string{"l"})
	requireActionBinding(t, pageMode.Actions, "page-scroll-down-fast", []string{"shift+j"})
	requireActionBinding(t, pageMode.Actions, "page-scroll-up-fast", []string{"shift+k"})
	requireActionBinding(t, pageMode.Actions, "confirm", []string{"enter"})
	requireActionBinding(t, pageMode.Actions, "cancel", []string{"escape"})
}

func TestPageModeConfig_GetKeyBindings(t *testing.T) {
	cfg := DefaultConfig()
	bindings := cfg.Workspace.PageMode.GetKeyBindings()

	assert.Equal(t, "page-scroll-left", bindings["h"])
	assert.Equal(t, "page-scroll-down", bindings["j"])
	assert.Equal(t, "page-scroll-up", bindings["k"])
	assert.Equal(t, "page-scroll-right", bindings["l"])
	assert.Equal(t, "page-scroll-down-fast", bindings["shift+j"])
	assert.Equal(t, "page-scroll-up-fast", bindings["shift+k"])
	assert.Equal(t, "confirm", bindings["enter"])
	assert.Equal(t, "cancel", bindings["escape"])
	assert.Len(t, bindings, 8)
}

func TestValidatePageMode_NonNegativeTimeout(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Config)
		want   string
	}{
		{
			name: "negative timeout fails",
			mutate: func(cfg *Config) {
				cfg.Workspace.PageMode.TimeoutMilliseconds = -1
			},
			want: "workspace.page_mode.timeout_ms must be non-negative",
		},
		{
			name: "zero timeout ok",
			mutate: func(_ *Config) {
			},
			want: "",
		},
		{
			name: "positive timeout ok",
			mutate: func(cfg *Config) {
				cfg.Workspace.PageMode.TimeoutMilliseconds = 5000
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			tt.mutate(cfg)
			err := validateConfig(cfg)
			if tt.want == "" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.want)
			}
		})
	}
}

func TestValidatePageMode_NilActionsFail(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Workspace.PageMode.Actions = nil

	err := validateConfig(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "workspace.page_mode.actions cannot be empty")
}

func TestValidatePageMode_DuplicateKeyBindings(t *testing.T) {
	cfg := DefaultConfig()
	// Add a duplicate key binding that conflicts with an existing one.
	action := "custom-action"
	cfg.Workspace.PageMode.Actions[action] = ActionBinding{
		Keys: []string{"h"}, // "h" is already used by page-scroll-left
		Desc: "Custom action",
	}

	err := validateConfig(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate key binding 'h' found in page_mode actions")
	assert.Contains(t, err.Error(), "page-scroll-left")
	assert.Contains(t, err.Error(), action)
}

func TestValidatePageMode_EmptyActionKeysFail(t *testing.T) {
	cfg := DefaultConfig()
	// Override an action with no key bindings.
	cfg.Workspace.PageMode.Actions = map[string]ActionBinding{
		"page-scroll-left": {Keys: []string{}, Desc: "No keys"},
	}

	err := validateConfig(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "workspace.page_mode.actions.page-scroll-left must have at least one key binding")
}

func TestSchemaProvider_PageModeKeys(t *testing.T) {
	provider := &SchemaProvider{}
	schema := provider.GetSchema()

	var foundActivation, foundTimeout, foundActions bool
	for _, key := range schema {
		switch key.Key {
		case "workspace.page_mode.activation_shortcut":
			foundActivation = true
			assert.Equal(t, "string", key.Type)
			assert.Equal(t, "ctrl+y", key.Default)
			assert.Equal(t, SectionWorkspace, key.Section)
		case "workspace.page_mode.timeout_ms":
			foundTimeout = true
			assert.Equal(t, "int", key.Type)
			assert.Equal(t, "0", key.Default)
			assert.Equal(t, ">=0", key.Range)
			assert.Equal(t, SectionWorkspace, key.Section)
		case "workspace.page_mode.actions.<action>":
			foundActions = true
			assert.Equal(t, "[]string", key.Type)
			assert.Equal(t, SectionWorkspace, key.Section)
		}
	}

	assert.True(t, foundActivation, "missing workspace.page_mode.activation_shortcut schema key")
	assert.True(t, foundTimeout, "missing workspace.page_mode.timeout_ms schema key")
	assert.True(t, foundActions, "missing workspace.page_mode.actions.<action> schema key")
}
