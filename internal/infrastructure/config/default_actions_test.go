package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func loadConfigFromTOML(t *testing.T, content string) *Config {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, ".local", "state"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))

	configFile, err := GetConfigFile()
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(configFile), 0o755))
	require.NoError(t, os.WriteFile(configFile, []byte(content), 0o644))

	mgr, err := NewManager()
	require.NoError(t, err)
	require.NoError(t, mgr.Load())
	return mgr.Get()
}

// unusedKey returns a valid key sequence that no default action in the map uses.
func unusedKey(spec actionMapSpec) string {
	if spec.canonKey != nil {
		return "<C-x>"
	}
	return "ctrl+alt+f12"
}

func TestActionMapSpecs_CoverEveryActionMap(t *testing.T) {
	cfg := DefaultConfig()
	wantKeys := []string{
		"workspace.shortcuts.actions",
		"workspace.prefix_mode.actions",
		"workspace.pane_mode.actions",
		"workspace.tab_mode.actions",
		"workspace.vim_mode.actions",
		"workspace.resize_mode.actions",
		"session.session_mode.actions",
	}
	var gotKeys []string
	for _, spec := range actionMapSpecs() {
		gotKeys = append(gotKeys, spec.key)
		assert.NotEmpty(t, *spec.actions(cfg), spec.key)
	}
	assert.ElementsMatch(t, wantKeys, gotKeys)
}

func TestCompleteDefaultActions_AllMaps(t *testing.T) {
	for _, spec := range actionMapSpecs() {
		t.Run(spec.key, func(t *testing.T) {
			defaults := DefaultConfig()
			defaultActions := *spec.actions(defaults)

			// Partial user map: a single custom action on an unused key.
			cfg := DefaultConfig()
			*spec.actions(cfg) = map[string]ActionBinding{
				"custom-user-action": {Keys: []string{unusedKey(spec)}, Desc: "custom"},
			}
			completeDefaultActions(cfg)

			got := *spec.actions(cfg)
			assert.Contains(t, got, "custom-user-action")
			assert.Len(t, got, len(defaultActions)+1)
			for name, binding := range defaultActions {
				assert.Equal(t, binding, got[name], name)
			}
			require.NoError(t, validateConfig(cfg))
		})
	}
}

func TestCompleteDefaultActions_UserOverrideWins(t *testing.T) {
	for _, spec := range actionMapSpecs() {
		t.Run(spec.key, func(t *testing.T) {
			defaults := DefaultConfig()
			var name string
			for n, b := range *spec.actions(defaults) {
				if len(b.Keys) > 0 {
					name = n
					break
				}
			}
			require.NotEmpty(t, name)

			cfg := DefaultConfig()
			*spec.actions(cfg) = map[string]ActionBinding{
				name: {Keys: []string{unusedKey(spec)}, Desc: "mine"},
			}
			completeDefaultActions(cfg)

			got := (*spec.actions(cfg))[name]
			assert.Equal(t, []string{unusedKey(spec)}, got.Keys)
			assert.Equal(t, "mine", got.Desc)
		})
	}
}

func TestCompleteDefaultActions_SkipsDefaultsConflictingWithUserKeys(t *testing.T) {
	cfg := DefaultConfig()
	// The user key canonicalizes to the same vim sequence as the default "shift+j"
	// key of vim-scroll-down-fast; the user binds it to a custom action.
	cfg.Workspace.VimMode.Actions = map[string]ActionBinding{
		"my-action": {Keys: []string{"shift+j"}, Desc: "mine"},
	}
	completeDefaultActions(cfg)

	got := cfg.Workspace.VimMode.Actions
	assert.Contains(t, got, "my-action")
	assert.NotContains(t, got, "vim-scroll-down-fast")
	assert.Contains(t, got, "vim-scroll-up-fast")
	require.NoError(t, validateConfig(cfg))
}

