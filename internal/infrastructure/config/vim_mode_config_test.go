package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultConfig_VimModeDefaults(t *testing.T) {
	cfg := DefaultConfig()

	// VimMode must exist in workspace config.
	vimMode := cfg.Workspace.VimMode
	assert.Equal(t, "ctrl+y", vimMode.ActivationShortcut)
	assert.Equal(t, 0, vimMode.TimeoutMilliseconds)

	// All required actions must exist with exact key bindings.
	require.NotNil(t, vimMode.Actions)
	requireActionBinding(t, vimMode.Actions, "vim-scroll-left", []string{"h"})
	requireActionBinding(t, vimMode.Actions, "vim-scroll-down", []string{"j"})
	requireActionBinding(t, vimMode.Actions, "vim-scroll-up", []string{"k"})
	requireActionBinding(t, vimMode.Actions, "vim-scroll-right", []string{"l"})
	requireActionBinding(t, vimMode.Actions, "vim-scroll-down-fast", []string{"shift+j"})
	requireActionBinding(t, vimMode.Actions, "vim-scroll-up-fast", []string{"shift+k"})
	requireActionBinding(t, vimMode.Actions, "confirm", []string{"enter"})
	requireActionBinding(t, vimMode.Actions, "cancel", []string{"escape"})
}

func TestVimModeConfig_GetKeyBindings(t *testing.T) {
	cfg := DefaultConfig()
	bindings := cfg.Workspace.VimMode.GetKeyBindings()

	assert.Equal(t, "vim-scroll-left", bindings["h"])
	assert.Equal(t, "vim-scroll-down", bindings["j"])
	assert.Equal(t, "vim-scroll-up", bindings["k"])
	assert.Equal(t, "vim-scroll-right", bindings["l"])
	assert.Equal(t, "vim-scroll-down-fast", bindings["shift+j"])
	assert.Equal(t, "vim-scroll-up-fast", bindings["shift+k"])
	assert.Equal(t, "confirm", bindings["enter"])
	assert.Equal(t, "cancel", bindings["escape"])
	assert.Len(t, bindings, 8)
}

func TestValidateVimMode_NonNegativeTimeout(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Config)
		want   string
	}{
		{
			name: "negative timeout fails",
			mutate: func(cfg *Config) {
				cfg.Workspace.VimMode.TimeoutMilliseconds = -1
			},
			want: "workspace.vim_mode.timeout_ms must be non-negative",
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
				cfg.Workspace.VimMode.TimeoutMilliseconds = 5000
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

func TestValidateVimMode_NilActionsFail(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Workspace.VimMode.Actions = nil

	err := validateConfig(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "workspace.vim_mode.actions cannot be empty")
}

func TestValidateVimMode_DuplicateKeyBindings(t *testing.T) {
	cfg := DefaultConfig()
	// Add a duplicate key binding that conflicts with an existing one.
	action := "custom-action"
	cfg.Workspace.VimMode.Actions[action] = ActionBinding{
		Keys: []string{"h"}, // "h" is already used by vim-scroll-left
		Desc: "Custom action",
	}

	err := validateConfig(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate key binding 'h' found in vim_mode actions")
	assert.Contains(t, err.Error(), "vim-scroll-left")
	assert.Contains(t, err.Error(), action)
}

func TestValidateVimMode_EmptyActionKeysFail(t *testing.T) {
	cfg := DefaultConfig()
	// Override an action with no key bindings.
	cfg.Workspace.VimMode.Actions = map[string]ActionBinding{
		"vim-scroll-left": {Keys: []string{}, Desc: "No keys"},
	}

	err := validateConfig(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "workspace.vim_mode.actions.vim-scroll-left must have at least one key binding")
}

func TestValidateModalModeActions_RegressionTable(t *testing.T) {
	tests := []struct {
		name     string
		modePath string
		timeout  int
		actions  map[string]ActionBinding
		want     string
	}{
		{
			name:     "page negative timeout",
			modePath: "vim_mode",
			timeout:  -1,
			actions:  map[string]ActionBinding{"vim-scroll-left": {Keys: []string{"h"}}},
			want:     "workspace.vim_mode.timeout_ms must be non-negative",
		},
		{
			name:     "pane empty actions",
			modePath: "pane_mode",
			timeout:  0,
			actions:  nil,
			want:     "workspace.pane_mode.actions cannot be empty",
		},
		{
			name:     "tab empty keys",
			modePath: "tab_mode",
			timeout:  0,
			actions:  map[string]ActionBinding{"new-tab": {Keys: []string{}}},
			want:     "workspace.tab_mode.actions.new-tab must have at least one key binding",
		},
		{
			name:     "page duplicate vim binding",
			modePath: "vim_mode",
			timeout:  0,
			actions: map[string]ActionBinding{
				"vim-scroll-left":  {Keys: []string{"h"}},
				"vim-scroll-right": {Keys: []string{"h"}},
			},
			want: "duplicate key binding 'h' found in vim_mode actions",
		},
		{
			name:     "page canonical vim set ok",
			modePath: "vim_mode",
			timeout:  0,
			actions: map[string]ActionBinding{
				"vim-scroll-left":      {Keys: []string{"h"}},
				"vim-scroll-down":      {Keys: []string{"j"}},
				"vim-scroll-up":        {Keys: []string{"k"}},
				"vim-scroll-right":     {Keys: []string{"l"}},
				"vim-scroll-down-fast": {Keys: []string{"shift+j"}},
				"vim-scroll-up-fast":   {Keys: []string{"shift+k"}},
				"confirm":              {Keys: []string{"enter"}},
				"cancel":               {Keys: []string{"escape"}},
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := validateModalModeActions(tt.modePath, tt.timeout, tt.actions)
			if tt.want == "" {
				assert.Empty(t, errs)
				return
			}
			require.NotEmpty(t, errs)
			assert.Contains(t, errs[0], tt.want)
		})
	}
}

func TestSchemaProvider_VimModeKeys(t *testing.T) {
	provider := &SchemaProvider{}
	schema := provider.GetSchema()

	var foundActivation, foundTimeout, foundActions bool
	for _, key := range schema {
		switch key.Key {
		case "workspace.vim_mode.activation_shortcut":
			foundActivation = true
			assert.Equal(t, "string", key.Type)
			assert.Equal(t, "ctrl+y", key.Default)
			assert.Equal(t, SectionWorkspace, key.Section)
		case "workspace.vim_mode.timeout_ms":
			foundTimeout = true
			assert.Equal(t, "int", key.Type)
			assert.Equal(t, "0", key.Default)
			assert.Equal(t, ">=0", key.Range)
			assert.Equal(t, SectionWorkspace, key.Section)
		case "workspace.vim_mode.actions.<action>":
			foundActions = true
			assert.Equal(t, "[]string", key.Type)
			assert.Equal(t, SectionWorkspace, key.Section)
		}
	}

	assert.True(t, foundActivation, "missing workspace.vim_mode.activation_shortcut schema key")
	assert.True(t, foundTimeout, "missing workspace.vim_mode.timeout_ms schema key")
	assert.True(t, foundActions, "missing workspace.vim_mode.actions.<action> schema key")
}
