package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bnema/dumber/internal/infrastructure/webkit"
	"github.com/bnema/dumber/internal/logging"
)

// NavigateHandler processes navigation requests from the frontend.
type NavigateHandler struct{}

// NewNavigateHandler creates a navigation handler.
func NewNavigateHandler() *NavigateHandler {
	return &NavigateHandler{}
}

// Handle loads the requested URL in the target WebView.
func (h *NavigateHandler) Handle(ctx context.Context, webviewID webkit.WebViewID, payload json.RawMessage) (any, error) {
	log := logging.FromContext(ctx)

	var req struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("decode navigate payload: %w", err)
	}

	url := strings.TrimSpace(req.URL)
	if url == "" {
		return nil, fmt.Errorf("navigation URL is required")
	}

	wv := webkit.LookupWebView(webviewID)
	if wv == nil {
		return nil, fmt.Errorf("webview %d not found", webviewID)
	}

	if err := wv.LoadURI(ctx, url); err != nil {
		log.Warn().
			Err(err).
			Str("url", url).
			Uint64("webview_id", uint64(webviewID)).
			Msg("failed to load URL from frontend")
		return nil, err
	}

	log.Info().
		Str("url", url).
		Uint64("webview_id", uint64(webviewID)).
		Msg("navigation triggered from frontend")

	return map[string]string{"status": "ok"}, nil
}
