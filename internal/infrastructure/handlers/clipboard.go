package handlers

import (
	"context"
	"encoding/json"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/logging"
)

// ClipboardHandler handles clipboard-related messages from webviews.
type ClipboardHandler struct {
	orchestrator port.ClipboardTextOrchestrator
}

// NewClipboardHandler creates a new ClipboardHandler.
func NewClipboardHandler(orchestrator port.ClipboardTextOrchestrator) *ClipboardHandler {
	return &ClipboardHandler{
		orchestrator: orchestrator,
	}
}

// autoCopyRequest represents the payload for auto-copy selection messages.
type autoCopyRequest struct {
	Text string `json:"text"`
}

type explicitCopyRequest struct {
	Text   string `json:"text"`
	Action string `json:"action"`
}

// HandleAutoCopySelection handles the auto_copy_selection message from JS.
// It copies the selected text to the clipboard if the feature is enabled.
func (h *ClipboardHandler) HandleAutoCopySelection() port.WebUIMessageHandler {
	return port.WebUIMessageHandlerFunc(func(ctx context.Context, webviewID port.WebViewID, payload json.RawMessage) (any, error) {
		log := logging.FromContext(ctx)

		var req autoCopyRequest
		if err := json.Unmarshal(payload, &req); err != nil {
			log.Debug().Err(err).Msg("failed to unmarshal auto-copy payload")
			return nil, nil // Silently ignore malformed requests
		}
		if h.orchestrator == nil {
			return nil, nil
		}
		if err := h.orchestrator.HandleSelectionUpdate(ctx, port.SelectionClipboardInput{
			Text:         req.Text,
			SourceEngine: port.ClipboardSourceWebKit,
			ViewID:       webviewID,
		}); err != nil {
			log.Debug().Err(err).Msg("clipboard selection handling failed")
		}

		return nil, nil
	})
}

// HandleExplicitCopy handles the explicit_text_copy message from JS.
func (h *ClipboardHandler) HandleExplicitCopy() port.WebUIMessageHandler {
	return port.WebUIMessageHandlerFunc(func(ctx context.Context, webviewID port.WebViewID, payload json.RawMessage) (any, error) {
		log := logging.FromContext(ctx)

		var req explicitCopyRequest
		if err := json.Unmarshal(payload, &req); err != nil {
			log.Debug().Err(err).Msg("failed to unmarshal explicit copy payload")
			return nil, nil
		}
		if h.orchestrator == nil {
			return nil, nil
		}
		if err := h.orchestrator.HandleExplicitCopy(ctx, port.ExplicitClipboardInput{
			Text:          req.Text,
			Action:        req.Action,
			SourceEngine:  port.ClipboardSourceWebKit,
			ViewID:        webviewID,
			NativeHandled: true,
		}); err != nil {
			log.Debug().Err(err).Msg("clipboard explicit handling failed")
		}

		return nil, nil
	})
}

// RegisterClipboardHandlers registers clipboard handlers with the router.
func RegisterClipboardHandlers(
	ctx context.Context,
	router port.WebUIHandlerRouter,
	orchestrator port.ClipboardTextOrchestrator,
) error {
	handler := NewClipboardHandler(orchestrator)

	// Register auto_copy_selection handler
	if err := router.RegisterHandler("auto_copy_selection", handler.HandleAutoCopySelection()); err != nil {
		return err
	}
	if err := router.RegisterHandler("explicit_text_copy", handler.HandleExplicitCopy()); err != nil {
		return err
	}

	log := logging.FromContext(ctx)
	log.Info().Msg("registered clipboard handlers")

	return nil
}
