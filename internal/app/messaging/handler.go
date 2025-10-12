package messaging

import (
	"context"
	"encoding/json"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/bnema/dumber/internal/app/constants"
	"github.com/bnema/dumber/internal/app/control"
	"github.com/bnema/dumber/internal/config"
	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/dumber/internal/services"
	"github.com/bnema/dumber/pkg/webkit"
)

// Handler processes script messages from the WebView
type WorkspaceObserver interface {
	OnWorkspaceMessage(source *webkit.WebView, msg Message)
}

type Handler struct {
	parserService        *services.ParserService
	browserService       *services.BrowserService
	webView              *webkit.WebView
	navigationController *control.NavigationController
	lastTheme            string
	workspaceObserver    WorkspaceObserver
}

// Message represents a script message from the WebView
type Message struct {
	Type      string `json:"type"`
	URL       string `json:"url"`
	Q         string `json:"q"`
	Limit     int    `json:"limit"`
	Offset    int    `json:"offset"`
	Value     string `json:"value"`
	Event     string `json:"event"`
	Action    string `json:"action"`
	Direction string `json:"direction"`
	// History operations
	HistoryID string `json:"historyId"`
	// Request tracking
	RequestID string `json:"requestId"`
	// Popup close tracking
	WebViewID string `json:"webviewId"`
	Reason    string `json:"reason"`
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

// SetNavigationController injects the navigation controller for unified navigation flow.
func (h *Handler) SetNavigationController(controller *control.NavigationController) {
	h.navigationController = controller
}

// SetWorkspaceObserver registers a workspace event observer.
func (h *Handler) SetWorkspaceObserver(observer WorkspaceObserver) {
	h.workspaceObserver = observer
}

// Handle processes incoming script messages
func (h *Handler) Handle(payload string) {
	var msg Message
	if err := json.Unmarshal([]byte(payload), &msg); err != nil {
		log.Printf("[ERROR] Failed to unmarshal message: %v", err)
		return
	}

	switch msg.Type {
	case "navigate":
		h.handleNavigation(msg)
	case "query":
		log.Printf("[DEBUG] Handling query message: q=%s, limit=%d", msg.Q, msg.Limit)
		if h.webView == nil {
			log.Printf("[WARN] Dropping query response: webview unavailable for suggestions")
			return
		}
		h.handleQuery(msg)
	case "wails":
		h.handleWailsBridge(msg)
	case "theme":
		h.handleTheme(msg)
	case "history_recent":
		h.handleHistoryRecent(msg)
	case "history_stats":
		h.handleHistoryStats(msg)
	case "history_search":
		h.handleHistorySearch(msg)
	case "history_delete":
		h.handleHistoryDelete(msg)
	case "workspace":
		h.handleWorkspace(msg)
	case "close-popup":
		h.handleClosePopup(msg)
	case "console-message":
		h.handleConsoleMessage(msg)
	case "request-webview-id":
		h.handleWebViewIDRequest(msg)
	case "get_search_shortcuts":
		h.handleGetSearchShortcuts(msg)
	case "get_color_palettes":
		h.handleGetColorPalettes(msg)
	}
}

// SetWebView sets the WebView reference (needed for script injection)
func (h *Handler) SetWebView(webView *webkit.WebView) {
	h.webView = webView
}

// handleNavigation processes navigation requests from the frontend
func (h *Handler) handleNavigation(msg Message) {
	if h.navigationController != nil {
		if err := h.navigationController.NavigateToURL(msg.URL); err != nil {
			log.Printf("[messaging] Navigation controller failed for input %q: %v", msg.URL, err)
			h.legacyNavigate(msg)
		}
		return
	}

	h.legacyNavigate(msg)
}

func (h *Handler) legacyNavigate(msg Message) {
	ctx := context.Background()
	res, err := h.parserService.ParseInput(ctx, msg.URL)
	if err != nil {
		log.Printf("[messaging] Legacy navigation parse failed for %q: %v", msg.URL, err)
		return
	}

	if _, navErr := h.browserService.Navigate(ctx, res.URL); navErr != nil {
		log.Printf("Warning: failed to navigate to %s: %v", res.URL, navErr)
	}

	if h.webView == nil {
		return
	}

	if err := h.webView.LoadURL(res.URL); err != nil {
		log.Printf("[messaging] Legacy LoadURL failed for %s: %v", res.URL, err)
	}

	if z, zerr := h.browserService.GetZoomLevel(ctx, res.URL); zerr == nil {
		if err := h.webView.SetZoom(z); err != nil {
			log.Printf("[messaging] Legacy SetZoom failed for %s: %v", res.URL, err)
		}
	}
}

// (legacy query handler removed; omnibox suggestions now fetched via dumb://api/omnibox/suggestions)
// handleQuery computes omnibox suggestions natively and returns them to the GUI without fetch
func (h *Handler) handleWorkspace(msg Message) {
	if h.workspaceObserver == nil {
		log.Printf("[workspace] Received workspace event %q but no observer registered", msg.Event)
		return
	}
	if h.webView == nil {
		log.Printf("[workspace] Ignoring workspace event %q: webview not attached", msg.Event)
		return
	}
	log.Printf("[workspace] Forwarding workspace event: event=%s direction=%s action=%s", msg.Event, msg.Direction, msg.Action)
	h.workspaceObserver.OnWorkspaceMessage(h.webView, msg)
}

func (h *Handler) handleClosePopup(msg Message) {
	log.Printf("[messaging] Received close-popup request: webviewId=%s reason=%s", msg.WebViewID, msg.Reason)

	if h.workspaceObserver == nil {
		log.Printf("[messaging] No workspace observer registered for close-popup request")
		return
	}
	if h.webView == nil {
		log.Printf("[messaging] No webview attached for close-popup request")
		return
	}

	// Forward close-popup request to workspace manager via observer
	closeMsg := Message{
		Type:      "workspace",
		Event:     "close-popup",
		WebViewID: msg.WebViewID,
		Reason:    msg.Reason,
	}

	log.Printf("[messaging] Forwarding close-popup to workspace: webviewId=%s reason=%s", msg.WebViewID, msg.Reason)
	h.workspaceObserver.OnWorkspaceMessage(h.webView, closeMsg)
}

func (h *Handler) handleQuery(msg Message) {
	if h.webView == nil {
		log.Printf("[WARN] Skipping query handling: webview is nil")
		return
	}
	log.Printf("[DEBUG] handleQuery called: q='%s', limit=%d", msg.Q, msg.Limit)
	if h.browserService == nil {
		log.Printf("[ERROR] handleQuery: browserService is nil")
		return
	}
	ctx := context.Background()
	q := msg.Q
	if q == "" {
		q = ""
	}
	limit := msg.Limit
	if limit <= 0 {
		limit = 10
	}

	// Build suggestions: shortcuts first, then history search
	type suggestion struct {
		URL     string `json:"url"`
		Favicon string `json:"favicon,omitempty"`
	}

	buildFavicon := func(raw string) string {
		if raw == "" {
			return ""
		}
		scheme := "https"
		host := raw
		if i := strings.Index(raw, "://"); i >= 0 {
			if i > 0 {
				scheme = raw[:i]
			}
			rest := raw[i+3:]
			if j := strings.IndexByte(rest, '/'); j >= 0 {
				host = rest[:j]
			} else {
				host = rest
			}
		} else {
			if j := strings.IndexByte(raw, '/'); j >= 0 {
				host = raw[:j]
			}
		}
		if host == "" {
			return ""
		}
		return scheme + "://" + host + "/favicon.ico"
	}

	results := make([]suggestion, 0, limit)
	seen := make(map[string]struct{}, limit*2)

	// Shortcuts intentionally omitted; rely on explicit prefix commands (e.g., gh:) or history results

	// History
	if len(results) < limit {
		remaining := limit - len(results)
		log.Printf("[DEBUG] Searching history for '%s' with limit %d", q, remaining)
		if entries, err := h.browserService.SearchHistory(ctx, q, remaining); err != nil {
			log.Printf("[ERROR] Failed to search history: %v", err)
		} else {
			log.Printf("[DEBUG] Found %d history entries", len(entries))
			for i, e := range entries {
				// JSON roundtrip to map to get url field agnostic of struct tag
				bb, _ := json.Marshal(e)
				var m map[string]any
				_ = json.Unmarshal(bb, &m)
				var url string
				if s, ok := m["url"].(string); ok {
					url = s
				} else if s, ok := m["URL"].(string); ok {
					url = s
				}
				log.Printf("[DEBUG] History entry %d: url=%s", i, url)
				if url == "" {
					continue
				}
				if _, ok := seen[url]; ok {
					continue
				}
				// Use favicon_url from database if available, otherwise build it
				favicon := ""
				if s, ok := m["favicon_url"].(string); ok && s != "" {
					favicon = s
				} else {
					favicon = buildFavicon(url)
				}
				results = append(results, suggestion{URL: url, Favicon: favicon})
				seen[url] = struct{}{}
				if len(results) >= limit {
					break
				}
			}
		}
	}

	// Inject back to GUI
	log.Printf("[DEBUG] Final results count: %d", len(results))
	if b, err := json.Marshal(results); err != nil {
		log.Printf("[ERROR] Failed to marshal results: %v", err)
	} else {
		// Prefer unified page-world API; fallback to legacy global function
		script := "(window.__dumber?.omnibox?.suggestions ? window.__dumber.omnibox.suggestions(" + string(b) + ") : (window.__dumber_omnibox_suggestions && window.__dumber_omnibox_suggestions(" + string(b) + ")))"
		log.Print("[DEBUG] Injecting omnibox suggestions")
		if injErr := h.webView.InjectScript(script); injErr != nil {
			log.Printf("[ERROR] Failed to inject suggestions script: %v", injErr)
		} else {
			log.Printf("[DEBUG] Successfully injected omnibox suggestions")
		}
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

// handleHistoryRecent processes recent history requests
func (h *Handler) handleHistoryRecent(msg Message) {
	if h.browserService == nil || h.webView == nil {
		return
	}

	ctx := context.Background()
	limit := msg.Limit
	if limit <= 0 {
		limit = 20
	}
	offset := msg.Offset

	var entries []interface{}
	var err error

	// Use paginated version if offset is provided
	if offset > 0 {
		histEntries, e := h.browserService.GetRecentHistoryWithOffset(ctx, limit, offset)
		if e != nil {
			err = e
		} else {
			entries = make([]interface{}, len(histEntries))
			for i, entry := range histEntries {
				entries[i] = entry
			}
		}
	} else {
		// Use original method for backward compatibility
		histEntries, e := h.browserService.GetRecentHistory(ctx, limit)
		if e != nil {
			err = e
		} else {
			entries = make([]interface{}, len(histEntries))
			for i, entry := range histEntries {
				entries[i] = entry
			}
		}
	}

	if err != nil {
		log.Printf("[messaging] Failed to get recent history: %v", err)
		requestIdParam := ""
		if msg.RequestID != "" {
			requestIdParam = ", '" + msg.RequestID + "'"
		}
		_ = h.webView.InjectScript("window.__dumber_history_error && window.__dumber_history_error('Failed to load recent history'" + requestIdParam + ")")
		return
	}

	b, _ := json.Marshal(entries)
	requestIdParam := ""
	if msg.RequestID != "" {
		requestIdParam = ", '" + msg.RequestID + "'"
	}
	_ = h.webView.InjectScript("window.__dumber_history_recent && window.__dumber_history_recent(" + string(b) + requestIdParam + ")")
}

// handleHistoryStats processes history stats requests
func (h *Handler) handleHistoryStats(_ Message) {
	if h.browserService == nil || h.webView == nil {
		return
	}

	ctx := context.Background()
	stats, err := h.browserService.GetHistoryStats(ctx)
	if err != nil {
		log.Printf("[messaging] Failed to get history stats: %v", err)
		_ = h.webView.InjectScript("window.__dumber_history_error && window.__dumber_history_error('Failed to load history stats')")
		return
	}

	b, _ := json.Marshal(stats)
	_ = h.webView.InjectScript("window.__dumber_history_stats && window.__dumber_history_stats(" + string(b) + ")")
}

// handleHistorySearch processes history search requests
func (h *Handler) handleHistorySearch(msg Message) {
	if h.browserService == nil || h.webView == nil {
		return
	}

	ctx := context.Background()
	query := msg.Q
	limit := msg.Limit
	if limit <= 0 {
		limit = 5
	}

	entries, err := h.browserService.SearchHistory(ctx, query, limit)
	if err != nil {
		log.Printf("[messaging] Failed to search history: %v", err)
		_ = h.webView.InjectScript("window.__dumber_history_error && window.__dumber_history_error('Failed to search history')")
		return
	}

	b, _ := json.Marshal(entries)
	_ = h.webView.InjectScript("window.__dumber_history_search && window.__dumber_history_search(" + string(b) + ")")
}

// handleHistoryDelete processes history deletion requests
func (h *Handler) handleHistoryDelete(msg Message) {
	if h.browserService == nil || h.webView == nil {
		return
	}

	if msg.HistoryID == "" {
		log.Printf("[messaging] History delete: missing historyId")
		_ = h.webView.InjectScript("window.__dumber_history_error && window.__dumber_history_error('Missing history ID')")
		return
	}

	// Convert string ID to int64
	id, err := strconv.ParseInt(msg.HistoryID, 10, 64)
	if err != nil {
		log.Printf("[messaging] History delete: invalid ID format: %v", err)
		_ = h.webView.InjectScript("window.__dumber_history_error && window.__dumber_history_error('Invalid history ID format')")
		return
	}

	ctx := context.Background()
	err = h.browserService.DeleteHistoryEntry(ctx, id)
	if err != nil {
		log.Printf("[messaging] Failed to delete history entry: %v", err)
		_ = h.webView.InjectScript("window.__dumber_history_error && window.__dumber_history_error('Failed to delete history entry')")
		return
	}

	// Send success response with the deleted ID
	successData := map[string]string{"deletedId": msg.HistoryID}
	b, _ := json.Marshal(successData)
	_ = h.webView.InjectScript("window.__dumber_history_deleted && window.__dumber_history_deleted(" + string(b) + ")")
}

// handleConsoleMessage processes console-message from JavaScript
func (h *Handler) handleConsoleMessage(msg Message) {
	// Check if console capture is enabled
	cfg := config.Get()
	if !cfg.Logging.CaptureConsole {
		return
	}

	// Parse the console message payload
	var consolePayload struct {
		Level   string `json:"level"`
		Message string `json:"message"`
		URL     string `json:"url"`
	}

	if err := json.Unmarshal(msg.Payload, &consolePayload); err != nil {
		log.Printf("[messaging] Failed to unmarshal console-message: %v", err)
		return
	}

	// Send to logging system with [CONSOLE] tag
	logging.CaptureWebKitLog(consolePayload.Message)
}

// handleWebViewIDRequest responds to JavaScript requests for the webview ID
func (h *Handler) handleWebViewIDRequest(msg Message) {
	if h.webView == nil {
		log.Printf("[messaging] Cannot provide webview ID - no webview available")
		return
	}

	// Check if WebView is destroyed before attempting any operations
	if h.webView.IsDestroyed() {
		log.Printf("[messaging] Cannot provide webview ID - webview is destroyed")
		return
	}

	webViewID := h.webView.ID()
	log.Printf("[messaging] Sending webview ID %d to JavaScript", webViewID)

	// Send the webview ID back to JavaScript via custom event
	if err := h.webView.DispatchCustomEvent("dumber:webview-id", map[string]any{
		"webviewId": webViewID,
		"timestamp": time.Now().UnixMilli(),
	}); err != nil {
		log.Printf("[messaging] Failed to send webview ID: %v", err)
	}
}

// handleGetSearchShortcuts sends search shortcuts configuration to JavaScript
func (h *Handler) handleGetSearchShortcuts(msg Message) {
	if h.webView == nil {
		log.Printf("[messaging] Cannot provide search shortcuts - no webview available")
		return
	}

	if h.webView.IsDestroyed() {
		log.Printf("[messaging] Cannot provide search shortcuts - webview is destroyed")
		return
	}

	// Get search shortcuts from browser service config
	shortcuts, err := h.browserService.GetSearchShortcuts(context.Background())
	if err != nil {
		log.Printf("[messaging] Failed to get search shortcuts: %v", err)
		_ = h.webView.InjectScript("window.__dumber_search_shortcuts_error && window.__dumber_search_shortcuts_error('Failed to get search shortcuts')")
		return
	}

	// Marshal to JSON
	b, err := json.Marshal(shortcuts)
	if err != nil {
		log.Printf("[messaging] Failed to marshal search shortcuts: %v", err)
		_ = h.webView.InjectScript("window.__dumber_search_shortcuts_error && window.__dumber_search_shortcuts_error('Failed to load search shortcuts')")
		return
	}

	// Inject the search shortcuts into the page
	log.Printf("[messaging] Sending search shortcuts to JavaScript")
	_ = h.webView.InjectScript("window.__dumber_search_shortcuts && window.__dumber_search_shortcuts(" + string(b) + ")")
}

// handleGetColorPalettes sends color palettes configuration to JavaScript
func (h *Handler) handleGetColorPalettes(msg Message) {
	if h.webView == nil {
		log.Printf("[messaging] Cannot provide color palettes - no webview available")
		return
	}

	if h.webView.IsDestroyed() {
		log.Printf("[messaging] Cannot provide color palettes - webview is destroyed")
		return
	}

	// Get color palettes from browser service config
	palettes := h.browserService.GetColorPalettesForMessaging()

	// Marshal to JSON
	b, err := json.Marshal(palettes)
	if err != nil {
		log.Printf("[messaging] Failed to marshal color palettes: %v", err)
		_ = h.webView.InjectScript("window.__dumber_color_palettes_error && window.__dumber_color_palettes_error('Failed to load color palettes')")
		return
	}

	// Inject the color palettes into the page
	log.Printf("[messaging] Sending color palettes to JavaScript")
	_ = h.webView.InjectScript("window.__dumber_color_palettes && window.__dumber_color_palettes(" + string(b) + ")")
}
