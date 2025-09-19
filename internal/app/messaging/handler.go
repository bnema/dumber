package messaging

import (
	"context"
	"encoding/json"
	"log"
	neturl "net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/bnema/dumber/internal/app/constants"
	"github.com/bnema/dumber/internal/app/control"
	"github.com/bnema/dumber/pkg/webkit"
	"github.com/bnema/dumber/services"
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
	PaneID    string `json:"paneId"`
	// History operations
	HistoryID string `json:"historyId"`
	// Request tracking
	RequestID string `json:"requestId"`
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
	log.Printf("[DEBUG] Received message: %s", payload)
	var msg Message
	if err := json.Unmarshal([]byte(payload), &msg); err != nil {
		log.Printf("[ERROR] Failed to unmarshal message: %v", err)
		return
	}
	log.Printf("[DEBUG] Parsed message type: %s", msg.Type)

	switch msg.Type {
	case "navigate":
		h.handleNavigation(msg)
	case "query":
		log.Printf("[DEBUG] Handling query message: q=%s, limit=%d", msg.Q, msg.Limit)
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

func (h *Handler) handleQuery(msg Message) {
	log.Printf("[DEBUG] handleQuery called: q='%s', limit=%d", msg.Q, msg.Limit)
	if h.webView == nil {
		log.Printf("[ERROR] handleQuery: webView is nil")
		return
	}
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

	expandShortcut := func(tpl string, query string) string {
		esc := neturl.QueryEscape(query)
		u := strings.ReplaceAll(tpl, "{query}", esc)
		u = strings.ReplaceAll(u, "%s", esc)
		return u
	}

	results := make([]suggestion, 0, limit)
	seen := make(map[string]struct{}, limit*2)

	// Shortcuts
	if shortcuts, err := h.browserService.GetSearchShortcuts(ctx); err != nil {
		log.Printf("[ERROR] Failed to get search shortcuts: %v", err)
	} else if len(shortcuts) > 0 {
		log.Printf("[DEBUG] Found %d search shortcuts", len(shortcuts))
		keys := make([]string, 0, len(shortcuts))
		for k := range shortcuts {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			sc := shortcuts[k]
			url := expandShortcut(sc.URL, q)
			log.Printf("[DEBUG] Shortcut %s -> %s", k, url)
			if url == "" {
				continue
			}
			if _, ok := seen[url]; ok {
				continue
			}
			results = append(results, suggestion{URL: url, Favicon: buildFavicon(url)})
			seen[url] = struct{}{}
			if len(results) >= limit {
				break
			}
		}
	} else {
		log.Printf("[DEBUG] No search shortcuts found")
	}

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
				results = append(results, suggestion{URL: url, Favicon: buildFavicon(url)})
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
		log.Printf("[DEBUG] Injecting omnibox suggestions: %s", script)
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
func (h *Handler) handleHistoryStats(msg Message) {
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
