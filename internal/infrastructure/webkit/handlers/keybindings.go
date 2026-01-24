package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/infrastructure/config"
	"github.com/bnema/dumber/internal/infrastructure/webkit"
	"github.com/bnema/dumber/internal/logging"
)

const modeGlobal = "global"

// KeybindingsHandler handles keybinding-related messages.
type KeybindingsHandler struct{}

// NewKeybindingsHandler creates a new KeybindingsHandler.
func NewKeybindingsHandler() *KeybindingsHandler {
	return &KeybindingsHandler{}
}

// HandleGetKeybindings returns all keybindings grouped by mode.
func (*KeybindingsHandler) HandleGetKeybindings(ctx context.Context, _ webkit.WebViewID, _ json.RawMessage) (any, error) {
	log := logging.FromContext(ctx).With().Str("handler", "keybindings").Logger()

	mgr := config.GetManager()
	if mgr == nil {
		return nil, fmt.Errorf("config manager not initialized")
	}

	cfg := mgr.Get()
	defaults := config.DefaultConfig()

	groups := []port.KeybindingGroup{
		buildGlobalGroup(cfg, defaults),
		buildPaneModeGroup(cfg, defaults),
		buildTabModeGroup(cfg, defaults),
		buildResizeModeGroup(cfg, defaults),
		buildSessionModeGroup(cfg, defaults),
	}

	log.Debug().Int("groups", len(groups)).Msg("returning keybindings")
	return port.KeybindingsConfig{Groups: groups}, nil
}

// HandleSetKeybinding updates a single keybinding.
func (*KeybindingsHandler) HandleSetKeybinding(ctx context.Context, _ webkit.WebViewID, payload json.RawMessage) (any, error) {
	log := logging.FromContext(ctx).With().Str("handler", "keybindings").Logger()
	log.Debug().RawJSON("payload", payload).Msg("HandleSetKeybinding called")

	var req port.SetKeybindingRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		log.Error().Err(err).RawJSON("payload", payload).Msg("failed to unmarshal request")
		return nil, fmt.Errorf("invalid request: %w", err)
	}

	log.Info().Str("mode", req.Mode).Str("action", req.Action).Strs("keys", req.Keys).Msg("setting keybinding")

	mgr := config.GetManager()
	if mgr == nil {
		return nil, fmt.Errorf("config manager not initialized")
	}

	cfg := mgr.Get()

	// Check for conflicts before saving
	conflicts := checkConflicts(cfg, req.Mode, req.Action, req.Keys)
	if len(conflicts) > 0 {
		log.Warn().Int("conflicts", len(conflicts)).Msg("keybinding conflicts detected")
	}

	updateKeybinding(cfg, req)

	if err := mgr.Save(cfg); err != nil {
		return nil, fmt.Errorf("failed to save: %w", err)
	}

	return map[string]any{
		"status":    "success",
		"conflicts": conflicts,
	}, nil
}

// HandleResetKeybinding resets a keybinding to default.
func (*KeybindingsHandler) HandleResetKeybinding(ctx context.Context, _ webkit.WebViewID, payload json.RawMessage) (any, error) {
	log := logging.FromContext(ctx).With().Str("handler", "keybindings").Logger()

	var req port.ResetKeybindingRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("invalid request: %w", err)
	}

	log.Info().Str("mode", req.Mode).Str("action", req.Action).Msg("resetting keybinding to default")

	defaults := config.DefaultConfig()
	defaultKeys := getDefaultKeys(defaults, req.Mode, req.Action)

	setReq := port.SetKeybindingRequest{
		Mode:   req.Mode,
		Action: req.Action,
		Keys:   defaultKeys,
	}

	mgr := config.GetManager()
	if mgr == nil {
		return nil, fmt.Errorf("config manager not initialized")
	}

	cfg := mgr.Get()
	updateKeybinding(cfg, setReq)

	if err := mgr.Save(cfg); err != nil {
		return nil, fmt.Errorf("failed to save: %w", err)
	}

	return map[string]any{"status": "success"}, nil
}

