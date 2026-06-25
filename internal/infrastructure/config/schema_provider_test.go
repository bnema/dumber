package config

import (
	"sort"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func TestSchemaProviderCoversViperDefaults(t *testing.T) {
	m := &Manager{viper: viper.New()}
	m.setDefaults()

	schemaKeys := make(map[string]struct{})
	for _, key := range NewSchemaProvider().GetSchema() {
		require.NotEmpty(t, key.Key, "schema key should not be empty")
		require.NotContains(t, schemaKeys, key.Key, "duplicate schema key %s", key.Key)
		schemaKeys[key.Key] = struct{}{}
	}

	defaultsCoveredByAggregateSchemaKeys := map[string]string{
		"appearance.light_palette":         "appearance.light_palette.*",
		"appearance.dark_palette":          "appearance.dark_palette.*",
		"search_shortcuts":                 "search_shortcuts.<key>",
		"workspace.pane_mode.actions":      "workspace.pane_mode.actions.<action>",
		"workspace.tab_mode.actions":       "workspace.tab_mode.actions.<action>",
		"workspace.resize_mode.actions":    "workspace.resize_mode.actions.<action>",
		"workspace.floating_pane.profiles": "workspace.floating_pane.profiles.<name>",
		"session.session_mode.actions":     "session.session_mode.actions.<action>",
	}

	var missing []string
	for _, key := range m.viper.AllKeys() {
		if _, ok := schemaKeys[key]; ok {
			continue
		}
		if schemaKey, ok := defaultsCoveredByAggregateSchemaKeys[key]; ok {
			if _, hasAggregate := schemaKeys[schemaKey]; hasAggregate {
				continue
			}
		}
		missing = append(missing, key)
	}

	sort.Strings(missing)
	require.Empty(t, missing, "every Viper default should have SchemaProvider metadata or an explicit aggregate schema key")
}
