package handler

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/bnema/dumber/internal/infrastructure/config"
	"github.com/bnema/dumber/internal/infrastructure/webkit"
	"github.com/bnema/dumber/internal/logging"
)

// PaletteHandler returns color palettes to the frontend.
type PaletteHandler struct {
	config *config.Config
}

// NewPaletteHandler creates a palette handler.
func NewPaletteHandler(cfg *config.Config) *PaletteHandler {
	return &PaletteHandler{config: cfg}
}

// PaletteResponse mirrors the frontend expectation.
type PaletteResponse struct {
	Light   config.ColorPalette `json:"light"`
	Dark    config.ColorPalette `json:"dark"`
	Current string              `json:"current"`
}

// Handle returns light/dark palettes and the current preference.
func (h *PaletteHandler) Handle(ctx context.Context, _ webkit.WebViewID, _ json.RawMessage) (any, error) {
	log := logging.FromContext(ctx)

	if h.config == nil {
		log.Warn().Msg("config is nil; returning default palettes")
		defaultCfg := config.DefaultConfig()
		return PaletteResponse{
			Light:   defaultCfg.Appearance.LightPalette,
			Dark:    defaultCfg.Appearance.DarkPalette,
			Current: "light",
		}, nil
	}

	current := strings.ToLower(h.config.Appearance.ColorScheme)
	switch current {
	case "prefer-dark", "dark":
		current = "dark"
	case "prefer-light", "light":
		current = "light"
	default:
		current = "system"
	}

	return PaletteResponse{
		Light:   h.config.Appearance.LightPalette,
		Dark:    h.config.Appearance.DarkPalette,
		Current: current,
	}, nil
}
