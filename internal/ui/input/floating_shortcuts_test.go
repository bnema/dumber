package input

import (
	"context"
	"testing"

	"github.com/bnema/dumber/internal/infrastructure/config"
	"github.com/jwijenbergh/puregotk/v4/gdk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShortcutSet_GlobalToggleFloatingPane(t *testing.T) {
	cfg := config.DefaultConfig()
	set := NewShortcutSet(context.Background(), cfg)

	action, ok := set.Lookup(KeyBinding{Keyval: uint(gdk.KEY_f), Modifiers: ModAlt}, ModeNormal)
	require.True(t, ok)
	assert.Equal(t, ActionToggleFloatingPane, action)
}

func TestShortcutSet_FloatingProfiles_RegisterMultiple(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Workspace.FloatingPane.Profiles = map[string]config.FloatingPaneProfile{
		"google": {
			Keys: []string{"alt+g"},
			URL:  "https://google.com",
		},
		"github": {
			Keys: []string{"alt+y"},
			URL:  "https://github.com",
		},
	}

	set := NewShortcutSet(context.Background(), cfg)

	googleAction, ok := set.Lookup(KeyBinding{Keyval: uint(gdk.KEY_g), Modifiers: ModAlt}, ModeNormal)
	require.True(t, ok)
	googleTarget, ok := ParseFloatingProfileTarget(googleAction)
	require.True(t, ok)
	assert.Equal(t, "google", googleTarget.SessionID)
	assert.Equal(t, "https://google.com", googleTarget.URL)

	githubAction, ok := set.Lookup(KeyBinding{Keyval: uint(gdk.KEY_y), Modifiers: ModAlt}, ModeNormal)
	require.True(t, ok)
	githubTarget, ok := ParseFloatingProfileTarget(githubAction)
	require.True(t, ok)
	assert.Equal(t, "github", githubTarget.SessionID)
	assert.Equal(t, "https://github.com", githubTarget.URL)
}

func TestCollectFloatingProfileShortcuts_SkipsConflicts(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Workspace.FloatingPane.Profiles = map[string]config.FloatingPaneProfile{
		"tab-conflict": {
			Keys: []string{"alt+1"},
			URL:  "https://example.com/tab",
		},
		"pane-conflict": {
			Keys: []string{"alt+["},
			URL:  "https://example.com/pane",
		},
		"ok": {
			Keys: []string{"alt+g"},
			URL:  "https://example.com/ok",
		},
	}

	occupied := map[KeyBinding]Action{
		{Keyval: uint(gdk.KEY_1), Modifiers: ModAlt}:           ActionSwitchTabIndex1,
		{Keyval: uint(gdk.KEY_bracketleft), Modifiers: ModAlt}: ActionConsumeOrExpelLeft,
	}

	shortcuts := collectFloatingProfileShortcuts(context.Background(), cfg, occupied)
	require.Len(t, shortcuts, 1)
	assert.Equal(t, KeyBinding{Keyval: uint(gdk.KEY_g), Modifiers: ModAlt}, shortcuts[0].Binding)
	target, ok := ParseFloatingProfileTarget(shortcuts[0].Action)
	require.True(t, ok)
	assert.Equal(t, "ok", target.SessionID)
	assert.Equal(t, "https://example.com/ok", target.URL)
}

func TestShortcutSet_FloatingProfilesPreserveExistingGlobalShortcuts(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Workspace.FloatingPane.Profiles = map[string]config.FloatingPaneProfile{
		"conflict": {
			Keys: []string{"alt+["},
			URL:  "https://example.com/override-attempt",
		},
	}

	set := NewShortcutSet(context.Background(), cfg)

	tabAction, ok := set.Lookup(KeyBinding{Keyval: uint(gdk.KEY_bracketleft), Modifiers: ModAlt}, ModeNormal)
	require.True(t, ok)
	assert.Equal(t, ActionConsumeOrExpelLeft, tabAction)

	omniboxAction, ok := set.Lookup(KeyBinding{Keyval: uint(gdk.KEY_l), Modifiers: ModCtrl}, ModeNormal)
	require.True(t, ok)
	assert.Equal(t, ActionOpenOmnibox, omniboxAction)
}

