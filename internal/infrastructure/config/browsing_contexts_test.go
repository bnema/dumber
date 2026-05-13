package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Legacy popups → browsing_contexts transform tests ---

func TestTransformLegacyPopupsToBrowsingContexts_BasicMapping(t *testing.T) {
	transformer := NewLegacyConfigTransformer()

	rawConfig := map[string]any{
		"workspace": map[string]any{
			"popups": map[string]any{
				"behavior":         "tabbed",
				"oauth_auto_close": false,
			},
		},
	}

	transformer.TransformLegacyPopupsToBrowsingContexts(rawConfig)

	ws := rawConfig["workspace"].(map[string]any)
	// Old key should be deleted.
	_, hasOld := ws["popups"]
	assert.False(t, hasOld, "popups key should be removed after transform")

	// New key should contain the old values.
	newCfg, hasNew := ws["browsing_contexts"].(map[string]any)
	require.True(t, hasNew, "browsing_contexts key should exist")
	assert.Equal(t, "tabbed", newCfg["behavior"])
	assert.Equal(t, false, newCfg["oauth_auto_close"])
}

func TestTransformLegacyPopupsToBrowsingContexts_NewKeyAlreadyPresent(t *testing.T) {
	transformer := NewLegacyConfigTransformer()

	rawConfig := map[string]any{
		"workspace": map[string]any{
			"popups": map[string]any{
				"behavior": "tabbed",
			},
			"browsing_contexts": map[string]any{
				"behavior": "split",
			},
		},
	}

	transformer.TransformLegacyPopupsToBrowsingContexts(rawConfig)

	ws := rawConfig["workspace"].(map[string]any)
	// Old key should still exist (new key was already present, skip transform).
	oldCfg, hasOld := ws["popups"].(map[string]any)
	assert.True(t, hasOld, "popups key should remain when browsing_contexts already exists")
	assert.Equal(t, "tabbed", oldCfg["behavior"])

	// New key should NOT be overwritten.
	newCfg, hasNew := ws["browsing_contexts"].(map[string]any)
	require.True(t, hasNew, "browsing_contexts key should still exist")
	assert.Equal(t, "split", newCfg["behavior"], "browsing_contexts should not be overwritten by legacy transform")
}

func TestTransformLegacyPopupsToBrowsingContexts_NoWorkspace(t *testing.T) {
	transformer := NewLegacyConfigTransformer()
	rawConfig := map[string]any{"other": "value"}
	// Should not panic
	transformer.TransformLegacyPopupsToBrowsingContexts(rawConfig)
	assert.NotNil(t, rawConfig)
}

func TestTransformLegacyPopupsToBrowsingContexts_NoPopupsKey(t *testing.T) {
	transformer := NewLegacyConfigTransformer()
	rawConfig := map[string]any{
		"workspace": map[string]any{
			"new_pane_url": "about:blank",
		},
	}
	transformer.TransformLegacyPopupsToBrowsingContexts(rawConfig)
	ws := rawConfig["workspace"].(map[string]any)
	assert.Equal(t, "about:blank", ws["new_pane_url"])
	_, hasBC := ws["browsing_contexts"]
	assert.False(t, hasBC, "browsing_contexts should not be created if popups absent")
}

// --- Default config writing emits browsing_contexts instead of popups ---

func TestDefaultConfig_TOMLHasBrowsingContextsNotPopups(t *testing.T) {
	cfg := DefaultConfig()

	content, err := encodeConfigToTOML(cfg)
	require.NoError(t, err)

	// Should contain [workspace.browsing_contexts] section.
	assert.Contains(t, content, "[workspace.browsing_contexts]",
		"TOML output should contain browsing_contexts section")

	// Should NOT contain [workspace.popups] section.
	assert.NotContains(t, content, "[workspace.popups]",
		"TOML output should NOT contain deprecated popups section")

	// The section should have the expected keys.
	assert.Contains(t, content, "behavior = ")
	assert.Contains(t, content, "oauth_auto_close = ")
}

