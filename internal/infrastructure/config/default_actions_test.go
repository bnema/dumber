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

func TestCompleteDefaultActions_LegacyUnderscoreNameCountsAsConfigured(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Workspace.PaneMode.Actions = map[string]ActionBinding{
		"stack_pane": {Keys: []string{"ctrl+alt+f12"}},
	}
	completeDefaultActions(cfg)
	assert.NotContains(t, cfg.Workspace.PaneMode.Actions, "stack-pane")
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
