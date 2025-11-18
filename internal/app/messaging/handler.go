package messaging

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
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
	// Favorites operations
	Title      string `json:"title"`
	FaviconURL string `json:"faviconURL"`
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
	msg, err := parseIncomingMessage(payload)
	if err != nil {
		log.Printf("[ERROR] Failed to unmarshal message: %v", err)
		return
	}

	switch msg.Type {
	case "navigate":
		h.handleNavigation(msg)
	case "query":
		if h.webView == nil {
			return
		}
		h.handleQuery(msg)
	case "omnibox_initial_history":
		if h.webView == nil {
			return
		}
		h.handleOmniboxInitialHistory(msg)
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
	case "get_favorites":
		h.handleGetFavorites(msg)
	case "toggle_favorite":
		h.handleToggleFavorite(msg)
	case "is_favorite":
		h.handleIsFavorite(msg)
	}
}

func parseIncomingMessage(payload string) (Message, error) {
	data := []byte(payload)
	var msg Message
	if err := json.Unmarshal(data, &msg); err == nil {
		return msg, nil
	} else {
		normalized, normErr := normalizeWebViewIDPayload(data)
		if normErr != nil {
			return Message{}, err
		}

		if err := json.Unmarshal(normalized, &msg); err != nil {
			return Message{}, err
		}

		return msg, nil
	}
}

func normalizeWebViewIDPayload(data []byte) ([]byte, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	rawID, ok := raw["webviewId"]
	if !ok {
		return nil, fmt.Errorf("webviewId missing in payload")
	}

	normalizedID, err := parseWebViewIDRaw(rawID)
	if err != nil {
		return nil, err
	}

	raw["webviewId"] = json.RawMessage(strconv.Quote(normalizedID))

	return json.Marshal(raw)
}

