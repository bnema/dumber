package config

import (
	"context"
	"reflect"
	"sort"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/logging"
)

const modeGlobal = "global"

// KeybindingsGateway implements port.KeybindingsProvider and port.KeybindingsSaver.
type KeybindingsGateway struct {
	mgr *Manager
}

// NewKeybindingsGateway creates a new KeybindingsGateway.
func NewKeybindingsGateway(mgr *Manager) *KeybindingsGateway {
	return &KeybindingsGateway{mgr: mgr}
}

// GetKeybindings returns all keybindings grouped by mode.
func (g *KeybindingsGateway) GetKeybindings(ctx context.Context) (port.KeybindingsConfig, error) {
	log := logging.FromContext(ctx)
	cfg := g.mgr.Get()
	defaults := DefaultConfig()
	log.Debug().Msg("keybindings gateway: fetching keybindings")

	groups := []port.KeybindingGroup{
		g.buildGlobalGroup(cfg, defaults),
		g.buildPaneModeGroup(cfg, defaults),
		g.buildTabModeGroup(cfg, defaults),
		g.buildResizeModeGroup(cfg, defaults),
		g.buildSessionModeGroup(cfg, defaults),
	}

	return port.KeybindingsConfig{Groups: groups}, nil
}

// GetDefaultKeybindings returns the default keybindings configuration.
func (g *KeybindingsGateway) GetDefaultKeybindings(ctx context.Context) (port.KeybindingsConfig, error) {
	log := logging.FromContext(ctx)
	defaults := DefaultConfig()

	groups := []port.KeybindingGroup{
		g.buildGlobalGroup(defaults, defaults),
		g.buildPaneModeGroup(defaults, defaults),
		g.buildTabModeGroup(defaults, defaults),
		g.buildResizeModeGroup(defaults, defaults),
		g.buildSessionModeGroup(defaults, defaults),
	}

	log.Debug().Int("groups", len(groups)).Msg("keybindings gateway: returning default keybindings")
	return port.KeybindingsConfig{Groups: groups}, nil
}

// CheckConflicts scans all modes for keybinding conflicts.
func (g *KeybindingsGateway) CheckConflicts(ctx context.Context, mode, action string, keys []string) ([]port.KeybindingConflict, error) {
	log := logging.FromContext(ctx)
	cfg := g.mgr.Get()

	conflicts := g.checkConflicts(cfg, mode, action, keys)
	if len(conflicts) > 0 {
		log.Warn().Int("conflicts", len(conflicts)).Msg("keybindings gateway: conflicts detected")
	}

	return conflicts, nil
}

// SetKeybinding updates a single keybinding.
func (g *KeybindingsGateway) SetKeybinding(ctx context.Context, req port.SetKeybindingRequest) error {
	log := logging.FromContext(ctx)
	log.Debug().Str("mode", req.Mode).Str("action", req.Action).Msg("keybindings gateway: setting keybinding")
	cfg := g.mgr.Get()
	g.updateKeybinding(cfg, req)
	return g.mgr.Save(cfg)
}

// ResetKeybinding resets a keybinding to its default value.
func (g *KeybindingsGateway) ResetKeybinding(ctx context.Context, req port.ResetKeybindingRequest) error {
	log := logging.FromContext(ctx)
	log.Info().Str("mode", req.Mode).Str("action", req.Action).Msg("keybindings gateway: resetting keybinding to default")

	defaults := DefaultConfig()
	defaultKeys := g.getDefaultKeys(defaults, req.Mode, req.Action)

	setReq := port.SetKeybindingRequest{
		Mode:   req.Mode,
		Action: req.Action,
		Keys:   defaultKeys,
	}

	cfg := g.mgr.Get()
	g.updateKeybinding(cfg, setReq)

	return g.mgr.Save(cfg)
}

// ResetAllKeybindings resets all keybindings to their default values.
func (g *KeybindingsGateway) ResetAllKeybindings(ctx context.Context) error {
	log := logging.FromContext(ctx)
	log.Info().Msg("keybindings gateway: resetting all keybindings to defaults")

	cfg := g.mgr.Get()
	defaults := DefaultConfig()

	cfg.Workspace.PaneMode.Actions = defaults.Workspace.PaneMode.Actions
	cfg.Workspace.TabMode.Actions = defaults.Workspace.TabMode.Actions
	cfg.Workspace.ResizeMode.Actions = defaults.Workspace.ResizeMode.Actions
	cfg.Workspace.Shortcuts = defaults.Workspace.Shortcuts
	cfg.Session.SessionMode.Actions = defaults.Session.SessionMode.Actions

	return g.mgr.Save(cfg)
}

// buildGlobalGroup builds the global shortcuts group.
func (g *KeybindingsGateway) buildGlobalGroup(cfg, defaults *Config) port.KeybindingGroup {
	return port.KeybindingGroup{
		Mode:        modeGlobal,
		DisplayName: "Global Shortcuts",
		Bindings:    g.buildModeBindings(cfg.Workspace.Shortcuts.Actions, defaults.Workspace.Shortcuts.Actions),
	}
}

// buildPaneModeGroup builds the pane mode group.
func (g *KeybindingsGateway) buildPaneModeGroup(cfg, defaults *Config) port.KeybindingGroup {
	return port.KeybindingGroup{
		Mode:        "pane",
		DisplayName: "Pane Mode",
		Bindings:    g.buildModeBindings(cfg.Workspace.PaneMode.Actions, defaults.Workspace.PaneMode.Actions),
		Activation:  cfg.Workspace.PaneMode.ActivationShortcut,
	}
}

