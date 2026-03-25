package input

import (
	"context"
	"testing"

	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/puregotk/v4/gdk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// defaultWorkspaceConfig returns a WorkspaceConfig with the default shortcuts,
// mirroring the relevant portion of config.DefaultConfig().
func defaultWorkspaceConfig() *entity.WorkspaceConfig {
	return &entity.WorkspaceConfig{
		Shortcuts: entity.GlobalShortcutsConfig{
			Actions: map[string]entity.ActionBinding{
				"toggle_floating_pane":   {Keys: []string{"alt+f"}},
				"consume_or_expel_left":  {Keys: []string{"alt+["}},
				"consume_or_expel_right": {Keys: []string{"alt+]"}},
			},
		},
	}
}

func TestShortcutSet_GlobalToggleFloatingPane(t *testing.T) {
	ws := defaultWorkspaceConfig()
	set := NewShortcutSet(context.Background(), ws, nil)

	action, ok := set.Lookup(KeyBinding{Keyval: uint(gdk.KEY_f), Modifiers: ModAlt}, ModeNormal)
	require.True(t, ok)
	assert.Equal(t, ActionToggleFloatingPane, action)
}

func TestShortcutSet_FloatingProfiles_RegisterMultiple(t *testing.T) {
	ws := defaultWorkspaceConfig()
	ws.FloatingPane.Profiles = map[string]entity.FloatingPaneProfile{
		"google": {
			Keys: []string{"alt+g"},
			URL:  "https://google.com",
		},
		"github": {
			Keys: []string{"alt+y"},
			URL:  "https://github.com",
		},
	}

	set := NewShortcutSet(context.Background(), ws, nil)

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
	ws := defaultWorkspaceConfig()
	ws.FloatingPane.Profiles = map[string]entity.FloatingPaneProfile{
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

	shortcuts := collectFloatingProfileShortcutsFromWorkspace(context.Background(), ws, occupied)
	require.Len(t, shortcuts, 1)
	assert.Equal(t, KeyBinding{Keyval: uint(gdk.KEY_g), Modifiers: ModAlt}, shortcuts[0].Binding)
	target, ok := ParseFloatingProfileTarget(shortcuts[0].Action)
	require.True(t, ok)
	assert.Equal(t, "ok", target.SessionID)
	assert.Equal(t, "https://example.com/ok", target.URL)
}

func TestShortcutSet_FloatingProfilesPreserveExistingGlobalShortcuts(t *testing.T) {
	ws := defaultWorkspaceConfig()
	ws.FloatingPane.Profiles = map[string]entity.FloatingPaneProfile{
		"conflict": {
			Keys: []string{"alt+["},
			URL:  "https://example.com/override-attempt",
		},
	}

	set := NewShortcutSet(context.Background(), ws, nil)

	tabAction, ok := set.Lookup(KeyBinding{Keyval: uint(gdk.KEY_bracketleft), Modifiers: ModAlt}, ModeNormal)
	require.True(t, ok)
	assert.Equal(t, ActionConsumeOrExpelLeft, tabAction)

	omniboxAction, ok := set.Lookup(KeyBinding{Keyval: uint(gdk.KEY_l), Modifiers: ModCtrl}, ModeNormal)
	require.True(t, ok)
	assert.Equal(t, ActionOpenOmnibox, omniboxAction)
}

func TestShortcutSet_FloatingProfilesSkipGlobalOnlyConflicts(t *testing.T) {
	ws := defaultWorkspaceConfig()
	ws.FloatingPane.Profiles = map[string]entity.FloatingPaneProfile{
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

	set := NewShortcutSet(context.Background(), ws, nil)

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
	ws := defaultWorkspaceConfig()
	ws.FloatingPane.Profiles = map[string]entity.FloatingPaneProfile{
		"work-mail": {
			Keys: []string{"alt+w"},
			URL:  "https://mail.google.com",
		},
		"personal-mail": {
			Keys: []string{"alt+p"},
			URL:  "https://mail.google.com",
		},
	}

	set := NewShortcutSet(context.Background(), ws, nil)

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
	ws := defaultWorkspaceConfig()
	ws.FloatingPane.Profiles = map[string]entity.FloatingPaneProfile{
		"mail": {
			Keys: []string{"ctrl+alt+m"},
			URL:  "https://mail.example.com",
		},
		"search": {
			Keys: []string{"ctrl+shift+y"},
			URL:  "https://search.example.com",
		},
	}

	set := NewShortcutSet(context.Background(), ws, nil)

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

func TestParseFloatingProfileTarget_AllowsPipeInURL(t *testing.T) {
	action := NewFloatingProfileAction("pipe-test", "https://example.com/a|b")

	target, ok := ParseFloatingProfileTarget(action)
	require.True(t, ok)
	assert.Equal(t, "pipe-test", target.SessionID)
	assert.Equal(t, "https://example.com/a|b", target.URL)
}