// --- Loader defaults expose browsing_contexts keys, NOT popups ---

func TestSetWorkspaceDefaults_ExposesBrowsingContextsNotPopups(t *testing.T) {
	mgr := &Manager{viper: viper.New()}
	mgr.setDefaults()

	// browsing_contexts defaults should be registered.
	assert.Equal(t, "split", mgr.viper.GetString("workspace.browsing_contexts.behavior"))
	assert.Equal(t, "right", mgr.viper.GetString("workspace.browsing_contexts.placement"))
	assert.Equal(t, "stacked", mgr.viper.GetString("workspace.browsing_contexts.blank_target_behavior"))
	assert.True(t, mgr.viper.GetBool("workspace.browsing_contexts.open_in_new_pane"))
	assert.True(t, mgr.viper.GetBool("workspace.browsing_contexts.follow_pane_context"))
	assert.True(t, mgr.viper.GetBool("workspace.browsing_contexts.enable_smart_detection"))
	assert.True(t, mgr.viper.GetBool("workspace.browsing_contexts.oauth_auto_close"))

	// popups defaults should NOT be registered.
	assert.Empty(t, mgr.viper.GetString("workspace.popups.behavior"))
	assert.Empty(t, mgr.viper.GetString("workspace.popups.placement"))
	assert.Empty(t, mgr.viper.GetString("workspace.popups.blank_target_behavior"))
	assert.False(t, mgr.viper.GetBool("workspace.popups.open_in_new_pane"))
	assert.False(t, mgr.viper.GetBool("workspace.popups.follow_pane_context"))
	assert.False(t, mgr.viper.GetBool("workspace.popups.enable_smart_detection"))
	assert.False(t, mgr.viper.GetBool("workspace.popups.oauth_auto_close"))
}

func TestBindBrowsingContextEnvAliases_AcceptsLegacyPopupEnvNames(t *testing.T) {
	t.Setenv("DUMBER_WORKSPACE_POPUPS_BEHAVIOR", "tabbed")

	v := viper.New()
	v.SetEnvPrefix("DUMBER")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()
	v.SetDefault("workspace.browsing_contexts.behavior", "split")
	require.NoError(t, bindBrowsingContextEnvAliases(v))

	assert.Equal(t, "tabbed", v.GetString("workspace.browsing_contexts.behavior"))
}

// --- Validation error paths mention workspace.browsing_contexts, not workspace.popups ---

func TestValidatePopups_ErrorMessagesUseBrowsingContexts(t *testing.T) {
	tests := []struct {
		name     string
		mutate   func(*Config)
		wantText string
	}{
		{
			name: "invalid behavior",
			mutate: func(cfg *Config) {
				cfg.Workspace.BrowsingContexts.Behavior = PopupBehavior("bad")
			},
			wantText: "workspace.browsing_contexts.behavior",
		},
		{
			name: "invalid placement",
			mutate: func(cfg *Config) {
				cfg.Workspace.BrowsingContexts.Behavior = PopupBehaviorSplit
				cfg.Workspace.BrowsingContexts.Placement = "center"
			},
			wantText: "workspace.browsing_contexts.placement",
		},
		{
			name: "invalid blank_target_behavior",
			mutate: func(cfg *Config) {
				cfg.Workspace.BrowsingContexts.BlankTargetBehavior = "popup"
			},
			wantText: "workspace.browsing_contexts.blank_target_behavior",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			// Sync Popups alias so both fields match (validation reads BrowsingContexts).
			cfg.Workspace.Popups = cfg.Workspace.BrowsingContexts
			tt.mutate(cfg)
			cfg.Workspace.Popups = cfg.Workspace.BrowsingContexts

			err := validateConfig(cfg)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantText,
				"validation error should reference browsing_contexts, not popups")

			// Ensure it does NOT reference the old popups nomenclature.
			assert.NotContains(t, err.Error(), "workspace.popups.",
				"validation error should NOT reference deprecated workspace.popups")
		})
	}
}

