package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildSystemviewConfigPayload_EmptySearchShortcutsUsesEmptyMap(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SearchShortcuts = map[string]SearchShortcut{}
	payload := BuildSystemviewConfigPayload(cfg, nil)
	require.NotNil(t, payload.SearchShortcuts)
	require.Empty(t, payload.SearchShortcuts)
}