// HandleResetAllKeybindings resets all keybindings to defaults.
func (*KeybindingsHandler) HandleResetAllKeybindings(ctx context.Context, _ webkit.WebViewID, _ json.RawMessage) (any, error) {
	log := logging.FromContext(ctx).With().Str("handler", "keybindings").Logger()
	log.Info().Msg("resetting all keybindings to defaults")

	mgr := config.GetManager()
	if mgr == nil {
		return nil, fmt.Errorf("config manager not initialized")
	}

	cfg := mgr.Get()
	defaults := config.DefaultConfig()

	// Reset all keybinding sections
	cfg.Workspace.PaneMode.Actions = defaults.Workspace.PaneMode.Actions
	cfg.Workspace.TabMode.Actions = defaults.Workspace.TabMode.Actions
	cfg.Workspace.ResizeMode.Actions = defaults.Workspace.ResizeMode.Actions
	cfg.Workspace.Shortcuts = defaults.Workspace.Shortcuts
	cfg.Session.SessionMode.Actions = defaults.Session.SessionMode.Actions

	if err := mgr.Save(cfg); err != nil {
		return nil, fmt.Errorf("failed to save: %w", err)
	}

	return map[string]any{"status": "success"}, nil
}

// buildGlobalGroup builds the global shortcuts group.
func buildGlobalGroup(cfg, defaults *config.Config) port.KeybindingGroup {
	return port.KeybindingGroup{
		Mode:        modeGlobal,
		DisplayName: "Global Shortcuts",
		Bindings:    buildModeBindings(cfg.Workspace.Shortcuts.Actions, defaults.Workspace.Shortcuts.Actions),
	}
}

// buildPaneModeGroup builds the pane mode group.
func buildPaneModeGroup(cfg, defaults *config.Config) port.KeybindingGroup {
	return port.KeybindingGroup{
		Mode:        "pane",
		DisplayName: "Pane Mode",
		Bindings:    buildModeBindings(cfg.Workspace.PaneMode.Actions, defaults.Workspace.PaneMode.Actions),
		Activation:  cfg.Workspace.PaneMode.ActivationShortcut,
	}
}

// buildTabModeGroup builds the tab mode group.
func buildTabModeGroup(cfg, defaults *config.Config) port.KeybindingGroup {
	return port.KeybindingGroup{
		Mode:        "tab",
		DisplayName: "Tab Mode",
		Bindings:    buildModeBindings(cfg.Workspace.TabMode.Actions, defaults.Workspace.TabMode.Actions),
		Activation:  cfg.Workspace.TabMode.ActivationShortcut,
	}
}

// buildResizeModeGroup builds the resize mode group.
func buildResizeModeGroup(cfg, defaults *config.Config) port.KeybindingGroup {
	return port.KeybindingGroup{
		Mode:        "resize",
		DisplayName: "Resize Mode",
		Bindings:    buildModeBindings(cfg.Workspace.ResizeMode.Actions, defaults.Workspace.ResizeMode.Actions),
		Activation:  cfg.Workspace.ResizeMode.ActivationShortcut,
	}
}

// buildSessionModeGroup builds the session mode group.
func buildSessionModeGroup(cfg, defaults *config.Config) port.KeybindingGroup {
	return port.KeybindingGroup{
		Mode:        "session",
		DisplayName: "Session Mode",
		Bindings:    buildModeBindings(cfg.Session.SessionMode.Actions, defaults.Session.SessionMode.Actions),
		Activation:  cfg.Session.SessionMode.ActivationShortcut,
	}
}