// buildTabModeGroup builds the tab mode group.
func (g *KeybindingsGateway) buildTabModeGroup(cfg, defaults *Config) port.KeybindingGroup {
	return port.KeybindingGroup{
		Mode:        "tab",
		DisplayName: "Tab Mode",
		Bindings:    g.buildModeBindings(cfg.Workspace.TabMode.Actions, defaults.Workspace.TabMode.Actions),
		Activation:  cfg.Workspace.TabMode.ActivationShortcut,
	}
}

// buildResizeModeGroup builds the resize mode group.
func (g *KeybindingsGateway) buildResizeModeGroup(cfg, defaults *Config) port.KeybindingGroup {
	return port.KeybindingGroup{
		Mode:        "resize",
		DisplayName: "Resize Mode",
		Bindings:    g.buildModeBindings(cfg.Workspace.ResizeMode.Actions, defaults.Workspace.ResizeMode.Actions),
		Activation:  cfg.Workspace.ResizeMode.ActivationShortcut,
	}
}

// buildSessionModeGroup builds the session mode group.
func (g *KeybindingsGateway) buildSessionModeGroup(cfg, defaults *Config) port.KeybindingGroup {
	return port.KeybindingGroup{
		Mode:        "session",
		DisplayName: "Session Mode",
		Bindings:    g.buildModeBindings(cfg.Session.SessionMode.Actions, defaults.Session.SessionMode.Actions),
		Activation:  cfg.Session.SessionMode.ActivationShortcut,
	}
}

// buildModeBindings builds keybinding entries for a modal mode.
func (*KeybindingsGateway) buildModeBindings(actions, defaultActions map[string]ActionBinding) []port.KeybindingEntry {
	var entries []port.KeybindingEntry

	actionNames := make([]string, 0, len(actions))
	for action := range actions {
		actionNames = append(actionNames, action)
	}
	sort.Strings(actionNames)

	for _, action := range actionNames {
		binding := actions[action]
		defaultBinding := defaultActions[action]

		entries = append(entries, port.KeybindingEntry{
			Action:      action,
			Description: binding.Desc,
			Keys:        binding.Keys,
			DefaultKeys: defaultBinding.Keys,
			IsCustom:    !reflect.DeepEqual(binding.Keys, defaultBinding.Keys),
		})
	}

	return entries
}

// updateKeybinding updates a keybinding in the config.
func (*KeybindingsGateway) updateKeybinding(cfg *Config, req port.SetKeybindingRequest) {
	switch req.Mode {
	case modeGlobal:
		if existing, ok := cfg.Workspace.Shortcuts.Actions[req.Action]; ok {
			existing.Keys = req.Keys
			cfg.Workspace.Shortcuts.Actions[req.Action] = existing
		}
	case "pane":
		if existing, ok := cfg.Workspace.PaneMode.Actions[req.Action]; ok {
			existing.Keys = req.Keys
			cfg.Workspace.PaneMode.Actions[req.Action] = existing
		}
	case "tab":
		if existing, ok := cfg.Workspace.TabMode.Actions[req.Action]; ok {
			existing.Keys = req.Keys
			cfg.Workspace.TabMode.Actions[req.Action] = existing
		}
	case "resize":
		if existing, ok := cfg.Workspace.ResizeMode.Actions[req.Action]; ok {
			existing.Keys = req.Keys
			cfg.Workspace.ResizeMode.Actions[req.Action] = existing
		}
	case "session":
		if existing, ok := cfg.Session.SessionMode.Actions[req.Action]; ok {
			existing.Keys = req.Keys
			cfg.Session.SessionMode.Actions[req.Action] = existing
		}
	}
}

// getDefaultKeys returns the default keys for an action.
func (*KeybindingsGateway) getDefaultKeys(defaults *Config, mode, action string) []string {
	switch mode {
	case modeGlobal:
		if binding, ok := defaults.Workspace.Shortcuts.Actions[action]; ok {
			return binding.Keys
		}
	case "pane":
		if binding, ok := defaults.Workspace.PaneMode.Actions[action]; ok {
			return binding.Keys
		}
	case "tab":
		if binding, ok := defaults.Workspace.TabMode.Actions[action]; ok {
			return binding.Keys
		}
	case "resize":
		if binding, ok := defaults.Workspace.ResizeMode.Actions[action]; ok {
			return binding.Keys
		}
	case "session":
		if binding, ok := defaults.Session.SessionMode.Actions[action]; ok {
			return binding.Keys
		}
	}
	return nil
}

// checkConflicts scans all modes for keybinding conflicts.
func (*KeybindingsGateway) checkConflicts(cfg *Config, targetMode, targetAction string, newKeys []string) []port.KeybindingConflict {
	var conflicts []port.KeybindingConflict

	type binding struct {
		mode   string
		action string
	}
	keyMap := make(map[string][]binding)

	addBindings := func(mode string, actions map[string]ActionBinding) {
		for action, ab := range actions {
			for _, key := range ab.Keys {
				keyMap[key] = append(keyMap[key], binding{mode: mode, action: action})
			}
		}
	}

	addBindings(modeGlobal, cfg.Workspace.Shortcuts.Actions)
	addBindings("pane", cfg.Workspace.PaneMode.Actions)
	addBindings("tab", cfg.Workspace.TabMode.Actions)
	addBindings("resize", cfg.Workspace.ResizeMode.Actions)
	addBindings("session", cfg.Session.SessionMode.Actions)

	for _, newKey := range newKeys {
		if existing, ok := keyMap[newKey]; ok {
			for _, b := range existing {
				if b.mode == targetMode && b.action == targetAction {
					continue
				}
				if targetMode == modeGlobal || b.mode == modeGlobal || b.mode == targetMode {
					conflicts = append(conflicts, port.KeybindingConflict{
						ConflictingAction: b.action,
						ConflictingMode:   b.mode,
						Key:               newKey,
					})
				}
			}
		}
	}

	return conflicts
}