func TestCompleteDefaultActions_DoesNotMutateSharedMaps(t *testing.T) {
	cfg := DefaultConfig()
	original := map[string]ActionBinding{"custom": {Keys: []string{"ctrl+alt+f12"}}}
	cfg.Workspace.TabMode.Actions = original
	completeDefaultActions(cfg)
	assert.Len(t, original, 1)
	assert.Greater(t, len(cfg.Workspace.TabMode.Actions), 1)
}

func TestManagerLoad_PartialVimActionsGetMergedDefaults(t *testing.T) {
	cfg := loadConfigFromTOML(t, `
[workspace.vim_mode.actions.cancel]
keys = ["escape"]
desc = "Cancel"
`)
	defaults := DefaultConfig().Workspace.VimMode.Actions
	assert.Len(t, cfg.Workspace.VimMode.Actions, len(defaults))
	for name := range defaults {
		assert.Contains(t, cfg.Workspace.VimMode.Actions, name)
	}
}

func TestManagerLoad_UserKeysOverrideDefaultAndConflictingDefaultSkipped(t *testing.T) {
	// Rebind scroll-left to "j", which is the default key of scroll-down.
	cfg := loadConfigFromTOML(t, `
[workspace.vim_mode.actions.vim-scroll-left]
keys = ["j"]
desc = "Mine"
`)
	actions := cfg.Workspace.VimMode.Actions
	assert.Equal(t, []string{"j"}, actions["vim-scroll-left"].Keys)
	assert.NotContains(t, actions, "vim-scroll-down")
	assert.Contains(t, actions, "vim-scroll-up")
}

func TestManagerLoad_EmptyKeysDisablesActionAndIsNotRefilled(t *testing.T) {
	cfg := loadConfigFromTOML(t, `
[workspace.vim_mode.actions.vim-scroll-left]
keys = []
desc = "Disabled"

[workspace.pane_mode.actions.stack-pane]
keys = []
`)
	left, ok := cfg.Workspace.VimMode.Actions["vim-scroll-left"]
	require.True(t, ok)
	assert.Empty(t, left.Keys)
	assert.Equal(t, "Disabled", left.Desc)

	stack, ok := cfg.Workspace.PaneMode.Actions["stack-pane"]
	require.True(t, ok)
	assert.Empty(t, stack.Keys)

	// Other defaults are still completed.
	assert.Contains(t, cfg.Workspace.VimMode.Actions, "vim-scroll-right")
	assert.Contains(t, cfg.Workspace.PaneMode.Actions, "eject-pane-to-window")
}

func TestValidateModalModeActions_EmptyKeysAreDisabledNotInvalid(t *testing.T) {
	errs := validateModalModeActions("tab_mode", 0, map[string]ActionBinding{
		"new-tab": {Keys: []string{}},
		"other":   {Keys: nil},
	})
	assert.Empty(t, errs)
}

func TestMigrator_DisabledActionIsNotReportedMissing(t *testing.T) {
	m := NewMigrator()
	missing := m.detectMissingDefaultActions(map[string]any{
		"workspace.pane_mode.actions": map[string]any{
			"stack-pane": map[string]any{"keys": []any{}, "desc": "off"},
		},
	})
	assert.NotContains(t, missing, "workspace.pane_mode.actions.stack-pane")
	assert.Contains(t, missing, "workspace.pane_mode.actions.eject-pane-to-window")
}

func TestMigrator_MergeSkipsDefaultsConflictingWithUserKeys(t *testing.T) {
	m := NewMigrator()
	raw := map[string]any{
		"workspace": map[string]any{
			"vim_mode": map[string]any{
				"actions": map[string]any{
					"vim-scroll-left": map[string]any{"keys": []any{"j"}, "desc": "mine"},
				},
			},
		},
	}
	m.mergeMissingDefaultActions(raw)
	actions := m.getNestedValue(raw, "workspace.vim_mode.actions").(map[string]any)
	assert.NotContains(t, actions, "vim-scroll-down")
	assert.Contains(t, actions, "vim-scroll-up")
}

