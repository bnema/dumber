package config

import (
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var legacyVimModeActions = []string{
	"vim-scroll-left",
	"vim-scroll-down",
	"vim-scroll-up",
	"vim-scroll-right",
	"vim-scroll-down-fast",
	"vim-scroll-up-fast",
	"confirm",
	"cancel",
}

var sequenceVimModeActions = map[string]string{
	"heading-next":   "]]",
	"heading-prev":   "[[",
	"code-next":      "]c",
	"code-prev":      "[c",
	"table-next":     "]t",
	"image-next":     "]i",
	"list-next":      "]l",
	"outline":        "gO",
	"yank-section":   "yah",
	"half-page-down": "<C-d>",
	"half-page-up":   "<C-u>",
}

func TestDefaultConfig_VimModeDefaults(t *testing.T) {
	cfg := DefaultConfig()

	// VimMode must exist in workspace config.
	vimMode := cfg.Workspace.VimMode
	assert.Equal(t, "ctrl+y", vimMode.ActivationShortcut)
	assert.Equal(t, 0, vimMode.TimeoutMilliseconds)
	assert.Equal(t, 500, vimMode.SequenceTimeoutMilliseconds)

	// All required actions must exist with exact key bindings.
	require.NotNil(t, vimMode.Actions)
	require.Len(t, legacyVimModeActions, 8)
	require.Len(t, vimMode.Actions, len(legacyVimModeActions)+len(sequenceVimModeActions))
	requireActionBinding(t, vimMode.Actions, "vim-scroll-left", []string{"h"})
	requireActionBinding(t, vimMode.Actions, "vim-scroll-down", []string{"j"})
	requireActionBinding(t, vimMode.Actions, "vim-scroll-up", []string{"k"})
	requireActionBinding(t, vimMode.Actions, "vim-scroll-right", []string{"l"})
	requireActionBinding(t, vimMode.Actions, "vim-scroll-down-fast", []string{"shift+j"})
	requireActionBinding(t, vimMode.Actions, "vim-scroll-up-fast", []string{"shift+k"})
	requireActionBinding(t, vimMode.Actions, "confirm", []string{"enter"})
	requireActionBinding(t, vimMode.Actions, "cancel", []string{"escape"})
	for _, action := range legacyVimModeActions {
		_, ok := vimMode.Actions[action]
		require.True(t, ok, "missing legacy vim mode action %q", action)
	}
}

func TestVimModeSequenceDefaults(t *testing.T) {
	cfg := DefaultConfig()
	vimMode := cfg.Workspace.VimMode

	assert.Equal(t, 500, vimMode.SequenceTimeoutMilliseconds)
	require.Len(t, sequenceVimModeActions, 11)

	for action, key := range sequenceVimModeActions {
		requireActionBinding(t, vimMode.Actions, action, []string{key})
	}
}

func TestVimModeConfig_DefaultsLoadThroughViper(t *testing.T) {
	mgr := &Manager{viper: viper.New()}
	mgr.setDefaults()

	cfg, err := mgr.unmarshalConfig()
	require.NoError(t, err)

	vimMode := cfg.Workspace.VimMode
	assert.Equal(t, "ctrl+y", vimMode.ActivationShortcut)
	assert.Equal(t, 0, vimMode.TimeoutMilliseconds)
	assert.Equal(t, 500, vimMode.SequenceTimeoutMilliseconds)
	require.Len(t, vimMode.Actions, len(legacyVimModeActions)+len(sequenceVimModeActions))
	requireActionBinding(t, vimMode.Actions, "confirm", []string{"enter"})
	requireActionBinding(t, vimMode.Actions, "cancel", []string{"escape"})
	requireActionBinding(t, vimMode.Actions, "half-page-down", []string{"<C-d>"})
	requireActionBinding(t, vimMode.Actions, "heading-next", []string{"]]"})
}

func TestDefaultConfig_VimModeDeepClone(t *testing.T) {
	first := DefaultConfig()
	second := DefaultConfig()

	first.Workspace.VimMode.SequenceTimeoutMilliseconds = 999
	first.Workspace.VimMode.Actions["confirm"] = ActionBinding{Keys: []string{"mutated"}, Desc: "mutated"}
	delete(first.Workspace.VimMode.Actions, "heading-next")

	assert.Equal(t, 500, second.Workspace.VimMode.SequenceTimeoutMilliseconds)
	requireActionBinding(t, second.Workspace.VimMode.Actions, "confirm", []string{"enter"})
	requireActionBinding(t, second.Workspace.VimMode.Actions, "heading-next", []string{"]]"})
	require.Len(t, second.Workspace.VimMode.Actions, len(legacyVimModeActions)+len(sequenceVimModeActions))
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
	assert.Equal(t, "heading-next", bindings["]]"])
	assert.Equal(t, "heading-prev", bindings["[["])
	assert.Equal(t, "code-next", bindings["]c"])
	assert.Equal(t, "code-prev", bindings["[c"])
	assert.Equal(t, "table-next", bindings["]t"])
	assert.Equal(t, "image-next", bindings["]i"])
	assert.Equal(t, "list-next", bindings["]l"])
	assert.Equal(t, "outline", bindings["gO"])
	assert.Equal(t, "yank-section", bindings["yah"])
	assert.Equal(t, "half-page-down", bindings["<C-d>"])
	assert.Equal(t, "half-page-up", bindings["<C-u>"])
	assert.Len(t, bindings, 19)
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

func TestValidateVimMode_SequenceTimeout(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Config)
		want   string
	}{
		{
			name: "negative sequence timeout fails",
			mutate: func(cfg *Config) {
				cfg.Workspace.VimMode.SequenceTimeoutMilliseconds = -1
			},
			want: "workspace.vim_mode.sequence_timeout_ms must be non-negative",
		},
		{
			name: "zero sequence timeout ok",
			mutate: func(cfg *Config) {
				cfg.Workspace.VimMode.SequenceTimeoutMilliseconds = 0
			},
			want: "",
		},
		{
			name: "positive sequence timeout ok",
			mutate: func(cfg *Config) {
				cfg.Workspace.VimMode.SequenceTimeoutMilliseconds = 750
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

func TestValidateVimMode_MalformedBindings(t *testing.T) {
	malformed := []string{"<C-d", "<Nope>", "<C->"}
	for _, key := range malformed {
		t.Run(key, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.Workspace.VimMode.Actions["broken"] = ActionBinding{
				Keys: []string{key},
				Desc: "Broken binding",
			}

			err := validateConfig(cfg)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "broken")
			assert.Contains(t, err.Error(), key)
		})
	}
}

func TestValidateVimMode_LegacyEnterEscapeRemainCompatible(t *testing.T) {
	cfg := DefaultConfig()
	requireActionBinding(t, cfg.Workspace.VimMode.Actions, "confirm", []string{"enter"})
	requireActionBinding(t, cfg.Workspace.VimMode.Actions, "cancel", []string{"escape"})
	require.NoError(t, validateConfig(cfg))

	assert.Equal(t, "<Return>", canonicalizeVimModeBinding("enter"))
	assert.Equal(t, "<Escape>", canonicalizeVimModeBinding("escape"))
	assert.Equal(t, "<C-d>", canonicalizeVimModeBinding("<C-d>"))
}

func TestValidateVimMode_CanonicalAliasConflicts(t *testing.T) {
	tests := []struct {
		name    string
		actionA string
		keysA   []string
		actionB string
		keysB   []string
		canon   string
	}{
		{
			name:    "CR and Return",
			actionA: "alias-a",
			keysA:   []string{"<CR>"},
			actionB: "alias-b",
			keysB:   []string{"<Return>"},
			canon:   "<Return>",
		},
		{
			name:    "enter and Return",
			actionA: "alias-a",
			keysA:   []string{"enter"},
			actionB: "alias-b",
			keysB:   []string{"<Return>"},
			canon:   "<Return>",
		},
		{
			name:    "escape and Escape",
			actionA: "alias-a",
			keysA:   []string{"escape"},
			actionB: "alias-b",
			keysB:   []string{"<Escape>"},
			canon:   "<Escape>",
		},
		{
			name:    "J and shift+j",
			actionA: "alias-a",
			keysA:   []string{"J"},
			actionB: "alias-b",
			keysB:   []string{"shift+j"},
			canon:   "J",
		},
		{
			name:    "C-d and ctrl+d",
			actionA: "alias-a",
			keysA:   []string{"<C-d>"},
			actionB: "alias-b",
			keysB:   []string{"ctrl+d"},
			canon:   "<C-d>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			// Drop defaults that would collide with the aliases under test.
			delete(cfg.Workspace.VimMode.Actions, "half-page-down")
			delete(cfg.Workspace.VimMode.Actions, "vim-scroll-down-fast")
			delete(cfg.Workspace.VimMode.Actions, "confirm")
			delete(cfg.Workspace.VimMode.Actions, "cancel")
			cfg.Workspace.VimMode.Actions[tt.actionA] = ActionBinding{Keys: tt.keysA, Desc: "alias a"}
			cfg.Workspace.VimMode.Actions[tt.actionB] = ActionBinding{Keys: tt.keysB, Desc: "alias b"}

			err := validateConfig(cfg)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "duplicate key binding '"+tt.canon+"' found in vim_mode actions")
			assert.Contains(t, err.Error(), tt.actionA)
			assert.Contains(t, err.Error(), tt.actionB)
		})
	}
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
				"confirm":               {Keys: []string{"enter"}},
				"cancel":                {Keys: []string{"escape"}},
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

func TestValidateVimMode_DefaultsAccept(t *testing.T) {
	cfg := DefaultConfig()
	require.NoError(t, validateConfig(cfg))
}

func TestSchemaProvider_VimModeKeys(t *testing.T) {
	provider := &SchemaProvider{}
	schema := provider.GetSchema()

	var foundActivation, foundTimeout, foundSequenceTimeout, foundActions bool
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
		case "workspace.vim_mode.sequence_timeout_ms":
			foundSequenceTimeout = true
			assert.Equal(t, "int", key.Type)
			assert.Equal(t, "500", key.Default)
			assert.Equal(t, ">=0", key.Range)
			assert.Equal(t, SectionWorkspace, key.Section)
		case "workspace.vim_mode.actions.<action>":
			foundActions = true
			assert.Equal(t, "object", key.Type)
			assert.Equal(t, SectionWorkspace, key.Section)
		}
	}

	assert.True(t, foundActivation, "missing workspace.vim_mode.activation_shortcut schema key")
	assert.True(t, foundTimeout, "missing workspace.vim_mode.timeout_ms schema key")
	assert.True(t, foundSequenceTimeout, "missing workspace.vim_mode.sequence_timeout_ms schema key")
	assert.True(t, foundActions, "missing workspace.vim_mode.actions.<action> schema key")
}

func TestSchemaExposesVimModeSequenceTimeout(t *testing.T) {
	provider := &SchemaProvider{}
	schema := provider.GetSchema()

	var found bool
	for _, key := range schema {
		if key.Key != "workspace.vim_mode.sequence_timeout_ms" {
			continue
		}
		found = true
		assert.Equal(t, "int", key.Type)
		assert.Equal(t, "500", key.Default)
		assert.Equal(t, ">=0", key.Range)
		assert.Equal(t, SectionWorkspace, key.Section)
	}
	assert.True(t, found, "schema must expose workspace.vim_mode.sequence_timeout_ms")
}
