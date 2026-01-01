package handlers

import (
	"context"
	"encoding/json"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/infrastructure/config"
	"github.com/bnema/dumber/internal/infrastructure/webkit"
	"github.com/bnema/dumber/internal/logging"
)

// ClipboardHandler handles clipboard-related messages from webviews.
type ClipboardHandler struct {
	clipboard    port.Clipboard
	configGetter func() *config.Config
	onCopied     func(textLen int) // Called after successful auto-copy (for toast notification)
}

// NewClipboardHandler creates a new ClipboardHandler.
func NewClipboardHandler(clipboard port.Clipboard, configGetter func() *config.Config, onCopied func(textLen int)) *ClipboardHandler {
	return &ClipboardHandler{
		clipboard:    clipboard,
		configGetter: configGetter,
		onCopied:     onCopied,
	}
}

// autoCopyRequest represents the payload for auto-copy selection messages.
type autoCopyRequest struct {
	Text string `json:"text"`
}

// HandleAutoCopySelection handles the auto_copy_selection message from JS.
// It copies the selected text to the clipboard if the feature is enabled.
func (h *ClipboardHandler) HandleAutoCopySelection() webkit.MessageHandler {
	return webkit.MessageHandlerFunc(func(ctx context.Context, _ webkit.WebViewID, payload json.RawMessage) (any, error) {
		log := logging.FromContext(ctx)

		// Check if feature is enabled
		cfg := h.configGetter()
		if cfg == nil || !cfg.Clipboard.AutoCopyOnSelection {
			return nil, nil
		}

		var req autoCopyRequest
		if err := json.Unmarshal(payload, &req); err != nil {
			log.Debug().Err(err).Msg("failed to unmarshal auto-copy payload")
			return nil, nil // Silently ignore malformed requests
		}

		// Skip empty or very short selections (< 2 chars)
		if len(req.Text) < 2 {
			return nil, nil
		}

		if err := h.clipboard.WriteText(ctx, req.Text); err != nil {
			log.Debug().Err(err).Msg("failed to write selection to clipboard")
			return nil, nil // Don't propagate error to JS
		}

		log.Debug().Int("length", len(req.Text)).Msg("auto-copied selection to clipboard")

		// Notify UI for toast feedback (if callback is set)
		if h.onCopied != nil {
			h.onCopied(len(req.Text))
		}

		return nil, nil
	})
}

// RegisterClipboardHandlers registers clipboard handlers with the router.
func RegisterClipboardHandlers(
	ctx context.Context,
	router *webkit.MessageRouter,
	clipboard port.Clipboard,
	configGetter func() *config.Config,
	onCopied func(textLen int),
) error {
	handler := NewClipboardHandler(clipboard, configGetter, onCopied)

	// Register auto_copy_selection handler
	if err := router.RegisterHandler("auto_copy_selection", handler.HandleAutoCopySelection()); err != nil {
		return err
	}

	log := logging.FromContext(ctx)
	log.Info().Msg("registered clipboard handlers")

	return nil
}
