package messaging

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/bnema/dumber/internal/app/constants"
	"github.com/bnema/dumber/internal/app/control"
	"github.com/bnema/dumber/internal/app/messaging/handlers"
	"github.com/bnema/dumber/internal/config"
	"github.com/bnema/dumber/internal/filtering"
	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/dumber/internal/services"
	"github.com/bnema/dumber/pkg/webkit"
)

// WorkspaceObserver interface for observing workspace events.
type WorkspaceObserver interface {
	OnWorkspaceMessage(source *webkit.WebView, msg Message)
}

// Handler processes script messages from the WebView
type Handler struct {
	parserService        *services.ParserService
	browserService       *services.BrowserService
	webView              *webkit.WebView
	navigationController *control.NavigationController
	lastTheme            string
	workspaceObserver    WorkspaceObserver
	// Content filtering
	filterManager  *filtering.FilterManager
	bypassRegistry *filtering.BypassRegistry
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
	Range     string `json:"range"`
	Domain    string `json:"domain"`
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
	// Folder/Tag operations
	FolderID   int64  `json:"folderId"`
	TagID      int64  `json:"tagId"`
	FavoriteID int64  `json:"favoriteId"`
	Shortcut   int    `json:"shortcut"`
	Name       string `json:"name"`
	Icon       string `json:"icon"`
	Color      string `json:"color"`
	Position   int64  `json:"position"`
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

// SetWebView sets the WebView reference (needed for script injection)
func (h *Handler) SetWebView(webView *webkit.WebView) {
	h.webView = webView
}

// SetFilterManager sets the filter manager for whitelist operations.
func (h *Handler) SetFilterManager(fm *filtering.FilterManager) {
	h.filterManager = fm
}

// SetBypassRegistry sets the bypass registry for one-time bypass operations.
func (h *Handler) SetBypassRegistry(br *filtering.BypassRegistry) {
	h.bypassRegistry = br
}

// Handle processes incoming script messages
func (h *Handler) Handle(payload string) {
	msg, err := parseIncomingMessage(payload)
	if err != nil {
		logging.Error(fmt.Sprintf("[ERROR] Failed to unmarshal message: %v", err))
		return
	}

	ctx := h.handlerCtx()

	switch msg.Type {
	// ═══════════════════════════════════════════════════════════════
	// NAVIGATION
	// ═══════════════════════════════════════════════════════════════
	case "navigate":
		handlers.HandleNavigation(ctx, handlers.NavigationMessage{URL: msg.URL})

	// ═══════════════════════════════════════════════════════════════
	// OMNIBOX
	// ═══════════════════════════════════════════════════════════════
	case "query":
		if h.webView == nil {
			return
		}
		handlers.HandleQuery(ctx, h.toOmniboxMsg(msg))
	case "omnibox_initial_history":
		if h.webView == nil {
			return
		}
		handlers.HandleOmniboxInitialHistory(ctx, h.toOmniboxMsg(msg))
	case "prefix_query":
		handlers.HandlePrefixQuery(ctx, h.toOmniboxMsg(msg))

	// ═══════════════════════════════════════════════════════════════
	// HISTORY
	// ═══════════════════════════════════════════════════════════════
	case "history_recent":
		handlers.HandleHistoryRecent(ctx, h.toHistoryMsg(msg))
	case "history_stats":
		handlers.HandleHistoryStats(ctx)
	case "history_search":
		handlers.HandleHistorySearch(ctx, h.toHistoryMsg(msg))
	case "history_delete", "history_delete_entry":
		handlers.HandleHistoryDelete(ctx, h.toHistoryMsg(msg))
	case "history_timeline":
		handlers.HandleHistoryTimeline(ctx, h.toHistoryMsg(msg))
	case "history_search_fts":
		handlers.HandleHistorySearchFTS(ctx, h.toHistoryMsg(msg))
	case "history_delete_range":
		handlers.HandleHistoryDeleteRange(ctx, h.toHistoryMsg(msg))
	case "history_clear_all":
		handlers.HandleHistoryClearAll(ctx)
	case "history_delete_domain":
		handlers.HandleHistoryDeleteDomain(ctx, h.toHistoryMsg(msg))
	case "history_analytics":
		handlers.HandleHistoryAnalytics(ctx, h.toHistoryMsg(msg))
	case "history_domain_stats", "domain_stats":
		handlers.HandleDomainStats(ctx, h.toHistoryMsg(msg))

	// ═══════════════════════════════════════════════════════════════
	// FAVORITES
	// ═══════════════════════════════════════════════════════════════
	case "favorite_list", "get_favorites":
		handlers.HandleGetFavorites(ctx, h.toFavoritesMsg(msg))
	case "toggle_favorite":
		handlers.HandleToggleFavorite(ctx, h.toFavoritesMsg(msg))
	case "is_favorite":
		handlers.HandleIsFavorite(ctx, h.toFavoritesMsg(msg))
	case "favorite_set_shortcut":
		handlers.HandleFavoriteSetShortcut(ctx, h.toFavoritesMsg(msg))
	case "favorite_get_by_shortcut":
		handlers.HandleFavoriteGetByShortcut(ctx, h.toFavoritesMsg(msg))
	case "favorite_set_folder":
		handlers.HandleFavoriteSetFolder(ctx, h.toFavoritesMsg(msg))

	// ═══════════════════════════════════════════════════════════════
	// FOLDERS
	// ═══════════════════════════════════════════════════════════════
	case "folder_list":
		handlers.HandleFolderList(ctx, h.toFavoritesMsg(msg))
	case "folder_create":
		handlers.HandleFolderCreate(ctx, h.toFavoritesMsg(msg))
	case "folder_update":
		handlers.HandleFolderUpdate(ctx, h.toFavoritesMsg(msg))
	case "folder_delete":
		handlers.HandleFolderDelete(ctx, h.toFavoritesMsg(msg))

	// ═══════════════════════════════════════════════════════════════
	// TAGS
	// ═══════════════════════════════════════════════════════════════
	case "tag_list":
		handlers.HandleTagList(ctx, h.toFavoritesMsg(msg))
	case "tag_create":
		handlers.HandleTagCreate(ctx, h.toFavoritesMsg(msg))
	case "tag_update":
		handlers.HandleTagUpdate(ctx, h.toFavoritesMsg(msg))
	case "tag_delete":
		handlers.HandleTagDelete(ctx, h.toFavoritesMsg(msg))
	case "tag_assign":
		handlers.HandleTagAssign(ctx, h.toFavoritesMsg(msg))
	case "tag_remove":
		handlers.HandleTagRemove(ctx, h.toFavoritesMsg(msg))

	// ═══════════════════════════════════════════════════════════════
	// WORKSPACE
	// ═══════════════════════════════════════════════════════════════
	case "workspace":
		h.handleWorkspace(msg)
	case "close-popup":
		h.handleClosePopup(msg)

	// ═══════════════════════════════════════════════════════════════
	// CONTENT FILTERING
	// ═══════════════════════════════════════════════════════════════
	case "addToWhitelist":
		h.handleAddToWhitelist(msg)
	case "bypassOnce":
		h.handleBypassOnce(msg)

	// ═══════════════════════════════════════════════════════════════
	// MISC
	// ═══════════════════════════════════════════════════════════════
	case "wails":
		handlers.HandleWailsBridge(ctx, handlers.WailsBridgeMessage{
			ID:      msg.ID,
			Payload: msg.Payload,
		}, constants.HashGetRecentHistory, constants.HashGetSearchShortcuts)
	case "theme":
		handlers.HandleTheme(&h.lastTheme, handlers.ThemeMessage{Value: msg.Value})
	case "console-message":
		cfg := config.Get()
		handlers.HandleConsoleMessage(cfg.Logging.CaptureConsole, handlers.ConsoleMessage{Payload: msg.Payload})
	case "request-webview-id":
		handlers.HandleWebViewIDRequest(ctx)
	case "get_search_shortcuts":
		handlers.HandleGetSearchShortcuts(ctx)
	case "get_color_palettes":
		handlers.HandleGetColorPalettes(ctx)
	case "keyboard_blocking":
		handlers.HandleKeyboardBlocking(ctx, handlers.KeyboardBlockingMessage{Action: msg.Action})
	}
}

// ═══════════════════════════════════════════════════════════════
// MESSAGE PARSING
// ═══════════════════════════════════════════════════════════════

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

// ═══════════════════════════════════════════════════════════════
// WORKSPACE HANDLERS (require WorkspaceObserver interface)
// ═══════════════════════════════════════════════════════════════

func (h *Handler) handleWorkspace(msg Message) {
	if h.workspaceObserver == nil {
		logging.Warn(fmt.Sprintf("[workspace] Received workspace event %q but no observer registered", msg.Event))
		return
	}
	if h.webView == nil {
		logging.Debug(fmt.Sprintf("[workspace] Ignoring workspace event %q: webview not attached", msg.Event))
		return
	}
	logging.Debug(fmt.Sprintf("[workspace] Forwarding workspace event: event=%s direction=%s action=%s webviewId=%s", msg.Event, msg.Direction, msg.Action, msg.WebViewID))
	h.workspaceObserver.OnWorkspaceMessage(h.webView, msg)
}

func (h *Handler) handleClosePopup(msg Message) {
	logging.Debug(fmt.Sprintf("[messaging] Received close-popup request: webviewId=%s reason=%s", msg.WebViewID, msg.Reason))

	if h.workspaceObserver == nil {
		logging.Warn("[messaging] No workspace observer registered for close-popup request")
		return
	}
	if h.webView == nil {
		logging.Debug("[messaging] No webview attached for close-popup request")
		return
	}

	closeMsg := Message{
		Type:      "workspace",
		Event:     "close-popup",
		WebViewID: msg.WebViewID,
		Reason:    msg.Reason,
	}

	logging.Debug(fmt.Sprintf("[messaging] Forwarding close-popup to workspace: webviewId=%s reason=%s", msg.WebViewID, msg.Reason))
	h.workspaceObserver.OnWorkspaceMessage(h.webView, closeMsg)
}

// ═══════════════════════════════════════════════════════════════
// CONTENT FILTERING HANDLERS
// ═══════════════════════════════════════════════════════════════

func (h *Handler) handleAddToWhitelist(msg Message) {
	if h.filterManager == nil {
		logging.Error("[filtering] Cannot add to whitelist: filter manager not set")
		h.sendBlockedPageResponse(msg.RequestID, false, nil, "Filter manager not available")
		return
	}

	domain := msg.Domain
	if domain == "" {
		logging.Error("[filtering] Cannot add to whitelist: domain is empty")
		h.sendBlockedPageResponse(msg.RequestID, false, nil, "Domain is required")
		return
	}

	ctx := context.Background()
	if err := h.filterManager.AddToWhitelist(ctx, domain); err != nil {
		logging.Error(fmt.Sprintf("[filtering] Failed to add %s to whitelist: %v", domain, err))
		h.sendBlockedPageResponse(msg.RequestID, false, nil, err.Error())
		return
	}

	logging.Info(fmt.Sprintf("[filtering] Added %s to whitelist", domain))
	h.sendBlockedPageResponse(msg.RequestID, true, map[string]interface{}{"domain": domain}, "")
}

func (h *Handler) handleBypassOnce(msg Message) {
	if h.bypassRegistry == nil {
		logging.Error("[filtering] Cannot bypass: bypass registry not set")
		h.sendBlockedPageResponse(msg.RequestID, false, nil, "Bypass registry not available")
		return
	}

	url := msg.URL
	if url == "" {
		logging.Error("[filtering] Cannot bypass: URL is empty")
		h.sendBlockedPageResponse(msg.RequestID, false, nil, "URL is required")
		return
	}

	h.bypassRegistry.AllowOnce(url)
	logging.Info(fmt.Sprintf("[filtering] Allowed one-time bypass for %s", url))
	h.sendBlockedPageResponse(msg.RequestID, true, map[string]interface{}{"url": url}, "")
}

func (h *Handler) sendBlockedPageResponse(requestID string, success bool, data interface{}, errMsg string) {
	if h.webView == nil {
		return
	}

	response := map[string]interface{}{
		"requestId": requestID,
		"success":   success,
	}
	if data != nil {
		response["data"] = data
	}
	if errMsg != "" {
		response["error"] = errMsg
	}

	jsonBytes, err := json.Marshal(response)
	if err != nil {
		logging.Error(fmt.Sprintf("[filtering] Failed to marshal response: %v", err))
		return
	}

	script := fmt.Sprintf("window.postMessage(%s, '*')", string(jsonBytes))
	if err := h.webView.InjectScript(script); err != nil {
		logging.Error(fmt.Sprintf("[filtering] Failed to inject response: %v", err))
	}
}

// ═══════════════════════════════════════════════════════════════
// CONTEXT AND MESSAGE CONVERSION
// ═══════════════════════════════════════════════════════════════

func (h *Handler) handlerCtx() *handlers.Context {
	return &handlers.Context{
		ParserService:  h.parserService,
		BrowserService: h.browserService,
		WebView:        h.webView,
		NavController:  h.navigationController,
	}
}

func (h *Handler) toHistoryMsg(msg Message) handlers.HistoryMessage {
	return handlers.HistoryMessage{
		Type:      msg.Type,
		Q:         msg.Q,
		Limit:     msg.Limit,
		Offset:    msg.Offset,
		HistoryID: msg.HistoryID,
		RequestID: msg.RequestID,
		Range:     msg.Range,
		Domain:    msg.Domain,
	}
}

func (h *Handler) toFavoritesMsg(msg Message) handlers.FavoritesMessage {
	return handlers.FavoritesMessage{
		Type:       msg.Type,
		URL:        msg.URL,
		Title:      msg.Title,
		FaviconURL: msg.FaviconURL,
		ID:         msg.FavoriteID,
		FolderID:   msg.FolderID,
		TagID:      msg.TagID,
		Shortcut:   msg.Shortcut,
		Name:       msg.Name,
		Icon:       msg.Icon,
		Color:      msg.Color,
		Position:   msg.Position,
		RequestID:  msg.RequestID,
	}
}

func (h *Handler) toOmniboxMsg(msg Message) handlers.OmniboxMessage {
	return handlers.OmniboxMessage{
		Q:     msg.Q,
		Limit: msg.Limit,
	}
}