// buildModeBindings builds keybinding entries for a modal mode.
func buildModeBindings(actions, defaultActions map[string]config.ActionBinding) []port.KeybindingEntry {
	var entries []port.KeybindingEntry

	// Get all action names and sort them
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
func updateKeybinding(cfg *config.Config, req port.SetKeybindingRequest) {
	switch req.Mode {
	case modeGlobal:
		updateGlobalShortcut(cfg, req.Action, req.Keys)
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

// updateGlobalShortcut updates a global shortcut in the config.
func updateGlobalShortcut(cfg *config.Config, action string, keys []string) {
	if existing, ok := cfg.Workspace.Shortcuts.Actions[action]; ok {
		existing.Keys = keys
		cfg.Workspace.Shortcuts.Actions[action] = existing
	}
}

// getDefaultKeys returns the default keys for an action.
func getDefaultKeys(defaults *config.Config, mode, action string) []string {
	switch mode {
	case modeGlobal:
		return getDefaultGlobalKeys(defaults, action)
	case "pane":
		return defaults.Workspace.PaneMode.Actions[action].Keys
	case "tab":
		return defaults.Workspace.TabMode.Actions[action].Keys
	case "resize":
		return defaults.Workspace.ResizeMode.Actions[action].Keys
	case "session":
		return defaults.Session.SessionMode.Actions[action].Keys
	}
	return nil
}

// getDefaultGlobalKeys returns the default key for a global shortcut.
func getDefaultGlobalKeys(defaults *config.Config, action string) []string {
	if binding, ok := defaults.Workspace.Shortcuts.Actions[action]; ok {
		return binding.Keys
	}
	return nil
}

// checkConflicts scans all modes for keybinding conflicts.
// Returns a list of conflicts where the same key is bound to different actions.
func checkConflicts(cfg *config.Config, targetMode, targetAction string, newKeys []string) []port.KeybindingConflict {
	var conflicts []port.KeybindingConflict

	// Build a map of key -> (mode, action) for all bindings
	type binding struct {
		mode   string
		action string
	}
	keyMap := make(map[string][]binding)

	// Helper to add bindings from a mode
	addBindings := func(mode string, actions map[string]config.ActionBinding) {
		for action, ab := range actions {
			for _, key := range ab.Keys {
				keyMap[key] = append(keyMap[key], binding{mode: mode, action: action})
			}
		}
	}

	// Collect all current bindings
	addBindings(modeGlobal, cfg.Workspace.Shortcuts.Actions)
	addBindings("pane", cfg.Workspace.PaneMode.Actions)
	addBindings("tab", cfg.Workspace.TabMode.Actions)
	addBindings("resize", cfg.Workspace.ResizeMode.Actions)
	addBindings("session", cfg.Session.SessionMode.Actions)

	// Check each new key for conflicts
	for _, newKey := range newKeys {
		if existing, ok := keyMap[newKey]; ok {
			for _, b := range existing {
				// Skip if it's the same action we're updating
				if b.mode == targetMode && b.action == targetAction {
					continue
				}
				// Global shortcuts conflict with everything
				// Modal bindings only conflict within same mode or with global
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

// RegisterKeybindingsHandlers registers keybindings handlers with the router.
func RegisterKeybindingsHandlers(ctx context.Context, router *webkit.MessageRouter) error {
	handler := NewKeybindingsHandler()
	log := logging.FromContext(ctx).With().Str("component", "handlers").Logger()

	// Get all keybindings
	if err := router.RegisterHandlerWithCallbacks(
		"get_keybindings",
		"__dumber_keybindings_loaded",
		"__dumber_keybindings_error",
		"",
		webkit.MessageHandlerFunc(handler.HandleGetKeybindings),
	); err != nil {
		return err
	}

	// Set a single keybinding
	if err := router.RegisterHandlerWithCallbacks(
		"set_keybinding",
		"__dumber_keybinding_set",
		"__dumber_keybinding_set_error",
		"",
		webkit.MessageHandlerFunc(handler.HandleSetKeybinding),
	); err != nil {
		return err
	}

	// Reset a single keybinding
	if err := router.RegisterHandlerWithCallbacks(
		"reset_keybinding",
		"__dumber_keybinding_reset",
		"__dumber_keybinding_reset_error",
		"",
		webkit.MessageHandlerFunc(handler.HandleResetKeybinding),
	); err != nil {
		return err
	}

	// Reset all keybindings
	if err := router.RegisterHandlerWithCallbacks(
		"reset_all_keybindings",
		"__dumber_keybindings_reset_all",
		"__dumber_keybindings_reset_all_error",
		"",
		webkit.MessageHandlerFunc(handler.HandleResetAllKeybindings),
	); err != nil {
		return err
	}

	log.Info().Msg("registered keybindings handlers")
	return nil
}
