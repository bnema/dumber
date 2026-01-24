package handlers

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/infrastructure/webkit"
	"github.com/bnema/dumber/internal/logging"
)

// KeybindingsHandler handles keybinding-related messages.
type KeybindingsHandler struct {
	getUC      *usecase.GetKeybindingsUseCase
	setUC      *usecase.SetKeybindingUseCase
	resetUC    *usecase.ResetKeybindingUseCase
	resetAllUC *usecase.ResetAllKeybindingsUseCase
}

// NewKeybindingsHandler creates a new KeybindingsHandler.
func NewKeybindingsHandler(
	getUC *usecase.GetKeybindingsUseCase,
	setUC *usecase.SetKeybindingUseCase,
	resetUC *usecase.ResetKeybindingUseCase,
	resetAllUC *usecase.ResetAllKeybindingsUseCase,
) *KeybindingsHandler {
	return &KeybindingsHandler{
		getUC:      getUC,
		setUC:      setUC,
		resetUC:    resetUC,
		resetAllUC: resetAllUC,
	}
}

// HandleGetKeybindings returns all keybindings grouped by mode.
func (h *KeybindingsHandler) HandleGetKeybindings(ctx context.Context, _ webkit.WebViewID, _ json.RawMessage) (any, error) {
	return h.getUC.Execute(ctx)
}

// HandleSetKeybinding updates a single keybinding.
func (h *KeybindingsHandler) HandleSetKeybinding(ctx context.Context, _ webkit.WebViewID, payload json.RawMessage) (any, error) {
	log := logging.FromContext(ctx).With().Str("handler", "keybindings").Logger()
	log.Debug().RawJSON("payload", payload).Msg("HandleSetKeybinding called")

	var req port.SetKeybindingRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		log.Error().Err(err).RawJSON("payload", payload).Msg("failed to unmarshal request")
		return nil, fmt.Errorf("invalid request: %w", err)
	}

	resp, err := h.setUC.Execute(ctx, req)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"status":    "success",
		"conflicts": resp.Conflicts,
	}, nil
}

// HandleResetKeybinding resets a keybinding to default.
func (h *KeybindingsHandler) HandleResetKeybinding(ctx context.Context, _ webkit.WebViewID, payload json.RawMessage) (any, error) {
	log := logging.FromContext(ctx).With().Str("handler", "keybindings").Logger()

	var req port.ResetKeybindingRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("invalid request: %w", err)
	}

	log.Info().Str("mode", req.Mode).Str("action", req.Action).Msg("resetting keybinding to default")

	if err := h.resetUC.Execute(ctx, req); err != nil {
		return nil, err
	}

	return map[string]any{"status": "success"}, nil
}

// HandleResetAllKeybindings resets all keybindings to defaults.
func (h *KeybindingsHandler) HandleResetAllKeybindings(ctx context.Context, _ webkit.WebViewID, _ json.RawMessage) (any, error) {
	log := logging.FromContext(ctx).With().Str("handler", "keybindings").Logger()
	log.Info().Msg("resetting all keybindings to defaults")

	if err := h.resetAllUC.Execute(ctx); err != nil {
		return nil, err
	}

	return map[string]any{"status": "success"}, nil
}

// RegisterKeybindingsHandlers registers keybindings handlers with the router.
func RegisterKeybindingsHandlers(ctx context.Context, router *webkit.MessageRouter, handler *KeybindingsHandler) error {
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
