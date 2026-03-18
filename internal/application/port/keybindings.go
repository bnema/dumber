package port

import "context"

// KeybindingEntry represents a single keybinding for the UI.
type KeybindingEntry struct {
	Action      string   `json:"action"`
	Description string   `json:"description"`
	Keys        []string `json:"keys"`
	DefaultKeys []string `json:"default_keys"`
	IsCustom    bool     `json:"is_custom"`
}

// KeybindingGroup represents keybindings for a mode.
type KeybindingGroup struct {
	Mode        string            `json:"mode"`
	DisplayName string            `json:"display_name"`
	Bindings    []KeybindingEntry `json:"bindings"`
	Activation  string            `json:"activation,omitempty"`
}

// KeybindingsConfig represents all keybindings for the UI.
type KeybindingsConfig struct {
	Groups []KeybindingGroup `json:"groups"`
}

// SetKeybindingRequest represents a request to update a keybinding.
type SetKeybindingRequest struct {
	RequestID string   `json:"requestId"`
	Mode      string   `json:"mode"`
	Action    string   `json:"action"`
	Keys      []string `json:"keys"`
}

// ResetKeybindingRequest represents a request to reset a keybinding.
type ResetKeybindingRequest struct {
	RequestID string `json:"requestId"`
	Mode      string `json:"mode"`
	Action    string `json:"action"`
}

// KeybindingConflict represents a detected conflict.
type KeybindingConflict struct {
	ConflictingAction string `json:"conflicting_action"`
	ConflictingMode   string `json:"conflicting_mode"`
	Key               string `json:"key"`
}

// SetKeybindingResponse is the response from setting a keybinding.
type SetKeybindingResponse struct {
	Conflicts []KeybindingConflict `json:"conflicts"`
}

// KeybindingsProvider provides keybinding configuration data.
type KeybindingsProvider interface {
	GetKeybindings(ctx context.Context) (KeybindingsConfig, error)
	GetDefaultKeybindings(ctx context.Context) (KeybindingsConfig, error)
	CheckConflicts(ctx context.Context, mode, action string, keys []string) ([]KeybindingConflict, error)
}

// KeybindingsSaver persists keybinding changes.
type KeybindingsSaver interface {
	SetKeybinding(ctx context.Context, req SetKeybindingRequest) error
	ResetKeybinding(ctx context.Context, req ResetKeybindingRequest) error
	ResetAllKeybindings(ctx context.Context) error
}

// KeybindingsGetter retrieves all keybindings (narrow interface for the handler).
type KeybindingsGetter interface {
	Execute(ctx context.Context) (KeybindingsConfig, error)
}

// KeybindingSetter updates a single keybinding (narrow interface for the handler).
type KeybindingSetter interface {
	Execute(ctx context.Context, req SetKeybindingRequest) (SetKeybindingResponse, error)
}

// KeybindingResetter resets a single keybinding to default (narrow interface for the handler).
type KeybindingResetter interface {
	Execute(ctx context.Context, req ResetKeybindingRequest) error
}

// AllKeybindingsResetter resets all keybindings to defaults (narrow interface for the handler).
type AllKeybindingsResetter interface {
	Execute(ctx context.Context) error
}
