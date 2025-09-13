package messaging

import (
	"context"
	"encoding/json"
	"log"
	neturl "net/url"

	"github.com/bnema/dumber/internal/app/constants"
	"github.com/bnema/dumber/pkg/webkit"
	"github.com/bnema/dumber/services"
)

// Handler processes script messages from the WebView
type Handler struct {
	parserService  *services.ParserService
	browserService *services.BrowserService
	webView        *webkit.WebView
	lastTheme      string
}

// Message represents a script message from the WebView
type Message struct {
	Type  string `json:"type"`
	URL   string `json:"url"`
	Q     string `json:"q"`
	Limit int    `json:"limit"`
	Value string `json:"value"`
	// Wails fetch bridge
	ID      string          `json:"id"`
	Payload json.RawMessage `json:"payload"`
}

// NewHandler creates a new message handler
func NewHandler(parserService *services.ParserService, browserService *services.BrowserService) *Handler {
	return &Handler{
		parserService:  parserService,
		browserService: browserService,
	}
}

// Handle processes incoming script messages
func (h *Handler) Handle(payload string) {
	var msg Message
	if err := json.Unmarshal([]byte(payload), &msg); err != nil {
		return
	}

	switch msg.Type {
	case "navigate":
		h.handleNavigation(msg)
	case "query":
		h.handleQuery(msg)
	case "wails":
		h.handleWailsBridge(msg)
	case "theme":
		h.handleTheme(msg)
	}
}

// SetWebView sets the WebView reference (needed for script injection)
func (h *Handler) SetWebView(webView *webkit.WebView) {
	h.webView = webView
}

// handleNavigation processes navigation requests from the frontend
func (h *Handler) handleNavigation(msg Message) {
	ctx := context.Background()
	res, err := h.parserService.ParseInput(ctx, msg.URL)
	if err == nil {
		if _, navErr := h.browserService.Navigate(ctx, res.URL); navErr != nil {
			log.Printf("Warning: failed to navigate to %s: %v", res.URL, navErr)
		}
		if h.webView != nil {
			_ = h.webView.LoadURL(res.URL)
			if z, zerr := h.browserService.GetZoomLevel(ctx, res.URL); zerr == nil {
				_ = h.webView.SetZoom(z)
			}
		}
	}
}

// handleQuery processes history search queries from the frontend
func (h *Handler) handleQuery(msg Message) {
	ctx := context.Background()
	limit := msg.Limit
	if limit <= 0 || limit > 25 {
		limit = 10
	}
	entries, err := h.browserService.SearchHistory(ctx, msg.Q, limit)
	if err != nil {
		return
	}

	// Map to lightweight items for frontend
	type item struct {
		URL     string `json:"url"`
		Favicon string `json:"favicon"`
	}

	buildFavicon := func(raw string) string {
		u, err := h.parserService.ParseInput(ctx, raw)
		if err != nil || u.URL == "" {
			return ""
		}
		parsed, perr := neturl.Parse(u.URL)
		if perr != nil || parsed.Host == "" {
			return ""
		}
		scheme := parsed.Scheme
		if scheme == "" {
			scheme = "https"
		}
		return scheme + "://" + parsed.Host + "/favicon.ico"
	}

	items := make([]item, 0, len(entries))
	for _, e := range entries {
		items = append(items, item{URL: e.URL, Favicon: buildFavicon(e.URL)})
	}
	b, _ := json.Marshal(items)
	// Update suggestions in page
	if h.webView != nil {
		_ = h.webView.InjectScript("window.__dumber_setSuggestions && window.__dumber_setSuggestions(" + string(b) + ")")
	}
}

// handleWailsBridge processes Wails runtime bridge calls for homepage
func (h *Handler) handleWailsBridge(msg Message) {
	// Payload contains { methodID, methodName?, args }
	var p struct {
		MethodID   uint32          `json:"methodID"`
		MethodName string          `json:"methodName"`
		Args       json.RawMessage `json:"args"`
	}
	if err := json.Unmarshal(msg.Payload, &p); err != nil {
		return
	}

	// Only implement the IDs we need
	switch p.MethodID {
	case constants.HashGetRecentHistory: // BrowserService.GetRecentHistory(limit)
		var args []interface{}
		_ = json.Unmarshal(p.Args, &args)
		limit := 50
		if len(args) > 0 {
			if f, ok := args[0].(float64); ok {
				limit = int(f)
			}
		}
		ctx := context.Background()
		entries, err := h.browserService.GetRecentHistory(ctx, limit)
		if err != nil {
			return
		}
		resp, _ := json.Marshal(entries)
		if h.webView != nil {
			_ = h.webView.InjectScript("window.__dumber_wails_resolve('" + msg.ID + "', " + string(resp) + ")")
		}
	case constants.HashGetSearchShortcuts: // BrowserService.GetSearchShortcuts()
		ctx := context.Background()
		shortcuts, err := h.browserService.GetSearchShortcuts(ctx)
		if err != nil {
			return
		}
		resp, _ := json.Marshal(shortcuts)
		if h.webView != nil {
			_ = h.webView.InjectScript("window.__dumber_wails_resolve('" + msg.ID + "', " + string(resp) + ")")
		}
	default:
		// Return empty JSON to avoid breaking UI
		if h.webView != nil {
			_ = h.webView.InjectScript("window.__dumber_wails_resolve('" + msg.ID + "', '{}')")
		}
	}
}

// handleTheme processes theme-related messages
func (h *Handler) handleTheme(msg Message) {
	if msg.Value != "" && msg.Value != h.lastTheme {
		log.Printf("[theme] color-scheme changed: %s", msg.Value)
		h.lastTheme = msg.Value
	}
}