func TestManagerSave_KeepsDisabledActionsDisabledAcrossReload(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, ".local", "state"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))

	configFile, err := GetConfigFile()
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(configFile), 0o755))
	require.NoError(t, os.WriteFile(configFile, []byte(`
[workspace.vim_mode.actions.cancel]
keys = ["escape"]

[workspace.pane_mode.actions.stack-pane]
keys = []
`), 0o644))

	mgr, err := NewManager()
	require.NoError(t, err)
	require.NoError(t, mgr.Load())

	cfg := mgr.Get()
	require.NoError(t, mgr.Save(cfg))

	reloaded, err := NewManager()
	require.NoError(t, err)
	require.NoError(t, reloaded.Load())
	got := reloaded.Get()

	stack, ok := got.Workspace.PaneMode.Actions["stack-pane"]
	require.True(t, ok)
	assert.Empty(t, stack.Keys)
	assert.Len(t, got.Workspace.VimMode.Actions, len(DefaultConfig().Workspace.VimMode.Actions))
}

func TestManagerLoad_UnderscoreActionNamesAreCanonicalizedBeforeCompletion(t *testing.T) {
	cfg := loadConfigFromTOML(t, `
[workspace.pane_mode.actions.stack_pane]
keys = ["ctrl+alt+f12"]
desc = "Mine"

[workspace.resize_mode.actions.resize_increase_left]
keys = ["z"]

[workspace.prefix_mode.actions.enter_resize_mode]
keys = ["ctrl+alt+f11"]

[workspace.pane_mode.actions.focus_left]
keys = ["ctrl+alt+f10"]

[workspace.shortcuts.actions.toggle_floating_pane]
keys = ["alt+g"]
`)

	pane := cfg.Workspace.PaneMode.Actions
	assert.NotContains(t, pane, "stack_pane")
	assert.Equal(t, []string{"ctrl+alt+f12"}, pane["stack-pane"].Keys)
	assert.Equal(t, "Mine", pane["stack-pane"].Desc)
	assert.NotContains(t, pane, "focus_left")
	assert.Equal(t, []string{"ctrl+alt+f10"}, pane["focus-left"].Keys)

	resize := cfg.Workspace.ResizeMode.Actions
	assert.NotContains(t, resize, "resize_increase_left")
	assert.Equal(t, []string{"z"}, resize["resize-increase-left"].Keys)
	assert.Contains(t, resize, "resize-increase-right", "other defaults are still completed")

	prefix := cfg.Workspace.PrefixMode.Actions
	assert.NotContains(t, prefix, "enter_resize_mode")
	assert.Equal(t, []string{"ctrl+alt+f11"}, prefix["enter-resize-mode"].Keys)

	shortcuts := cfg.Workspace.Shortcuts.Actions
	assert.NotContains(t, shortcuts, "toggle_floating_pane")
	assert.Equal(t, []string{"alt+g"}, shortcuts["toggle-floating-pane"].Keys)

	for _, spec := range actionMapSpecs() {
		for name := range *spec.actions(cfg) {
			assert.NotContains(t, name, "_", "%s.%s", spec.key, name)
		}
	}
}

func TestManagerLoad_HyphenActionNameWinsOverUnderscoreDuplicate(t *testing.T) {
	cfg := loadConfigFromTOML(t, `
[workspace.pane_mode.actions.stack_pane]
keys = ["ctrl+alt+f12"]

[workspace.pane_mode.actions.stack-pane]
keys = ["ctrl+alt+f11"]
`)
	assert.Equal(t, []string{"ctrl+alt+f11"}, cfg.Workspace.PaneMode.Actions["stack-pane"].Keys)
	assert.NotContains(t, cfg.Workspace.PaneMode.Actions, "stack_pane")
}