func TestValidatePopups_CompatibilityAliasMirrorsBrowsingContexts(t *testing.T) {
	// When BrowsingContexts is valid but Popups alias is stale,
	// validation should still pass because it reads BrowsingContexts.
	cfg := DefaultConfig()
	// Deliberately make Popups stale.
	cfg.Workspace.Popups.Behavior = PopupBehavior("invalid")
	cfg.Workspace.Popups.OAuthAutoClose = !cfg.Workspace.BrowsingContexts.OAuthAutoClose

	err := validateConfig(cfg)
	require.NoError(t, err, "validation should read BrowsingContexts, not the stale Popups alias")
}

// --- Runtime alias sync (BrowsingContexts → Popups) ---

func TestNormalizeBrowsingContexts_SyncsPopupsAlias(t *testing.T) {
	cfg := DefaultConfig()

	// Make BrowsingContexts and Popups diverge.
	cfg.Workspace.BrowsingContexts.Behavior = PopupBehaviorTabbed
	cfg.Workspace.Popups.Behavior = PopupBehaviorSplit

	normalizeBrowsingContexts(cfg)

	assert.Equal(t, PopupBehaviorTabbed, cfg.Workspace.Popups.Behavior,
		"normalizeConfig should sync Popups ← BrowsingContexts")
	assert.Equal(t, cfg.Workspace.BrowsingContexts.Placement, cfg.Workspace.Popups.Placement)
	assert.Equal(t, cfg.Workspace.BrowsingContexts.OAuthAutoClose, cfg.Workspace.Popups.OAuthAutoClose)
	assert.Equal(t, cfg.Workspace.BrowsingContexts.OpenInNewPane, cfg.Workspace.Popups.OpenInNewPane)
}

func TestDefaultConfig_PopupsMirrorsBrowsingContexts(t *testing.T) {
	cfg := DefaultConfig()

	// After DefaultConfig(), both fields should be identical.
	assert.Equal(t, cfg.Workspace.BrowsingContexts.Behavior, cfg.Workspace.Popups.Behavior)
	assert.Equal(t, cfg.Workspace.BrowsingContexts.Placement, cfg.Workspace.Popups.Placement)
	assert.Equal(t, cfg.Workspace.BrowsingContexts.BlankTargetBehavior, cfg.Workspace.Popups.BlankTargetBehavior)
	assert.Equal(t, cfg.Workspace.BrowsingContexts.OpenInNewPane, cfg.Workspace.Popups.OpenInNewPane)
	assert.Equal(t, cfg.Workspace.BrowsingContexts.FollowPaneContext, cfg.Workspace.Popups.FollowPaneContext)
	assert.Equal(t, cfg.Workspace.BrowsingContexts.EnableSmartDetection, cfg.Workspace.Popups.EnableSmartDetection)
	assert.Equal(t, cfg.Workspace.BrowsingContexts.OAuthAutoClose, cfg.Workspace.Popups.OAuthAutoClose)
}

func TestSchemaProvider_UsesBrowsingContextsKeys(t *testing.T) {
	provider := NewSchemaProvider()
	schema := provider.GetSchema()

	foundBrowsingContexts := false
	foundPopups := false

	for _, key := range schema {
		if key.Key == "workspace.browsing_contexts.behavior" {
			foundBrowsingContexts = true
		}
		if key.Key == "workspace.popups.behavior" {
			foundPopups = true
		}
	}

	assert.True(t, foundBrowsingContexts, "schema should include workspace.browsing_contexts.* keys")
	assert.False(t, foundPopups, "schema should NOT include workspace.popups.* keys")
}

// --- End-to-end: legacy config round-trips through viper ---

