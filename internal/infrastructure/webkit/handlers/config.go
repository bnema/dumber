package handlers

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/infrastructure/config"
	"github.com/bnema/dumber/internal/infrastructure/webkit"
	"github.com/bnema/dumber/internal/logging"
)

// ConfigHandler handles configuration-related messages.
type ConfigHandler struct{}

// NewConfigHandler creates a new ConfigHandler.
func NewConfigHandler() *ConfigHandler {
	return &ConfigHandler{}
}

// Handle processes the save_config message.
func (h *ConfigHandler) Handle(ctx context.Context, _ webkit.WebViewID, payload json.RawMessage) (any, error) {
	log := logging.FromContext(ctx).With().Str("handler", "config").Logger()

	var payloadCfg port.WebUIConfig
	if err := json.Unmarshal(payload, &payloadCfg); err != nil {
		log.Error().Err(err).Msg("failed to unmarshal config payload")
		return nil, fmt.Errorf("invalid config format: %w", err)
	}

	log.Info().Msg("saving appearance configuration from webui")
	mgr := config.GetManager()
	if mgr == nil {
		return nil, fmt.Errorf("config manager not initialized")
	}

	uc := usecase.NewSaveWebUIConfigUseCase(config.NewWebUIConfigGateway(mgr))
	if err := uc.Execute(ctx, payloadCfg); err != nil {
		log.Error().Err(err).Msg("failed to save config")
		return nil, fmt.Errorf("failed to save config: %w", err)
	}

	return map[string]any{"status": "success"}, nil
}

// RegisterConfigHandlers registers configuration handlers with the router.
func RegisterConfigHandlers(ctx context.Context, router *webkit.MessageRouter) error {
	handler := NewConfigHandler()

	// worldName empty means main world (since dumb:// pages run in main world)
	// callback/errorCallback can be used if we want to notify JS of success/fail
	// In Svelte we use fetch() for GET, but for POST we use message bridge
	// Actually, if we use message bridge, we might want response callbacks.

	log := logging.FromContext(ctx).With().Str("component", "handlers").Logger()

	// Message type: save_config
	if err := router.RegisterHandlerWithCallbacks(
		"save_config",
		"__dumber_config_saved",
		"__dumber_config_error",
		"",
		handler,
	); err != nil {
		return err
	}

	log.Info().Msg("registered save_config handler")

	return nil
}