func TestCompleteDefaultActions_AliasSpellingsOfAUserKeyBlockTheDefault(t *testing.T) {
	tests := []struct {
		name        string
		userKey     string
		wantSkipped string
	}{
		{name: "right vs arrowright", userKey: "right", wantSkipped: "split-right"},
		{name: "Right vs arrowright", userKey: "Right", wantSkipped: "split-right"},
		{name: "uppercase letter vs explicit shift", userKey: "shift+M", wantSkipped: "move-pane-to-next-tab"},
		{name: "explicit shift vs uppercase letter", userKey: "M", wantSkipped: "move-pane-to-next-tab"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.Workspace.PaneMode.Actions = map[string]ActionBinding{
				"close-pane": {Keys: []string{tt.userKey}},
			}
			completeDefaultActions(cfg)

			got := cfg.Workspace.PaneMode.Actions
			assert.Equal(t, []string{tt.userKey}, got["close-pane"].Keys)
			if tt.wantSkipped != "" {
				assert.NotContains(t, got, tt.wantSkipped)
			}
			require.NoError(t, validateConfig(cfg), "completed config must stay valid")
		})
	}
}

func TestCompleteDefaultActions_ControlSpellingBlocksGlobalDefault(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Workspace.Shortcuts.Actions = map[string]ActionBinding{
		"new-tab": {Keys: []string{"Control+r"}},
	}
	completeDefaultActions(cfg)

	// Default reload is ["ctrl+r", "f5"]; "Control+r" is the same key as
	// "ctrl+r", so the user owns it and reload is not added back.
	assert.NotContains(t, cfg.Workspace.Shortcuts.Actions, "reload")
	assert.Contains(t, cfg.Workspace.Shortcuts.Actions, "hard-reload")
	assert.Contains(t, cfg.Workspace.Shortcuts.Actions, "open-omnibox")
	require.NoError(t, validateConfig(cfg))
}

func TestManagerLoad_AliasConflictIsResolvedDeterministically(t *testing.T) {
	cfg := loadConfigFromTOML(t, `
[workspace.pane_mode.actions.close-pane]
keys = ["right"]
`)
	pane := cfg.Workspace.PaneMode.Actions
	assert.Equal(t, []string{"right"}, pane["close-pane"].Keys)
	assert.NotContains(t, pane, "split-right", "arrowright is the same key as right")
	assert.Contains(t, pane, "split-left")
}

func TestValidateModalModeActions_DetectsAliasDuplicates(t *testing.T) {
	tests := []struct {
		name      string
		keysA     string
		keysB     string
		wantCanon string
	}{
		{name: "right and arrowright", keysA: "right", keysB: "arrowright", wantCanon: "right"},
		{name: "control and ctrl", keysA: "control+t", keysB: "ctrl+t", wantCanon: "ctrl+t"},
		{name: "uppercase and explicit shift", keysA: "M", keysB: "shift+m", wantCanon: "shift+m"},
		{name: "modifier order and case", keysA: "Shift+Ctrl+T", keysB: "ctrl+shift+t", wantCanon: "ctrl+shift+t"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := validateModalModeActions("pane_mode", 0, map[string]ActionBinding{
				"action-a": {Keys: []string{tt.keysA}},
				"action-b": {Keys: []string{tt.keysB}},
			})
			require.Len(t, errs, 1)
			assert.Contains(t, errs[0], "duplicate key binding '"+tt.wantCanon+"' found in pane_mode")
			assert.Contains(t, errs[0], "action-a")
			assert.Contains(t, errs[0], "action-b")
		})
	}
}

func TestValidateModalModeActions_SpellingsOfOneKeyInOneActionAreNotDuplicates(t *testing.T) {
	// The defaults list "M" and "shift+m" together.
	errs := validateModalModeActions("pane_mode", 0, map[string]ActionBinding{
		"move-pane-to-next-tab": {Keys: []string{"M", "shift+m"}},
	})
	assert.Empty(t, errs)
}