func TestLegacyPopupsFullRoundTrip(t *testing.T) {
	// Simulate reading an old config file with [workspace.popups].
	m := &Manager{viper: viper.New()}
	m.viper.Set("workspace.popups.behavior", "stacked")
	m.viper.Set("workspace.popups.placement", "left")
	m.viper.Set("workspace.popups.oauth_auto_close", false)

	// Transform should map popups → browsing_contexts.
	m.transformLegacyConfig()

	// popups key should be gone from viper after transform.
	assert.False(t, m.viper.IsSet("workspace.popups.behavior"),
		"workspace.popups.behavior should not exist after transform")

	// browsing_contexts should now hold the values.
	assert.Equal(t, "stacked", m.viper.GetString("workspace.browsing_contexts.behavior"))
	assert.Equal(t, "left", m.viper.GetString("workspace.browsing_contexts.placement"))
	assert.False(t, m.viper.GetBool("workspace.browsing_contexts.oauth_auto_close"))

	// Unmarshal should succeed.
	cfg := &Config{}
	err := m.viper.Unmarshal(cfg)
	require.NoError(t, err)

	// BrowsingContexts should have the legacy values.
	assert.Equal(t, PopupBehaviorStacked, cfg.Workspace.BrowsingContexts.Behavior)
	assert.Equal(t, "left", cfg.Workspace.BrowsingContexts.Placement)
	assert.False(t, cfg.Workspace.BrowsingContexts.OAuthAutoClose)

	// Popups should be empty (not unmarshaled, will be synced in normalizeConfig).
	assert.Equal(t, PopupBehavior(""), cfg.Workspace.Popups.Behavior)

	// After normalization, Popups should mirror BrowsingContexts.
	normalizeBrowsingContexts(cfg)
	assert.Equal(t, PopupBehaviorStacked, cfg.Workspace.Popups.Behavior)
	assert.Equal(t, "left", cfg.Workspace.Popups.Placement)
	assert.False(t, cfg.Workspace.Popups.OAuthAutoClose)
}

func TestLegacyPopupsFileOverridesBrowsingContextDefaults(t *testing.T) {
	configFile := filepath.Join(t.TempDir(), "config.toml")
	content := `
[workspace.popups]
behavior = "stacked"
placement = "left"
oauth_auto_close = false
`
	require.NoError(t, os.WriteFile(configFile, []byte(content), 0o644))

	m := &Manager{viper: viper.New()}
	m.viper.SetConfigFile(configFile)
	m.setDefaults()
	require.NoError(t, m.viper.ReadInConfig())

	m.transformLegacyConfig()

	assert.Equal(t, "stacked", m.viper.GetString("workspace.browsing_contexts.behavior"))
	assert.Equal(t, "left", m.viper.GetString("workspace.browsing_contexts.placement"))
	assert.False(t, m.viper.GetBool("workspace.browsing_contexts.oauth_auto_close"))

	cfg := &Config{}
	require.NoError(t, m.viper.Unmarshal(cfg))
	normalizeConfig(cfg)

	assert.Equal(t, PopupBehaviorStacked, cfg.Workspace.BrowsingContexts.Behavior)
	assert.Equal(t, "left", cfg.Workspace.BrowsingContexts.Placement)
	assert.False(t, cfg.Workspace.BrowsingContexts.OAuthAutoClose)
	assert.Equal(t, cfg.Workspace.BrowsingContexts, cfg.Workspace.Popups)
}

func TestLegacyPopupsFilePreservesCanonicalBrowsingContextsEnvOverride(t *testing.T) {
	configFile := filepath.Join(t.TempDir(), "config.toml")
	content := `
[workspace.popups]
behavior = "stacked"
placement = "left"
`
	require.NoError(t, os.WriteFile(configFile, []byte(content), 0o644))
	t.Setenv("DUMBER_WORKSPACE_BROWSING_CONTEXTS_BEHAVIOR", "tabbed")

	m := &Manager{viper: viper.New()}
	m.viper.SetConfigType("toml")
	m.viper.SetConfigFile(configFile)
	m.viper.SetEnvPrefix("DUMBER")
	m.viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	m.viper.AutomaticEnv()
	m.setDefaults()
	require.NoError(t, m.viper.ReadInConfig())

	m.transformLegacyConfig()

	assert.Equal(t, "tabbed", m.viper.GetString("workspace.browsing_contexts.behavior"))
	assert.Equal(t, "left", m.viper.GetString("workspace.browsing_contexts.placement"))
}

