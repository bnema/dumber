package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bnema/dumber/internal/infrastructure/webkit"
	"github.com/bnema/dumber/internal/logging"
)

// KeyboardBlockingHandler toggles keyboard and focus blocking in the page context.
type KeyboardBlockingHandler struct{}

// NewKeyboardBlockingHandler creates a new handler.
func NewKeyboardBlockingHandler() *KeyboardBlockingHandler {
	return &KeyboardBlockingHandler{}
}

// Handle enables or disables blocking by invoking page-world helpers.
func (h *KeyboardBlockingHandler) Handle(ctx context.Context, webviewID webkit.WebViewID, payload json.RawMessage) (any, error) {
	log := logging.FromContext(ctx)

	var req struct {
		Action string `json:"action"`
	}
	if err := json.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("decode keyboard payload: %w", err)
	}

	action := strings.ToLower(strings.TrimSpace(req.Action))
	if action == "" {
		return nil, fmt.Errorf("keyboard_blocking action is required")
	}

	wv := webkit.LookupWebView(webviewID)
	if wv == nil {
		return nil, fmt.Errorf("webview %d not found", webviewID)
	}

	var js string
	switch action {
	case "enable":
		js = `if(window.__dumber_enableKeyboardBlocking){window.__dumber_enableKeyboardBlocking();}`
	case "disable":
		js = `if(window.__dumber_disableKeyboardBlocking){window.__dumber_disableKeyboardBlocking();}`
	default:
		return nil, fmt.Errorf("unknown keyboard_blocking action: %s", action)
	}

	wv.RunJavaScript(ctx, js, "")

	log.Debug().
		Str("action", action).
		Uint64("webview_id", uint64(webviewID)).
		Msg("keyboard blocking toggled")

	return map[string]string{"status": "ok"}, nil
}
