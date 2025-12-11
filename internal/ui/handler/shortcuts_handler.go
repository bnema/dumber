package handler

import (
	"context"
	"encoding/json"

	"github.com/bnema/dumber/internal/infrastructure/config"
	"github.com/bnema/dumber/internal/infrastructure/webkit"
	"github.com/bnema/dumber/internal/logging"
)

// ShortcutsHandler returns configured search shortcuts.
type ShortcutsHandler struct {
	config *config.Config
}

// NewShortcutsHandler creates a new shortcuts handler.
func NewShortcutsHandler(cfg *config.Config) *ShortcutsHandler {
	return &ShortcutsHandler{config: cfg}
}

// Handle returns the configured search shortcuts map.
func (h *ShortcutsHandler) Handle(ctx context.Context, _ webkit.WebViewID, _ json.RawMessage) (any, error) {
	log := logging.FromContext(ctx)

	if h.config == nil {
		log.Warn().Msg("config is nil; returning empty search shortcuts")
		return map[string]config.SearchShortcut{}, nil
	}

	// Return a shallow copy to avoid accidental mutations.
	shortcuts := make(map[string]config.SearchShortcut, len(h.config.SearchShortcuts))
	for key, val := range h.config.SearchShortcuts {
		shortcuts[key] = val
	}

	return shortcuts, nil
}
