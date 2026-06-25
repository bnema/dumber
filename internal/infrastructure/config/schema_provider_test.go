package config

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func TestConfigurationReferenceCoversSchemaKeys(t *testing.T) {
	docKeys := configurationReferenceKeys(t)

	var missing []string
	for _, key := range NewSchemaProvider().GetSchema() {
		if configurationReferenceCoversKey(docKeys, key.Key) {
			continue
		}
		missing = append(missing, key.Key)
	}

	sort.Strings(missing)
	require.Empty(t, missing, "docs/reference/configuration.md should cover every SchemaProvider key")
}

func configurationReferenceKeys(t *testing.T) map[string]struct{} {
	t.Helper()

	_, filename, _, ok := runtime.Caller(0)
	require.True(t, ok, "locate current test file")
	docPath := filepath.Join(filepath.Dir(filename), "..", "..", "..", "docs", "reference", "configuration.md")
	data, err := os.ReadFile(docPath)
	require.NoError(t, err)

	// Keep this split point aligned with the markdown heading so legacy-key rows
	// stay out of the canonical schema key coverage set.
	canonicalReference, _, _ := strings.Cut(string(data), "Legacy key migration:")
	rowRE := regexp.MustCompile(`(?m)^\| ` + "`" + `([^` + "`" + `]+)` + "`" + ` \|`)
	keys := make(map[string]struct{})
	for _, match := range rowRE.FindAllStringSubmatch(canonicalReference, -1) {
		keys[match[1]] = struct{}{}
	}
	return keys
}

func configurationReferenceCoversKey(docKeys map[string]struct{}, schemaKey string) bool {
	if _, ok := docKeys[schemaKey]; ok {
		return true
	}

	if strings.HasSuffix(schemaKey, ".*") {
		prefix := strings.TrimSuffix(schemaKey, "*")
		for docKey := range docKeys {
			if strings.HasPrefix(docKey, prefix) {
				return true
			}
		}
	}

	if idx := strings.Index(schemaKey, "<"); idx >= 0 {
		prefix := schemaKey[:idx]
		for docKey := range docKeys {
			if strings.HasPrefix(docKey, prefix) {
				return true
			}
		}
	}

	return false
}

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