func TestShortcutSet_FloatingProfilesSkipGlobalOnlyConflicts(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Workspace.FloatingPane.Profiles = map[string]config.FloatingPaneProfile{
		"alt-one-conflict": {
			Keys: []string{"alt+1"},
			URL:  "https://example.com/one",
		},
		"alt-tab-conflict": {
			Keys: []string{"alt+tab"},
			URL:  "https://example.com/tab",
		},
		"ok": {
			Keys: []string{"alt+g"},
			URL:  "https://example.com/ok",
		},
	}

	set := NewShortcutSet(context.Background(), cfg)

	_, ok := set.Lookup(KeyBinding{Keyval: uint(gdk.KEY_1), Modifiers: ModAlt}, ModeNormal)
	assert.False(t, ok)

	_, ok = set.Lookup(KeyBinding{Keyval: uint(gdk.KEY_Tab), Modifiers: ModAlt}, ModeNormal)
	assert.False(t, ok)

	action, ok := set.Lookup(KeyBinding{Keyval: uint(gdk.KEY_g), Modifiers: ModAlt}, ModeNormal)
	require.True(t, ok)
	target, ok := ParseFloatingProfileTarget(action)
	require.True(t, ok)
	assert.Equal(t, "ok", target.SessionID)
	assert.Equal(t, "https://example.com/ok", target.URL)
}

func TestShortcutSet_FloatingProfilesWithSameURLGetDistinctSessionIDs(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Workspace.FloatingPane.Profiles = map[string]config.FloatingPaneProfile{
		"work-mail": {
			Keys: []string{"alt+w"},
			URL:  "https://mail.google.com",
		},
		"personal-mail": {
			Keys: []string{"alt+p"},
			URL:  "https://mail.google.com",
		},
	}

	set := NewShortcutSet(context.Background(), cfg)

	workAction, ok := set.Lookup(KeyBinding{Keyval: uint(gdk.KEY_w), Modifiers: ModAlt}, ModeNormal)
	require.True(t, ok)
	workTarget, ok := ParseFloatingProfileTarget(workAction)
	require.True(t, ok)

	personalAction, ok := set.Lookup(KeyBinding{Keyval: uint(gdk.KEY_p), Modifiers: ModAlt}, ModeNormal)
	require.True(t, ok)
	personalTarget, ok := ParseFloatingProfileTarget(personalAction)
	require.True(t, ok)

	assert.Equal(t, "https://mail.google.com", workTarget.URL)
	assert.Equal(t, "https://mail.google.com", personalTarget.URL)
	assert.NotEqual(t, workTarget.SessionID, personalTarget.SessionID)
}

func TestShortcutSet_FloatingProfiles_SupportModifierCombos(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Workspace.FloatingPane.Profiles = map[string]config.FloatingPaneProfile{
		"mail": {
			Keys: []string{"ctrl+alt+m"},
			URL:  "https://mail.example.com",
		},
		"search": {
			Keys: []string{"ctrl+shift+y"},
			URL:  "https://search.example.com",
		},
	}

	set := NewShortcutSet(context.Background(), cfg)

	mailAction, ok := set.Lookup(KeyBinding{Keyval: uint(gdk.KEY_m), Modifiers: ModCtrl | ModAlt}, ModeNormal)
	require.True(t, ok)
	mailTarget, ok := ParseFloatingProfileTarget(mailAction)
	require.True(t, ok)
	assert.Equal(t, "mail", mailTarget.SessionID)
	assert.Equal(t, "https://mail.example.com", mailTarget.URL)

	searchAction, ok := set.Lookup(KeyBinding{Keyval: uint(gdk.KEY_y), Modifiers: ModCtrl | ModShift}, ModeNormal)
	require.True(t, ok)
	searchTarget, ok := ParseFloatingProfileTarget(searchAction)
	require.True(t, ok)
	assert.Equal(t, "search", searchTarget.SessionID)
	assert.Equal(t, "https://search.example.com", searchTarget.URL)
}