func TestReload_ReappliesLegacyPopupsCompatibility(t *testing.T) {
	configFile := filepath.Join(t.TempDir(), "config.toml")
	writeConfig := func(content string) {
		require.NoError(t, os.WriteFile(configFile, []byte(content), 0o644))
	}

	m := &Manager{viper: viper.New()}
	m.viper.SetConfigType("toml")
	m.viper.SetConfigFile(configFile)
	m.viper.SetEnvPrefix("DUMBER")
	m.viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	m.viper.AutomaticEnv()
	m.setDefaults()

	writeConfig(`
[workspace.popups]
behavior = "stacked"
placement = "left"
`)
	require.NoError(t, m.reload())
	require.Equal(t, PopupBehaviorStacked, m.config.Workspace.BrowsingContexts.Behavior)
	require.Equal(t, "left", m.config.Workspace.BrowsingContexts.Placement)

	writeConfig(`
[workspace.popups]
behavior = "tabbed"
placement = "bottom"
`)
	require.NoError(t, m.reload())
	assert.Equal(t, PopupBehaviorTabbed, m.config.Workspace.BrowsingContexts.Behavior)
	assert.Equal(t, "bottom", m.config.Workspace.BrowsingContexts.Placement)
	assert.Equal(t, m.config.Workspace.BrowsingContexts, m.config.Workspace.Popups)
}

func TestReload_MixedLegacyAndCanonicalBrowsingContexts_BackfillsMissingCanonicalFields(t *testing.T) {
	configFile := filepath.Join(t.TempDir(), "config.toml")
	require.NoError(t, os.WriteFile(configFile, []byte(`
[workspace.popups]
placement = "left"
oauth_auto_close = false

[workspace.browsing_contexts]
behavior = "tabbed"
`), 0o644))

	m := &Manager{viper: viper.New()}
	m.viper.SetConfigType("toml")
	m.viper.SetConfigFile(configFile)
	m.viper.SetEnvPrefix("DUMBER")
	m.viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	m.viper.AutomaticEnv()
	m.setDefaults()

	require.NoError(t, m.reload())
	assert.Equal(t, PopupBehaviorTabbed, m.config.Workspace.BrowsingContexts.Behavior)
	assert.Equal(t, "left", m.config.Workspace.BrowsingContexts.Placement)
	assert.False(t, m.config.Workspace.BrowsingContexts.OAuthAutoClose)
	assert.Equal(t, m.config.Workspace.BrowsingContexts, m.config.Workspace.Popups)
}

func TestSave_WhenWatching_SyncsPopupsAliasBeforeSkipReload(t *testing.T) {
	configFile := filepath.Join(t.TempDir(), "config.toml")
	m := &Manager{viper: viper.New(), watching: true}
	m.viper.SetConfigType("toml")
	m.viper.SetConfigFile(configFile)
	m.setDefaults()
	m.config = DefaultConfig()

	cfg := DefaultConfig()
	cfg.Workspace.BrowsingContexts.Behavior = PopupBehaviorTabbed
	cfg.Workspace.BrowsingContexts.Placement = "bottom"
	cfg.Workspace.Popups.Behavior = PopupBehaviorSplit
	cfg.Workspace.Popups.Placement = "left"

	require.NoError(t, m.Save(cfg))
	assert.True(t, m.skipNextReload)
	assert.Equal(t, cfg.Workspace.BrowsingContexts.Behavior, m.config.Workspace.BrowsingContexts.Behavior)
	assert.Equal(t, cfg.Workspace.BrowsingContexts.Placement, m.config.Workspace.BrowsingContexts.Placement)
	assert.Equal(t, m.config.Workspace.BrowsingContexts, m.config.Workspace.Popups)
}
