package handlers

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/infrastructure/webkit"
	"github.com/bnema/dumber/internal/logging"
)

// ConfigHandler handles configuration-related messages.
type ConfigHandler struct {
	saveConfig func(context.Context, port.WebUIConfig) error
}

// NewConfigHandler creates a new ConfigHandler.
// saveConfig is called to persist config changes (typically usecase.SaveWebUIConfigUseCase.Execute).
func NewConfigHandler(saveConfig func(context.Context, port.WebUIConfig) error) *ConfigHandler {
	if saveConfig == nil {
		panic("NewConfigHandler: saveConfig must not be nil")
	}
	return &ConfigHandler{saveConfig: saveConfig}
}

// Handle processes the save_config message.
func (h *ConfigHandler) Handle(ctx context.Context, _ webkit.WebViewID, payload json.RawMessage) (any, error) {
	if h == nil {
		return nil, fmt.Errorf("config handler is nil")
	}
	if h.saveConfig == nil {
		return nil, fmt.Errorf("config handler: saveConfig not initialized")
	}
	log := logging.FromContext(ctx).With().Str("handler", "config").Logger()

	var payloadCfg port.WebUIConfig
	if err := json.Unmarshal(payload, &payloadCfg); err != nil {
		log.Error().Err(err).Msg("failed to unmarshal config payload")
		return nil, fmt.Errorf("invalid config format: %w", err)
	}

	log.Info().Msg("saving appearance configuration from webui")

	if err := h.saveConfig(ctx, payloadCfg); err != nil {
		log.Error().Err(err).Msg("failed to save config")
		return nil, fmt.Errorf("failed to save config: %w", err)
	}

	return map[string]any{"status": "success"}, nil
}

// RegisterConfigHandlers registers configuration handlers with the router.
func RegisterConfigHandlers(
	ctx context.Context, router *webkit.MessageRouter,
	saveConfig func(context.Context, port.WebUIConfig) error,
) error {
	handler := NewConfigHandler(saveConfig)

	log := logging.FromContext(ctx).With().Str("component", "handlers").Logger()

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
