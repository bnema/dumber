package usecase

import (
	"context"
	"fmt"

	"github.com/bnema/dumber/internal/application/port"
)

// GetKeybindingsUseCase retrieves all keybindings.
type GetKeybindingsUseCase struct {
	provider port.KeybindingsProvider
}

// NewGetKeybindingsUseCase creates a new GetKeybindingsUseCase.
func NewGetKeybindingsUseCase(provider port.KeybindingsProvider) *GetKeybindingsUseCase {
	return &GetKeybindingsUseCase{provider: provider}
}

// Execute retrieves all keybindings.
func (uc *GetKeybindingsUseCase) Execute(ctx context.Context) (port.KeybindingsConfig, error) {
	if uc == nil || uc.provider == nil {
		return port.KeybindingsConfig{}, fmt.Errorf("keybindings provider is nil")
	}
	return uc.provider.GetKeybindings(ctx)
}

// SetKeybindingUseCase updates a single keybinding.
type SetKeybindingUseCase struct {
	saver port.KeybindingsSaver
}

// NewSetKeybindingUseCase creates a new SetKeybindingUseCase.
func NewSetKeybindingUseCase(saver port.KeybindingsSaver) *SetKeybindingUseCase {
	return &SetKeybindingUseCase{saver: saver}
}

// Execute updates a keybinding.
func (uc *SetKeybindingUseCase) Execute(ctx context.Context, req port.SetKeybindingRequest) error {
	if uc == nil || uc.saver == nil {
		return fmt.Errorf("keybindings saver is nil")
	}

	if err := validateSetKeybindingRequest(req); err != nil {
		return err
	}

	return uc.saver.SetKeybinding(ctx, req)
}

// ResetKeybindingUseCase resets a keybinding to default.
type ResetKeybindingUseCase struct {
	saver port.KeybindingsSaver
}

// NewResetKeybindingUseCase creates a new ResetKeybindingUseCase.
func NewResetKeybindingUseCase(saver port.KeybindingsSaver) *ResetKeybindingUseCase {
	return &ResetKeybindingUseCase{saver: saver}
}

// Execute resets a keybinding to default.
func (uc *ResetKeybindingUseCase) Execute(ctx context.Context, req port.ResetKeybindingRequest) error {
	if uc == nil || uc.saver == nil {
		return fmt.Errorf("keybindings saver is nil")
	}

	if err := validateResetKeybindingRequest(req); err != nil {
		return err
	}

	return uc.saver.ResetKeybinding(ctx, req)
}

// ResetAllKeybindingsUseCase resets all keybindings to defaults.
type ResetAllKeybindingsUseCase struct {
	saver port.KeybindingsSaver
}

// NewResetAllKeybindingsUseCase creates a new ResetAllKeybindingsUseCase.
func NewResetAllKeybindingsUseCase(saver port.KeybindingsSaver) *ResetAllKeybindingsUseCase {
	return &ResetAllKeybindingsUseCase{saver: saver}
}

// Execute resets all keybindings to defaults.
func (uc *ResetAllKeybindingsUseCase) Execute(ctx context.Context) error {
	if uc == nil || uc.saver == nil {
		return fmt.Errorf("keybindings saver is nil")
	}
	return uc.saver.ResetAllKeybindings(ctx)
}

func validateSetKeybindingRequest(req port.SetKeybindingRequest) error {
	if req.Mode == "" {
		return fmt.Errorf("mode is required")
	}
	if req.Action == "" {
		return fmt.Errorf("action is required")
	}
	validModes := map[string]bool{"global": true, "pane": true, "tab": true, "resize": true, "session": true}
	if !validModes[req.Mode] {
		return fmt.Errorf("invalid mode: %s", req.Mode)
	}
	return nil
}

func validateResetKeybindingRequest(req port.ResetKeybindingRequest) error {
	if req.Mode == "" {
		return fmt.Errorf("mode is required")
	}
	if req.Action == "" {
		return fmt.Errorf("action is required")
	}
	validModes := map[string]bool{"global": true, "pane": true, "tab": true, "resize": true, "session": true}
	if !validModes[req.Mode] {
		return fmt.Errorf("invalid mode: %s", req.Mode)
	}
	return nil
}