func parseWebViewIDRaw(raw json.RawMessage) (string, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return "", nil
	}

	if trimmed[0] == '"' {
		var id string
		if err := json.Unmarshal(trimmed, &id); err != nil {
			return "", err
		}
		return id, nil
	}

	var number json.Number
	if err := json.Unmarshal(trimmed, &number); err == nil {
		return number.String(), nil
	}

	return "", fmt.Errorf("unsupported webviewId format: %s", string(trimmed))
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
	log.Printf("[workspace] Forwarding workspace event: event=%s direction=%s action=%s webviewId=%s", msg.Event, msg.Direction, msg.Action, msg.WebViewID)
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
	if h.webView == nil || h.browserService == nil {
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

	results := make([]suggestion, 0, limit)
	seen := make(map[string]struct{}, limit*2)

	// Shortcuts intentionally omitted; rely on explicit prefix commands (e.g., gh:) or history results

	// History
	if len(results) < limit {
		remaining := limit - len(results)
		if entries, err := h.browserService.SearchHistory(ctx, q, remaining); err == nil {
			for _, e := range entries {
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
				if url == "" {
					continue
				}
				if _, ok := seen[url]; ok {
					continue
				}
				// Use favicon_url from database (populated by FaviconService)
				favicon := ""
				if s, ok := m["favicon_url"].(string); ok && s != "" {
					favicon = s
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
	if b, err := json.Marshal(results); err == nil {
		// Prefer unified page-world API; fallback to legacy global function
		script := "(window.__dumber?.omnibox?.suggestions ? window.__dumber.omnibox.suggestions(" + string(b) + ") : (window.__dumber_omnibox_suggestions && window.__dumber_omnibox_suggestions(" + string(b) + ")))"
		if err := h.webView.InjectScript(script); err != nil {
			log.Printf("[ERROR] Failed to inject omnibox suggestions: %v", err)
		}
	}
}

// handleOmniboxInitialHistory processes initial history display for empty omnibox
func (h *Handler) handleOmniboxInitialHistory(msg Message) {
	if h.webView == nil || h.browserService == nil {
		return
	}

	ctx := context.Background()
	limit := msg.Limit
	if limit <= 0 {
		limit = 10
	}

	// Get config to determine behavior
	cfg, err := h.browserService.GetConfig(ctx)
	if err != nil {
		log.Printf("[ERROR] Failed to get config: %v", err)
		return
	}
	behavior := cfg.Omnibox.InitialBehavior

	type suggestion struct {
		URL     string `json:"url"`
		Favicon string `json:"favicon,omitempty"`
	}

	results := make([]suggestion, 0, limit)
	seen := make(map[string]struct{}, limit*2)

	// Fetch history based on config
	var entries []interface{}

	if behavior == "recent" {
		historyEntries, histErr := h.browserService.GetRecentHistory(ctx, limit)
		err = histErr
		for _, e := range historyEntries {
			entries = append(entries, e)
		}
	} else if behavior == "most_visited" {
		historyEntries, histErr := h.browserService.GetMostVisited(ctx, limit)
		err = histErr
		for _, e := range historyEntries {
			entries = append(entries, e)
		}
	} else if behavior == "none" {
		// Return empty results
		entries = []interface{}{}
	}

	if err != nil {
		log.Printf("[ERROR] Failed to fetch initial history: %v", err)
		return
	}

	// Build suggestions from entries with deduplication
	for _, entry := range entries {
		bb, _ := json.Marshal(entry)
		var m map[string]any
		_ = json.Unmarshal(bb, &m)

		var url string
		if s, ok := m["url"].(string); ok {
			url = s
		} else if s, ok := m["URL"].(string); ok {
			url = s
		}
		if url == "" {
			continue
		}

		// Skip duplicates
		if _, ok := seen[url]; ok {
			continue
		}

		favicon := ""
		if s, ok := m["favicon_url"].(string); ok && s != "" {
			favicon = s
		}

		results = append(results, suggestion{URL: url, Favicon: favicon})
		seen[url] = struct{}{}
		if len(results) >= limit {
			break
		}
	}

	// Inject back to GUI
	if b, err := json.Marshal(results); err == nil {
		script := "(window.__dumber?.omnibox?.suggestions ? window.__dumber.omnibox.suggestions(" + string(b) + ") : (window.__dumber_omnibox_suggestions && window.__dumber_omnibox_suggestions(" + string(b) + ")))"
		if err := h.webView.InjectScript(script); err != nil {
			log.Printf("[ERROR] Failed to inject initial history: %v", err)
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

// handleGetFavorites sends all favorites to JavaScript
func (h *Handler) handleGetFavorites(msg Message) {
	if h.webView == nil {
		log.Printf("[messaging] Cannot provide favorites - no webview available")
		return
	}

	if h.webView.IsDestroyed() {
		log.Printf("[messaging] Cannot provide favorites - webview is destroyed")
		return
	}

	// Get favorites from browser service
	favorites, err := h.browserService.GetFavorites(context.Background())
	if err != nil {
		log.Printf("[messaging] Failed to get favorites: %v", err)
		_ = h.webView.InjectScript("window.__dumber_favorites_error && window.__dumber_favorites_error('Failed to get favorites')")
		return
	}

	// Marshal to JSON
	b, err := json.Marshal(favorites)
	if err != nil {
		log.Printf("[messaging] Failed to marshal favorites: %v", err)
		_ = h.webView.InjectScript("window.__dumber_favorites_error && window.__dumber_favorites_error('Failed to load favorites')")
		return
	}

	// Inject the favorites into the page
	log.Printf("[messaging] Sending %d favorites to JavaScript", len(favorites))
	_ = h.webView.InjectScript("window.__dumber_favorites && window.__dumber_favorites(" + string(b) + ")")
}

// handleToggleFavorite adds or removes a URL from favorites
func (h *Handler) handleToggleFavorite(msg Message) {
	if h.webView == nil {
		log.Printf("[messaging] Cannot toggle favorite - no webview available")
		return
	}

	if h.webView.IsDestroyed() {
		log.Printf("[messaging] Cannot toggle favorite - webview is destroyed")
		return
	}

	if msg.URL == "" {
		log.Printf("[messaging] Cannot toggle favorite - URL is empty")
		return
	}

	// Toggle the favorite
	added, err := h.browserService.ToggleFavorite(context.Background(), msg.URL, msg.Title, msg.FaviconURL)
	if err != nil {
		log.Printf("[messaging] Failed to toggle favorite for %s: %v", msg.URL, err)
		_ = h.webView.InjectScript("window.__dumber_favorite_toggled_error && window.__dumber_favorite_toggled_error('Failed to toggle favorite')")
		return
	}

	// Send result back to JavaScript
	result := map[string]interface{}{
		"url":   msg.URL,
		"added": added,
	}
	b, err := json.Marshal(result)
	if err != nil {
		log.Printf("[messaging] Failed to marshal toggle result: %v", err)
		return
	}

	log.Printf("[messaging] Favorite toggled for %s (added: %v)", msg.URL, added)
	_ = h.webView.InjectScript("window.__dumber_favorite_toggled && window.__dumber_favorite_toggled(" + string(b) + ")")
}

// handleIsFavorite checks if a URL is favorited
func (h *Handler) handleIsFavorite(msg Message) {
	if h.webView == nil {
		log.Printf("[messaging] Cannot check favorite - no webview available")
		return
	}

	if h.webView.IsDestroyed() {
		log.Printf("[messaging] Cannot check favorite - webview is destroyed")
		return
	}

	if msg.URL == "" {
		log.Printf("[messaging] Cannot check favorite - URL is empty")
		return
	}

	// Check if the URL is favorited
	isFavorite, err := h.browserService.IsFavorite(context.Background(), msg.URL)
	if err != nil {
		log.Printf("[messaging] Failed to check if favorite for %s: %v", msg.URL, err)
		return
	}

	// Send result back to JavaScript
	result := map[string]interface{}{
		"url":        msg.URL,
		"isFavorite": isFavorite,
	}
	b, err := json.Marshal(result)
	if err != nil {
		log.Printf("[messaging] Failed to marshal is_favorite result: %v", err)
		return
	}

	_ = h.webView.InjectScript("window.__dumber_is_favorite && window.__dumber_is_favorite(" + string(b) + ")")
}
