package webext

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/bnema/dumber/internal/config"
	"github.com/bnema/dumber/internal/webext/api"
	"github.com/bnema/dumber/pkg/webkit"
	webkitgtk "github.com/diamondburned/gotk4-webkitgtk/pkg/webkit/v6"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
)

// APIRequest represents an API call from the web process
type APIRequest struct {
	ExtensionID string          `json:"extensionId"`
	Function    string          `json:"function"`
	Args        json.RawMessage `json:"args"`
}

// APIResponse represents the response to an API call
type APIResponse struct {
	Data  interface{} `json:"data,omitempty"`
	Error string      `json:"error,omitempty"`
}

// Dispatcher routes WebExtension API calls to appropriate handlers
type Dispatcher struct {
	manager *Manager
	// storageAPI removed - using manager.GetExtension(id).Storage
	runtimeAPI       *api.RuntimeAPIDispatcher
	tabsAPI          *api.TabsAPIDispatcher
	browserActionAPI *api.BrowserActionDispatcher
	pageActionAPI    *api.PageActionDispatcher
	windowsAPI       *api.WindowsAPIDispatcher
	cookiesAPI       *api.CookiesAPIDispatcher
	alarmsAPI        *api.AlarmsAPIDispatcher
	menusAPI         *api.MenusAPIDispatcher
	notificationsAPI *api.NotificationsAPIDispatcher
	downloadsAPI     *api.DownloadsAPIDispatcher
	commandsAPI      *api.CommandsAPIDispatcher
	viewLookup       ViewLookup
	popupLookup      PopupInfoProvider
}

// viewLookupAdapter adapts ViewLookup to api.WebViewLookup
type viewLookupAdapter struct {
	lookup ViewLookup
}

func (a *viewLookupAdapter) GetViewByID(viewID uint64) api.WebViewOperations {
	view := a.lookup.GetViewByID(viewID)
	if view == nil {
		return nil
	}
	return view // *webkit.WebView implements api.WebViewOperations
}

// NewDispatcher creates a new API dispatcher
func NewDispatcher(manager *Manager, viewLookup ViewLookup) *Dispatcher {
	runtimeAPI := api.NewRuntimeAPIDispatcher(manager)

	// Get cookie manager from global network session
	networkSession := webkit.GetGlobalNetworkSession()
	var cookieManager *webkitgtk.CookieManager
	if networkSession != nil {
		cookieManager = networkSession.CookieManager()
	}

	// Create adapters for the tabs API
	// Note: viewLookup is expected to be *BrowserApp which implements both TabOperations and ViewLookup
	webViewLookupAdapter := &viewLookupAdapter{lookup: viewLookup}

	// Cast viewLookup to TabOperations (BrowserApp implements it)
	var tabOps api.TabOperations
	if ops, ok := viewLookup.(api.TabOperations); ok {
		tabOps = ops
	}

	// Get download directory from config
	downloadDir := filepath.Join(os.Getenv("HOME"), "Downloads") // Default
	if cfg := config.Get(); cfg != nil && cfg.Downloads.DefaultLocation != "" {
		downloadDir = cfg.Downloads.DefaultLocation
		// Expand ~ if present
		if strings.HasPrefix(downloadDir, "~/") {
			downloadDir = filepath.Join(os.Getenv("HOME"), downloadDir[2:])
		}
	}

	dispatcher := &Dispatcher{
		manager: manager,
		// storageAPI removed - using manager.GetExtension(id).Storage
		runtimeAPI:       runtimeAPI,
		tabsAPI:          api.NewTabsAPIDispatcher(manager, tabOps, webViewLookupAdapter), // Pass manager as PaneInfoProvider, viewLookup as TabOperations, adapter as WebViewLookup
		browserActionAPI: api.NewBrowserActionDispatcher(),
		pageActionAPI:    api.NewPageActionDispatcher(),
		windowsAPI:       api.NewWindowsAPIDispatcher(manager), // Pass manager as PaneInfoProvider
		cookiesAPI:       api.NewCookiesAPIDispatcher(cookieManager),
		alarmsAPI:        api.NewAlarmsAPIDispatcher(),
		menusAPI:         api.NewMenusAPIDispatcher(),
		notificationsAPI: api.NewNotificationsAPIDispatcher(),
		downloadsAPI:     api.NewDownloadsAPIDispatcher(downloadDir),
		commandsAPI:      api.NewCommandsAPIDispatcher(),
		viewLookup:       viewLookup,
	}

	runtimeAPI.SetPortEventEmitter(dispatcher.emitPortEvent)

	// If cookie manager is available now, great. Otherwise it will be set later.
	if cookieManager != nil {
		log.Printf("[dispatcher] Cookie manager initialized")
	} else {
		log.Printf("[dispatcher] Cookie manager will be initialized later when first WebView is created")
	}

	return dispatcher
}

// emitPortEvent delivers a port event to the given WebView by injecting a receive call.
func (d *Dispatcher) emitPortEvent(viewID uint64, event map[string]interface{}) error {
	if d.viewLookup == nil {
		return fmt.Errorf("no view lookup available")
	}
	view := d.viewLookup.GetViewByID(viewID)
	if view == nil {
		return fmt.Errorf("view %d not found", viewID)
	}
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}
	script := fmt.Sprintf(`try { window.__dumberWebExtReceive && window.__dumberWebExtReceive(%s); } catch (e) { console.error(e); }`, string(data))
	return view.InjectScript(script)
}

// SetPopupManager sets the popup manager on the browserAction API
func (d *Dispatcher) SetPopupManager(pm api.PopupManager) {
	if d.browserActionAPI != nil {
		d.browserActionAPI.SetPopupManager(pm)
	}
}

// SetPopupInfoProvider sets the provider for extension popup info lookups
func (d *Dispatcher) SetPopupInfoProvider(provider PopupInfoProvider) {
	d.popupLookup = provider
}

// InitializeCookieManager sets the cookie manager after network session is created
func (d *Dispatcher) InitializeCookieManager() {
	networkSession := webkit.GetGlobalNetworkSession()
	if networkSession != nil {
		cookieManager := networkSession.CookieManager()
		if cookieManager != nil {
			d.cookiesAPI.SetCookieManager(cookieManager)
			log.Printf("[dispatcher] Cookie manager initialized")
		}
	}
}

// GetCookiesAPI returns the cookies API dispatcher for use by background scripts
func (d *Dispatcher) GetCookiesAPI() *api.CookiesAPIDispatcher {
	return d.cookiesAPI
}

// PopupAPIRequest represents an API call from a popup's JavaScript bridge
type PopupAPIRequest struct {
	ID     int             `json:"id"`
	API    string          `json:"api"`
	Method string          `json:"method"`
	Args   json.RawMessage `json:"args"`
}

// PopupAPIResponse represents the response to a popup API call
type PopupAPIResponse struct {
	ID     int         `json:"id"`
	Result interface{} `json:"result,omitempty"`
	Error  string      `json:"error,omitempty"`
}

// HandlePopupAPIRequest handles API requests from popup JavaScript bridge
// Returns a JSON response to send back to the popup
func (d *Dispatcher) HandlePopupAPIRequest(extID string, viewID uint64, jsonPayload string) *PopupAPIResponse {
	var req PopupAPIRequest
	if err := json.Unmarshal([]byte(jsonPayload), &req); err != nil {
		return &PopupAPIResponse{Error: fmt.Sprintf("invalid request: %v", err)}
	}

	// Build function name in the format "api.method"
	functionName := req.API + "." + req.Method

	// Create context with view and extension info
	ctx := context.Background()
	ctx = context.WithValue(ctx, "sourceViewID", viewID)
	ctx = context.WithValue(ctx, "extensionID", extID)

	// Lookup pane info for the source view
	if d.viewLookup != nil && viewID != 0 {
		if paneInfo := d.viewLookup.GetPaneInfoByViewID(viewID); paneInfo != nil {
			ctx = context.WithValue(ctx, "sourcePaneInfo", paneInfo)
		}
	}

	// Fallback: check if this is an extension popup and use popup info
	if ctx.Value("sourcePaneInfo") == nil && d.popupLookup != nil && viewID != 0 {
		if popupInfo := d.popupLookup.GetPopupInfoByViewID(viewID); popupInfo != nil {
			// Convert to api.PopupInfo for runtime.Connect
			apiPopupInfo := &api.PopupInfo{
				ExtensionID: popupInfo.ExtensionID,
				URL:         popupInfo.URL,
			}
			ctx = context.WithValue(ctx, "sourcePopupInfo", apiPopupInfo)
		}
	}

	// Handle storage separately since it's not in the main dispatch
	if req.API == "storage" {
		result, err := d.handleStorageAPI(ctx, extID, req.Method, req.Args)
		if err != nil {
			return &PopupAPIResponse{ID: req.ID, Error: err.Error()}
		}
		return &PopupAPIResponse{ID: req.ID, Result: result}
	}

	// Handle i18n separately
	if req.API == "i18n" {
		result, err := d.handleI18nAPI(ctx, extID, req.Method, req.Args)
		if err != nil {
			return &PopupAPIResponse{ID: req.ID, Error: err.Error()}
		}
		return &PopupAPIResponse{ID: req.ID, Result: result}
	}

	// Route to main dispatch
	apiReq := &APIRequest{
		ExtensionID: extID,
		Function:    functionName,
		Args:        req.Args,
	}

	result, err := d.dispatch(ctx, apiReq)
	if err != nil {
		return &PopupAPIResponse{ID: req.ID, Error: err.Error()}
	}
	return &PopupAPIResponse{ID: req.ID, Result: result}
}

// handleStorageAPI handles storage.* API calls for popups
func (d *Dispatcher) handleStorageAPI(ctx context.Context, extID, method string, args json.RawMessage) (interface{}, error) {
	ext, ok := d.manager.GetExtension(extID)
	if !ok || ext == nil || ext.Storage == nil {
		return nil, fmt.Errorf("storage not available for extension %s", extID)
	}

	// Parse method: "get", "set", "remove", "clear" or with area prefix like "local.get"
	parts := strings.SplitN(method, ".", 2)
	methodName := method
	if len(parts) == 2 {
		// areaName = parts[0] - currently we only support local storage
		methodName = parts[1]
	}

	// Get storage area (local only for now - sync/session/managed all use local)
	storageArea := ext.Storage.Local()

	// Parse args - popup bridge sends [areaName, keys/items] for most operations
	var argsArray []json.RawMessage
	if err := json.Unmarshal(args, &argsArray); err != nil {
		// Not an array, use args directly
		argsArray = []json.RawMessage{args}
	}

	// Skip the first arg (areaName) if present
	dataArgIndex := 0
	if len(argsArray) > 1 {
		dataArgIndex = 1 // areaName is first, actual data is second
	}

	switch methodName {
	case "get":
		var keys interface{}
		if len(argsArray) > dataArgIndex && string(argsArray[dataArgIndex]) != "null" {
			if err := json.Unmarshal(argsArray[dataArgIndex], &keys); err != nil {
				return nil, fmt.Errorf("invalid keys for storage.get: %w", err)
			}
		}
		return storageArea.Get(keys)
	case "set":
		var items map[string]interface{}
		if len(argsArray) > dataArgIndex {
			if err := json.Unmarshal(argsArray[dataArgIndex], &items); err != nil {
				return nil, fmt.Errorf("invalid items for storage.set: %w", err)
			}
		}
		return nil, storageArea.Set(items)
	case "remove":
		var keys interface{}
		if len(argsArray) > dataArgIndex {
			if err := json.Unmarshal(argsArray[dataArgIndex], &keys); err != nil {
				return nil, fmt.Errorf("invalid keys for storage.remove: %w", err)
			}
		}
		return nil, storageArea.Remove(keys)
	case "clear":
		return nil, storageArea.Clear()
	case "getBytesInUse":
		// Stub - return 0 for now
		return 0, nil
	default:
		return nil, fmt.Errorf("unknown storage method: %s", methodName)
	}
}

// handleI18nAPI handles i18n.* API calls for popups
func (d *Dispatcher) handleI18nAPI(ctx context.Context, extID, method string, args json.RawMessage) (interface{}, error) {
	ext, ok := d.manager.GetExtension(extID)
	if !ok || ext == nil {
		return nil, fmt.Errorf("extension not found: %s", extID)
	}

	switch method {
	case "getMessage":
		// Parse args: [messageName, substitutions]
		var argsArray []json.RawMessage
		if err := json.Unmarshal(args, &argsArray); err != nil {
			return "", nil // Return empty string on error (Chrome behavior)
		}
		if len(argsArray) == 0 {
			return "", nil
		}

		var messageName string
		if err := json.Unmarshal(argsArray[0], &messageName); err != nil {
			return "", nil
		}

		// Get locale from manifest or default to "en"
		locale := "en"
		if ext.Manifest != nil && ext.Manifest.DefaultLocale != "" {
			locale = ext.Manifest.DefaultLocale
		}

		// Load messages from _locales directory
		messages, err := loadI18nMessages(ext.Path, locale)
		if err != nil {
			// Try fallback to "en" if different locale failed
			if locale != "en" {
				messages, err = loadI18nMessages(ext.Path, "en")
			}
			if err != nil {
				return "", nil // No i18n data available
			}
		}

		// Look up the message
		if msg, ok := messages[messageName]; ok {
			return msg.Message, nil
		}
		return "", nil // Return empty string if message not found (Chrome behavior)

	case "getUILanguage":
		return "en", nil // TODO: Get from system locale

	case "getAcceptLanguages":
		return []string{"en"}, nil // TODO: Get from system locale

	default:
		return nil, fmt.Errorf("unknown i18n method: %s", method)
	}
}

// HandleUserMessage handles UserMessage from web process extensions (legacy without view context)
// Returns true to indicate the message was handled
func (d *Dispatcher) HandleUserMessage(message *webkitgtk.UserMessage) bool {
	return d.HandleUserMessageWithView(0, message)
}

// HandleUserMessageWithView handles UserMessage from web process extensions with WebView context
// This is the main entry point for all webext:api calls
// Returns true to indicate the message was handled
func (d *Dispatcher) HandleUserMessageWithView(viewID uint64, message *webkitgtk.UserMessage) bool {
	msgName := message.Name()

	// Only handle webext:api messages
	if !strings.HasPrefix(msgName, "webext:api") {
		return false
	}

	// Extract message parameters
	params := message.Parameters()
	if params == nil {
		d.sendErrorReply(message, "missing message parameters")
		return true
	}

	// Parse the JSON payload
	jsonStr := params.String()
	// Remove quotes if present (GVariant string format)
	if len(jsonStr) >= 2 && jsonStr[0] == '"' && jsonStr[len(jsonStr)-1] == '"' {
		jsonStr = jsonStr[1 : len(jsonStr)-1]
	}

	var req APIRequest
	if err := json.Unmarshal([]byte(jsonStr), &req); err != nil {
		log.Printf("[dispatcher] Failed to parse API request: %v", err)
		d.sendErrorReply(message, fmt.Sprintf("invalid request format: %v", err))
		return true
	}

	// Create context with WebView ID and extension ID
	ctx := context.Background()
	ctx = context.WithValue(ctx, "sourceViewID", viewID)
	ctx = context.WithValue(ctx, "extensionID", req.ExtensionID)

	// Lookup pane info for the source view (for Tab/URL information)
	if d.viewLookup != nil && viewID != 0 {
		if paneInfo := d.viewLookup.GetPaneInfoByViewID(viewID); paneInfo != nil {
			ctx = context.WithValue(ctx, "sourcePaneInfo", paneInfo)
		}
	}

	result, err := d.dispatch(ctx, &req)
	if err != nil {
		d.sendErrorReply(message, err.Error())
		return true
	}

	// Send success response
	d.sendSuccessReply(message, result)
	return true
}

// unwrapJSArgs extracts the first element if args is an array (from JavaScript ...args)
// JavaScript sendAPICall uses ...args which wraps arguments in an array
func unwrapJSArgs(args json.RawMessage) json.RawMessage {
	if len(args) == 0 || string(args) == "null" {
		return args
	}

	var argsArray []json.RawMessage
	if err := json.Unmarshal(args, &argsArray); err == nil && len(argsArray) > 0 {
		// Successfully parsed as array, return first element
		return argsArray[0]
	}

	// Not an array or empty, return as-is
	return args
}

// dispatch routes the API call to the appropriate handler
func (d *Dispatcher) dispatch(ctx context.Context, req *APIRequest) (interface{}, error) {
	// Split function name into namespace and method (e.g., "storage.local.get" -> ["storage", "local.get"])
	parts := strings.SplitN(req.Function, ".", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid function name format: %s", req.Function)
	}

	namespace := parts[0]
	method := parts[1]

	switch namespace {
	case "runtime":
		return d.handleRuntimeAPI(ctx, req.ExtensionID, method, req.Args)

	case "tabs":
		return d.handleTabsAPI(ctx, req.ExtensionID, method, req.Args)

	case "webRequest":
		return d.handleWebRequestAPI(ctx, req.ExtensionID, method, req.Args)

	case "browserAction":
		return d.handleBrowserActionAPI(ctx, req.ExtensionID, method, req.Args)

	case "pageAction":
		return d.handlePageActionAPI(ctx, req.ExtensionID, method, req.Args)

	case "windows":
		return d.handleWindowsAPI(ctx, req.ExtensionID, method, req.Args)

	case "cookies":
		return d.handleCookiesAPI(ctx, req.ExtensionID, method, req.Args)

	case "alarms":
		return d.handleAlarmsAPI(ctx, req.ExtensionID, method, req.Args)

	case "menus", "contextMenus":
		return d.handleMenusAPI(ctx, req.ExtensionID, method, req.Args)

	case "notifications":
		return d.handleNotificationsAPI(ctx, req.ExtensionID, method, req.Args)

	case "downloads":
		return d.handleDownloadsAPI(ctx, req.ExtensionID, method, req.Args)

	case "commands":
		return d.handleCommandsAPI(ctx, req.ExtensionID, method, req.Args)

	case "permissions":
		return d.handlePermissionsAPI(ctx, req.ExtensionID, method, req.Args)

	default:
		return nil, fmt.Errorf("unsupported API namespace: %s", namespace)
	}
}

// handleRuntimeAPI handles runtime.* API calls
func (d *Dispatcher) handleRuntimeAPI(ctx context.Context, extID, method string, args json.RawMessage) (interface{}, error) {
	switch method {
	case "sendMessage":
		var message interface{}
		unwrapped := unwrapJSArgs(args)
		if err := json.Unmarshal(unwrapped, &message); err != nil {
			return nil, fmt.Errorf("invalid arguments for runtime.sendMessage: %w", err)
		}
		return d.runtimeAPI.SendMessage(ctx, extID, message)

	case "connect":
		var connectInfo api.ConnectInfo
		unwrapped := unwrapJSArgs(args)
		if len(unwrapped) > 0 && string(unwrapped) != "null" {
			if err := json.Unmarshal(unwrapped, &connectInfo); err != nil {
				return nil, fmt.Errorf("invalid arguments for runtime.connect: %w", err)
			}
		}
		return d.runtimeAPI.Connect(ctx, extID, &connectInfo)

	case "port.postMessage":
		var portPayload struct {
			PortID  string      `json:"portId"`
			Message interface{} `json:"message"`
		}
		unwrapped := unwrapJSArgs(args)
		if err := json.Unmarshal(unwrapped, &portPayload); err != nil {
			return nil, fmt.Errorf("invalid arguments for runtime.port.postMessage: %w", err)
		}
		return d.runtimeAPI.PortPostMessage(ctx, extID, portPayload.PortID, portPayload.Message)

	case "port.disconnect":
		var portPayload struct {
			PortID string `json:"portId"`
		}
		unwrapped := unwrapJSArgs(args)
		if err := json.Unmarshal(unwrapped, &portPayload); err != nil {
			return nil, fmt.Errorf("invalid arguments for runtime.port.disconnect: %w", err)
		}
		return d.runtimeAPI.PortDisconnect(ctx, extID, portPayload.PortID)

	case "getPlatformInfo":
		return d.runtimeAPI.GetPlatformInfo(ctx)

	case "openOptionsPage":
		return nil, d.runtimeAPI.OpenOptionsPage(ctx, extID)

	case "sendResponse":
		// Handle response from content script's sendResponse callback
		var payload struct {
			RequestID string      `json:"requestId"`
			Response  interface{} `json:"response"`
		}
		unwrapped := unwrapJSArgs(args)
		if err := json.Unmarshal(unwrapped, &payload); err != nil {
			return nil, fmt.Errorf("invalid arguments for runtime.sendResponse: %w", err)
		}
		if d.viewLookup != nil {
			d.viewLookup.HandleSendMessageResponse(payload.RequestID, payload.Response)
		}
		return nil, nil

	default:
		return nil, fmt.Errorf("unsupported runtime method: %s", method)
	}
}

// handleTabsAPI handles tabs.* API calls
func (d *Dispatcher) handleTabsAPI(ctx context.Context, extID, method string, args json.RawMessage) (interface{}, error) {
	// Get extension for permission checking
	ext, ok := d.manager.GetExtension(extID)
	if !ok || ext == nil {
		return nil, fmt.Errorf("extension not found: %s", extID)
	}

	unwrapped := unwrapJSArgs(args)

	switch method {
	case "query":
		var queryInfo map[string]interface{}
		if len(unwrapped) > 0 && string(unwrapped) != "null" {
			if err := json.Unmarshal(unwrapped, &queryInfo); err != nil {
				return nil, fmt.Errorf("invalid arguments for tabs.query: %w", err)
			}
		}
		return d.tabsAPI.Query(ctx, queryInfo)

	case "get":
		var tabID int64
		if err := json.Unmarshal(unwrapped, &tabID); err != nil {
			return nil, fmt.Errorf("invalid arguments for tabs.get: %w", err)
		}
		return d.tabsAPI.Get(ctx, tabID)

	case "create":
		var createProperties map[string]interface{}
		if len(unwrapped) > 0 && string(unwrapped) != "null" {
			if err := json.Unmarshal(unwrapped, &createProperties); err != nil {
				return nil, fmt.Errorf("invalid arguments for tabs.create: %w", err)
			}
		}
		if createProperties == nil {
			createProperties = make(map[string]interface{})
		}
		return d.tabsAPI.Create(ctx, ext, createProperties)

	case "remove":
		var tabIDs interface{}
		if err := json.Unmarshal(unwrapped, &tabIDs); err != nil {
			return nil, fmt.Errorf("invalid arguments for tabs.remove: %w", err)
		}
		return nil, d.tabsAPI.Remove(ctx, tabIDs)

	case "update":
		// Can be called as update(updateProperties) or update(tabId, updateProperties)
		var params []interface{}
		if err := json.Unmarshal(unwrapped, &params); err != nil {
			return nil, fmt.Errorf("invalid arguments for tabs.update: %w", err)
		}

		var tabID int64 = -1 // -1 means current tab
		var updateProperties map[string]interface{}

		if len(params) == 1 {
			// update(updateProperties)
			if props, ok := params[0].(map[string]interface{}); ok {
				updateProperties = props
			}
		} else if len(params) >= 2 {
			// update(tabId, updateProperties)
			if tidFloat, ok := params[0].(float64); ok {
				tabID = int64(tidFloat)
			}
			if props, ok := params[1].(map[string]interface{}); ok {
				updateProperties = props
			}
		}

		if updateProperties == nil {
			return nil, fmt.Errorf("tabs.update(): missing updateProperties")
		}

		return d.tabsAPI.Update(ctx, tabID, updateProperties)

	case "reload":
		// Can be called as reload() or reload(tabId) or reload(tabId, reloadProperties)
		var params []interface{}
		if len(unwrapped) > 0 && string(unwrapped) != "null" {
			if err := json.Unmarshal(unwrapped, &params); err != nil {
				return nil, fmt.Errorf("invalid arguments for tabs.reload: %w", err)
			}
		}

		var tabID int64 = -1 // -1 means current tab
		reloadProperties := make(map[string]interface{})

		if len(params) >= 1 {
			if tidFloat, ok := params[0].(float64); ok {
				tabID = int64(tidFloat)
			}
		}
		if len(params) >= 2 {
			if props, ok := params[1].(map[string]interface{}); ok {
				reloadProperties = props
			}
		}

		return nil, d.tabsAPI.Reload(ctx, tabID, reloadProperties)

	case "executeScript":
		// Can be called as executeScript(details) or executeScript(tabId, details)
		var params []interface{}
		if err := json.Unmarshal(unwrapped, &params); err != nil {
			return nil, fmt.Errorf("invalid arguments for tabs.executeScript: %w", err)
		}

		var tabID int64 = -1 // -1 means current tab
		var details map[string]interface{}

		if len(params) == 1 {
			// executeScript(details)
			if d, ok := params[0].(map[string]interface{}); ok {
				details = d
			}
		} else if len(params) >= 2 {
			// executeScript(tabId, details)
			if tidFloat, ok := params[0].(float64); ok {
				tabID = int64(tidFloat)
			}
			if d, ok := params[1].(map[string]interface{}); ok {
				details = d
			}
		}

		if details == nil {
			return nil, fmt.Errorf("tabs.executeScript(): missing details")
		}

		return d.tabsAPI.ExecuteScript(ctx, ext, tabID, details)

	case "insertCSS":
		// Can be called as insertCSS(details) or insertCSS(tabId, details)
		var params []interface{}
		if err := json.Unmarshal(unwrapped, &params); err != nil {
			return nil, fmt.Errorf("invalid arguments for tabs.insertCSS: %w", err)
		}

		var tabID int64 = -1 // -1 means current tab
		var details map[string]interface{}

		if len(params) == 1 {
			// insertCSS(details)
			if d, ok := params[0].(map[string]interface{}); ok {
				details = d
			}
		} else if len(params) >= 2 {
			// insertCSS(tabId, details)
			if tidFloat, ok := params[0].(float64); ok {
				tabID = int64(tidFloat)
			}
			if d, ok := params[1].(map[string]interface{}); ok {
				details = d
			}
		}

		if details == nil {
			return nil, fmt.Errorf("tabs.insertCSS(): missing details")
		}

		return nil, d.tabsAPI.InsertCSS(ctx, ext, tabID, details)

	case "removeCSS":
		// Can be called as removeCSS(details) or removeCSS(tabId, details)
		var params []interface{}
		if err := json.Unmarshal(unwrapped, &params); err != nil {
			return nil, fmt.Errorf("invalid arguments for tabs.removeCSS: %w", err)
		}

		var tabID int64 = -1 // -1 means current tab
		var details map[string]interface{}

		if len(params) == 1 {
			// removeCSS(details)
			if d, ok := params[0].(map[string]interface{}); ok {
				details = d
			}
		} else if len(params) >= 2 {
			// removeCSS(tabId, details)
			if tidFloat, ok := params[0].(float64); ok {
				tabID = int64(tidFloat)
			}
			if d, ok := params[1].(map[string]interface{}); ok {
				details = d
			}
		}

		if details == nil {
			return nil, fmt.Errorf("tabs.removeCSS(): missing details")
		}

		return nil, d.tabsAPI.RemoveCSS(ctx, ext, tabID, details)

	case "getZoom":
		// Can be called as getZoom() or getZoom(tabId)
		var tabID int64 = -1 // -1 means current tab
		if len(unwrapped) > 0 && string(unwrapped) != "null" {
			if err := json.Unmarshal(unwrapped, &tabID); err != nil {
				return nil, fmt.Errorf("invalid arguments for tabs.getZoom: %w", err)
			}
		}

		return d.tabsAPI.GetZoom(ctx, tabID)

	case "setZoom":
		// Can be called as setZoom(zoomFactor) or setZoom(tabId, zoomFactor)
		var params []interface{}
		if err := json.Unmarshal(unwrapped, &params); err != nil {
			return nil, fmt.Errorf("invalid arguments for tabs.setZoom: %w", err)
		}

		var tabID int64 = -1 // -1 means current tab
		var zoomFactor float64

		if len(params) == 1 {
			// setZoom(zoomFactor)
			if zoom, ok := params[0].(float64); ok {
				zoomFactor = zoom
			}
		} else if len(params) >= 2 {
			// setZoom(tabId, zoomFactor)
			if tidFloat, ok := params[0].(float64); ok {
				tabID = int64(tidFloat)
			}
			if zoom, ok := params[1].(float64); ok {
				zoomFactor = zoom
			}
		}

		return nil, d.tabsAPI.SetZoom(ctx, tabID, zoomFactor)

	case "sendMessage":
		// tabs.sendMessage(tabId, message, options)
		var params []interface{}
		if err := json.Unmarshal(unwrapped, &params); err != nil {
			return nil, fmt.Errorf("invalid arguments for tabs.sendMessage: %w", err)
		}

		if len(params) < 2 {
			return nil, fmt.Errorf("tabs.sendMessage(): requires tabId and message")
		}

		var tabID int64
		if tidFloat, ok := params[0].(float64); ok {
			tabID = int64(tidFloat)
		} else {
			return nil, fmt.Errorf("tabs.sendMessage(): invalid tabId")
		}

		message := params[1]

		var options map[string]interface{}
		if len(params) >= 3 {
			if opts, ok := params[2].(map[string]interface{}); ok {
				options = opts
			}
		}

		return d.tabsAPI.SendMessage(ctx, ext, tabID, message, options)

	default:
		return nil, fmt.Errorf("unsupported tabs method: %s", method)
	}
}

// handleWebRequestAPI handles webRequest.* API calls
// In the clean architecture, webRequest listeners are managed directly by BackgroundContext (Sobek VM).
// The web process dispatches events via Manager.DispatchWebRequestEvent -> BackgroundContext.
// These dispatcher methods are kept for compatibility but most are no-ops since listeners
// are registered in background scripts, not via the dispatcher API.
func (d *Dispatcher) handleWebRequestAPI(ctx context.Context, extID, method string, args json.RawMessage) (interface{}, error) {
	switch method {
	case "onBeforeRequest.addListener",
		"onBeforeSendHeaders.addListener",
		"onHeadersReceived.addListener",
		"onResponseStarted.addListener",
		"onCompleted.addListener",
		"onErrorOccurred.addListener":
		// Listener registration happens in BackgroundContext (Sobek VM).
		// Content scripts cannot register webRequest listeners directly.
		// Return success for API compatibility.
		log.Printf("[dispatcher] webRequest.%s called from content script (no-op, use background script)", method)
		return nil, nil

	case "onBeforeRequest.removeListener",
		"onBeforeSendHeaders.removeListener",
		"onHeadersReceived.removeListener",
		"onResponseStarted.removeListener",
		"onCompleted.removeListener",
		"onErrorOccurred.removeListener":
		// Listener removal handled by BackgroundContext lifecycle.
		return nil, nil

	case "handlerBehaviorChanged":
		// No-op - behavior changes are handled automatically.
		return nil, nil

	default:
		return nil, fmt.Errorf("unsupported webRequest method: %s", method)
	}
}

// handleWindowsAPI handles windows.* API calls
func (d *Dispatcher) handleWindowsAPI(ctx context.Context, extID, method string, args json.RawMessage) (interface{}, error) {
	unwrapped := unwrapJSArgs(args)

	switch method {
	case "get":
		var payload struct {
			WindowID int64                  `json:"windowId"`
			GetInfo  map[string]interface{} `json:"getInfo"`
		}
		if len(unwrapped) > 0 && string(unwrapped) != "null" {
			if err := json.Unmarshal(unwrapped, &payload); err != nil {
				return nil, fmt.Errorf("invalid arguments for windows.get: %w", err)
			}
		}
		return d.windowsAPI.Get(ctx, payload.WindowID, payload.GetInfo)

	case "getCurrent":
		var getInfo map[string]interface{}
		if len(unwrapped) > 0 && string(unwrapped) != "null" {
			if err := json.Unmarshal(unwrapped, &getInfo); err != nil {
				return nil, fmt.Errorf("invalid arguments for windows.getCurrent: %w", err)
			}
		}
		return d.windowsAPI.GetCurrent(ctx, getInfo)

	case "getLastFocused":
		var getInfo map[string]interface{}
		if len(unwrapped) > 0 && string(unwrapped) != "null" {
			if err := json.Unmarshal(unwrapped, &getInfo); err != nil {
				return nil, fmt.Errorf("invalid arguments for windows.getLastFocused: %w", err)
			}
		}
		return d.windowsAPI.GetLastFocused(ctx, getInfo)

	case "getAll":
		var getInfo map[string]interface{}
		if len(unwrapped) > 0 && string(unwrapped) != "null" {
			if err := json.Unmarshal(unwrapped, &getInfo); err != nil {
				return nil, fmt.Errorf("invalid arguments for windows.getAll: %w", err)
			}
		}
		return d.windowsAPI.GetAll(ctx, getInfo)

	case "create":
		var createData map[string]interface{}
		if len(unwrapped) > 0 && string(unwrapped) != "null" {
			if err := json.Unmarshal(unwrapped, &createData); err != nil {
				return nil, fmt.Errorf("invalid arguments for windows.create: %w", err)
			}
		}
		return d.windowsAPI.Create(ctx, createData)

	case "remove":
		var windowID int64
		if err := json.Unmarshal(unwrapped, &windowID); err != nil {
			return nil, fmt.Errorf("invalid arguments for windows.remove: %w", err)
		}
		return nil, d.windowsAPI.Remove(ctx, windowID)

	default:
		return nil, fmt.Errorf("unsupported windows method: %s", method)
	}
}

// handleCookiesAPI handles cookies.* API calls
func (d *Dispatcher) handleCookiesAPI(ctx context.Context, extID, method string, args json.RawMessage) (interface{}, error) {
	// Get extension for permission checking
	ext, ok := d.manager.GetExtension(extID)
	if !ok || ext == nil {
		return nil, fmt.Errorf("extension not found: %s", extID)
	}

	unwrapped := unwrapJSArgs(args)

	switch method {
	case "get":
		var details map[string]interface{}
		if len(unwrapped) > 0 && string(unwrapped) != "null" {
			if err := json.Unmarshal(unwrapped, &details); err != nil {
				return nil, fmt.Errorf("invalid arguments for cookies.get: %w", err)
			}
		}
		if details == nil {
			details = make(map[string]interface{})
		}
		return d.cookiesAPI.Get(ctx, ext, details)

	case "getAll":
		var details map[string]interface{}
		if len(unwrapped) > 0 && string(unwrapped) != "null" {
			if err := json.Unmarshal(unwrapped, &details); err != nil {
				return nil, fmt.Errorf("invalid arguments for cookies.getAll: %w", err)
			}
		}
		if details == nil {
			details = make(map[string]interface{})
		}
		return d.cookiesAPI.GetAll(ctx, ext, details)

	case "set":
		var details map[string]interface{}
		if len(unwrapped) > 0 && string(unwrapped) != "null" {
			if err := json.Unmarshal(unwrapped, &details); err != nil {
				return nil, fmt.Errorf("invalid arguments for cookies.set: %w", err)
			}
		}
		if details == nil {
			details = make(map[string]interface{})
		}
		return d.cookiesAPI.Set(ctx, ext, details)

	case "remove":
		var details map[string]interface{}
		if len(unwrapped) > 0 && string(unwrapped) != "null" {
			if err := json.Unmarshal(unwrapped, &details); err != nil {
				return nil, fmt.Errorf("invalid arguments for cookies.remove: %w", err)
			}
		}
		if details == nil {
			details = make(map[string]interface{})
		}
		return d.cookiesAPI.Remove(ctx, ext, details)

	case "getAllCookieStores":
		return d.cookiesAPI.GetAllCookieStores(ctx, ext)

	default:
		return nil, fmt.Errorf("unsupported cookies method: %s", method)
	}
}

// handlePageActionAPI handles pageAction.* API calls
func (d *Dispatcher) handlePageActionAPI(ctx context.Context, extID, method string, args json.RawMessage) (interface{}, error) {
	unwrapped := unwrapJSArgs(args)

	switch method {
	case "setIcon":
		var details map[string]interface{}
		if len(unwrapped) > 0 && string(unwrapped) != "null" {
			if err := json.Unmarshal(unwrapped, &details); err != nil {
				return nil, fmt.Errorf("invalid arguments for pageAction.setIcon: %w", err)
			}
		}
		if details == nil {
			details = make(map[string]interface{})
		}
		return nil, d.pageActionAPI.SetIcon(ctx, extID, details)

	case "setTitle":
		var details map[string]interface{}
		if len(unwrapped) > 0 && string(unwrapped) != "null" {
			if err := json.Unmarshal(unwrapped, &details); err != nil {
				return nil, fmt.Errorf("invalid arguments for pageAction.setTitle: %w", err)
			}
		}
		if details == nil {
			details = make(map[string]interface{})
		}
		return nil, d.pageActionAPI.SetTitle(ctx, extID, details)

	case "getTitle":
		var details map[string]interface{}
		if len(unwrapped) > 0 && string(unwrapped) != "null" {
			if err := json.Unmarshal(unwrapped, &details); err != nil {
				return nil, fmt.Errorf("invalid arguments for pageAction.getTitle: %w", err)
			}
		}
		if details == nil {
			details = make(map[string]interface{})
		}
		return d.pageActionAPI.GetTitle(ctx, extID, details)

	case "show":
		var tabID int64
		if err := json.Unmarshal(unwrapped, &tabID); err != nil {
			return nil, fmt.Errorf("invalid arguments for pageAction.show: %w", err)
		}
		return nil, d.pageActionAPI.Show(ctx, extID, tabID)

	case "hide":
		var tabID int64
		if err := json.Unmarshal(unwrapped, &tabID); err != nil {
			return nil, fmt.Errorf("invalid arguments for pageAction.hide: %w", err)
		}
		return nil, d.pageActionAPI.Hide(ctx, extID, tabID)

	default:
		return nil, fmt.Errorf("unsupported pageAction method: %s", method)
	}
}

// handleBrowserActionAPI handles browserAction.* API calls
func (d *Dispatcher) handleBrowserActionAPI(ctx context.Context, extID, method string, args json.RawMessage) (interface{}, error) {
	unwrapped := unwrapJSArgs(args)

	switch method {
	case "setBadgeText":
		var details map[string]interface{}
		if len(unwrapped) > 0 && string(unwrapped) != "null" {
			if err := json.Unmarshal(unwrapped, &details); err != nil {
				return nil, fmt.Errorf("invalid arguments for browserAction.setBadgeText: %w", err)
			}
		}
		if details == nil {
			details = make(map[string]interface{})
		}
		return nil, d.browserActionAPI.SetBadgeText(ctx, extID, details)

	case "setBadgeBackgroundColor":
		var details map[string]interface{}
		if len(unwrapped) > 0 && string(unwrapped) != "null" {
			if err := json.Unmarshal(unwrapped, &details); err != nil {
				return nil, fmt.Errorf("invalid arguments for browserAction.setBadgeBackgroundColor: %w", err)
			}
		}
		if details == nil {
			details = make(map[string]interface{})
		}
		return nil, d.browserActionAPI.SetBadgeBackgroundColor(ctx, extID, details)

	case "setPopup":
		var details map[string]interface{}
		if len(unwrapped) > 0 && string(unwrapped) != "null" {
			if err := json.Unmarshal(unwrapped, &details); err != nil {
				return nil, fmt.Errorf("invalid arguments for browserAction.setPopup: %w", err)
			}
		}
		if details == nil {
			details = make(map[string]interface{})
		}
		return nil, d.browserActionAPI.SetPopup(ctx, extID, details)

	case "getPopup":
		var details map[string]interface{}
		if len(unwrapped) > 0 && string(unwrapped) != "null" {
			if err := json.Unmarshal(unwrapped, &details); err != nil {
				return nil, fmt.Errorf("invalid arguments for browserAction.getPopup: %w", err)
			}
		}
		if details == nil {
			details = make(map[string]interface{})
		}
		return d.browserActionAPI.GetPopup(ctx, extID, details)

	case "openPopup":
		return nil, d.browserActionAPI.OpenPopup(ctx, extID)

	default:
		return nil, fmt.Errorf("unsupported browserAction method: %s", method)
	}
}

// handleAlarmsAPI handles alarms.* API calls
func (d *Dispatcher) handleAlarmsAPI(ctx context.Context, extID, method string, args json.RawMessage) (interface{}, error) {
	// Get extension for permission checking
	ext, ok := d.manager.GetExtension(extID)
	if !ok || ext == nil {
		return nil, fmt.Errorf("extension not found: %s", extID)
	}

	// Check alarms permission
	if !ext.Manifest.HasPermission("alarms") {
		return nil, fmt.Errorf("alarms.%s: extension does not have 'alarms' permission", method)
	}

	unwrapped := unwrapJSArgs(args)

	// Create event callback for onAlarm emission
	eventCallback := func(alarm api.Alarm) {
		// Emit onAlarm event to extension background context
		// Background contexts handle alarm events internally via their event system
		// TODO: Add alarm event dispatching to background context if needed
		log.Printf("[dispatcher] Alarm fired for %s: %+v (event dispatching not yet implemented)", extID, alarm)
	}

	switch method {
	case "create":
		// Parse args: [name (optional), alarmInfo (optional)]
		var name interface{}
		var alarmInfo map[string]interface{}

		// Try parsing as array first
		var argsArray []json.RawMessage
		if err := json.Unmarshal(unwrapped, &argsArray); err == nil {
			// Successfully parsed as array
			if len(argsArray) >= 1 {
				// First arg could be name (string) or alarmInfo (object)
				var nameStr string
				if err := json.Unmarshal(argsArray[0], &nameStr); err == nil {
					// It's a string name
					name = nameStr
					if len(argsArray) >= 2 {
						if err := json.Unmarshal(argsArray[1], &alarmInfo); err != nil {
							return nil, fmt.Errorf("invalid alarmInfo for alarms.create: %w", err)
						}
					}
				} else {
					// It's alarmInfo (no name provided)
					if err := json.Unmarshal(argsArray[0], &alarmInfo); err != nil {
						return nil, fmt.Errorf("invalid alarmInfo for alarms.create: %w", err)
					}
				}
			}
		} else {
			// Not an array - could be just a string name, or just alarmInfo object
			var nameStr string
			if err := json.Unmarshal(unwrapped, &nameStr); err == nil {
				// It's just a string name
				name = nameStr
			} else {
				// Try as alarmInfo object
				if err := json.Unmarshal(unwrapped, &alarmInfo); err != nil {
					return nil, fmt.Errorf("invalid arguments for alarms.create: %w", err)
				}
			}
		}

		return nil, d.alarmsAPI.Create(ctx, extID, name, alarmInfo, eventCallback)

	case "get":
		var name interface{}
		if len(unwrapped) > 0 && string(unwrapped) != "null" {
			if err := json.Unmarshal(unwrapped, &name); err != nil {
				return nil, fmt.Errorf("invalid arguments for alarms.get: %w", err)
			}
		}
		return d.alarmsAPI.Get(ctx, extID, name)

	case "getAll":
		return d.alarmsAPI.GetAll(ctx, extID)

	case "clear":
		var name interface{}
		if len(unwrapped) > 0 && string(unwrapped) != "null" {
			if err := json.Unmarshal(unwrapped, &name); err != nil {
				return nil, fmt.Errorf("invalid arguments for alarms.clear: %w", err)
			}
		}
		return d.alarmsAPI.Clear(ctx, extID, name)

	case "clearAll":
		return d.alarmsAPI.ClearAll(ctx, extID)

	default:
		return nil, fmt.Errorf("unsupported alarms method: %s", method)
	}
}

// handleMenusAPI handles menus.* and contextMenus.* API calls
func (d *Dispatcher) handleMenusAPI(ctx context.Context, extID, method string, args json.RawMessage) (interface{}, error) {
	// Get extension for permission checking
	ext, ok := d.manager.GetExtension(extID)
	if !ok || ext == nil {
		return nil, fmt.Errorf("extension not found: %s", extID)
	}

	// Check menus or contextMenus permission (either one works)
	if !ext.Manifest.HasPermission("menus") && !ext.Manifest.HasPermission("contextMenus") {
		return nil, fmt.Errorf("menus.%s: extension does not have 'menus' or 'contextMenus' permission", method)
	}

	unwrapped := unwrapJSArgs(args)

	switch method {
	case "create":
		var createProperties map[string]interface{}
		if err := json.Unmarshal(unwrapped, &createProperties); err != nil {
			return nil, fmt.Errorf("invalid arguments for menus.create: %w", err)
		}
		return d.menusAPI.Create(ctx, extID, createProperties)

	case "remove":
		var menuID string
		if err := json.Unmarshal(unwrapped, &menuID); err != nil {
			return nil, fmt.Errorf("invalid arguments for menus.remove: %w", err)
		}
		return nil, d.menusAPI.Remove(ctx, extID, menuID)

	case "removeAll":
		return nil, d.menusAPI.RemoveAll(ctx, extID)

	default:
		return nil, fmt.Errorf("unsupported menus method: %s", method)
	}
}

// handleNotificationsAPI handles notifications.* API calls
func (d *Dispatcher) handleNotificationsAPI(ctx context.Context, extID, method string, args json.RawMessage) (interface{}, error) {
	// Get extension for permission checking
	ext, ok := d.manager.GetExtension(extID)
	if !ok || ext == nil {
		return nil, fmt.Errorf("extension not found: %s", extID)
	}

	// Check notifications permission
	if !ext.Manifest.HasPermission("notifications") {
		return nil, fmt.Errorf("notifications.%s: extension does not have 'notifications' permission", method)
	}

	unwrapped := unwrapJSArgs(args)

	switch method {
	case "create":
		// Parse args: [notificationId (optional), options]
		var argsArray []json.RawMessage
		if err := json.Unmarshal(unwrapped, &argsArray); err != nil {
			// Not an array, try single options object
			var options map[string]interface{}
			if err := json.Unmarshal(unwrapped, &options); err == nil {
				return d.notificationsAPI.Create(ctx, extID, "", options)
			}
			return nil, fmt.Errorf("invalid arguments for notifications.create: %w", err)
		}

		// Parse arguments
		var notificationID string
		var options map[string]interface{}

		if len(argsArray) >= 1 {
			// First arg could be notificationId (string) or options (object)
			var idStr string
			if err := json.Unmarshal(argsArray[0], &idStr); err == nil {
				// It's a string ID
				notificationID = idStr
				if len(argsArray) >= 2 {
					if err := json.Unmarshal(argsArray[1], &options); err != nil {
						return nil, fmt.Errorf("invalid options for notifications.create: %w", err)
					}
				}
			} else {
				// It's options (no ID provided)
				if err := json.Unmarshal(argsArray[0], &options); err != nil {
					return nil, fmt.Errorf("invalid options for notifications.create: %w", err)
				}
			}
		}

		return d.notificationsAPI.Create(ctx, extID, notificationID, options)

	case "update":
		// Parse args: [notificationId, options]
		var argsArray []json.RawMessage
		if err := json.Unmarshal(unwrapped, &argsArray); err != nil || len(argsArray) < 2 {
			return nil, fmt.Errorf("invalid arguments for notifications.update: expected [notificationId, options]")
		}

		var notificationID string
		var options map[string]interface{}

		if err := json.Unmarshal(argsArray[0], &notificationID); err != nil {
			return nil, fmt.Errorf("invalid notificationId for notifications.update: %w", err)
		}

		if err := json.Unmarshal(argsArray[1], &options); err != nil {
			return nil, fmt.Errorf("invalid options for notifications.update: %w", err)
		}

		return d.notificationsAPI.Update(ctx, extID, notificationID, options)

	case "clear":
		var notificationID string
		if err := json.Unmarshal(unwrapped, &notificationID); err != nil {
			return nil, fmt.Errorf("invalid arguments for notifications.clear: %w", err)
		}
		return d.notificationsAPI.Clear(ctx, extID, notificationID)

	case "getAll":
		return d.notificationsAPI.GetAll(ctx, extID)

	default:
		return nil, fmt.Errorf("unsupported notifications method: %s", method)
	}
}

// handleDownloadsAPI handles downloads.* API calls
func (d *Dispatcher) handleDownloadsAPI(ctx context.Context, extID, method string, args json.RawMessage) (interface{}, error) {
	// Get extension for permission checking
	ext, ok := d.manager.GetExtension(extID)
	if !ok || ext == nil {
		return nil, fmt.Errorf("extension not found: %s", extID)
	}

	// Check downloads permission
	if !ext.Manifest.HasPermission("downloads") {
		return nil, fmt.Errorf("downloads.%s: extension does not have 'downloads' permission", method)
	}

	unwrapped := unwrapJSArgs(args)

	switch method {
	case "download":
		var options map[string]interface{}
		if err := json.Unmarshal(unwrapped, &options); err != nil {
			return nil, fmt.Errorf("invalid arguments for downloads.download: %w", err)
		}
		return d.downloadsAPI.Download(ctx, extID, ext.Manifest.Name, options)

	case "cancel":
		var downloadID float64
		if err := json.Unmarshal(unwrapped, &downloadID); err != nil {
			return nil, fmt.Errorf("invalid arguments for downloads.cancel: %w", err)
		}
		return nil, d.downloadsAPI.Cancel(ctx, int64(downloadID))

	case "open":
		var downloadID float64
		if err := json.Unmarshal(unwrapped, &downloadID); err != nil {
			return nil, fmt.Errorf("invalid arguments for downloads.open: %w", err)
		}
		return nil, d.downloadsAPI.Open(ctx, int64(downloadID))

	case "show":
		var downloadID float64
		if err := json.Unmarshal(unwrapped, &downloadID); err != nil {
			return nil, fmt.Errorf("invalid arguments for downloads.show: %w", err)
		}
		return nil, d.downloadsAPI.Show(ctx, int64(downloadID))

	case "removeFile":
		var downloadID float64
		if err := json.Unmarshal(unwrapped, &downloadID); err != nil {
			return nil, fmt.Errorf("invalid arguments for downloads.removeFile: %w", err)
		}
		return nil, d.downloadsAPI.RemoveFile(ctx, int64(downloadID))

	case "showDefaultFolder":
		return nil, d.downloadsAPI.ShowDefaultFolder(ctx)

	case "search":
		var query map[string]interface{}
		if err := json.Unmarshal(unwrapped, &query); err != nil {
			return nil, fmt.Errorf("invalid arguments for downloads.search: %w", err)
		}
		return d.downloadsAPI.Search(ctx, query)

	case "erase":
		var query map[string]interface{}
		if err := json.Unmarshal(unwrapped, &query); err != nil {
			return nil, fmt.Errorf("invalid arguments for downloads.erase: %w", err)
		}
		return d.downloadsAPI.Erase(ctx, query)

	default:
		return nil, fmt.Errorf("unsupported downloads method: %s", method)
	}
}

// handleCommandsAPI handles commands.* API calls
func (d *Dispatcher) handleCommandsAPI(ctx context.Context, extID, method string, args json.RawMessage) (interface{}, error) {
	unwrapped := unwrapJSArgs(args)

	switch method {
	case "getAll":
		return d.commandsAPI.GetAll(ctx, extID)

	case "reset":
		var commandName string
		if err := json.Unmarshal(unwrapped, &commandName); err != nil {
			return nil, fmt.Errorf("invalid arguments for commands.reset: %w", err)
		}
		return nil, d.commandsAPI.Reset(ctx, extID, commandName)

	case "update":
		var details map[string]interface{}
		if err := json.Unmarshal(unwrapped, &details); err != nil {
			return nil, fmt.Errorf("invalid arguments for commands.update: %w", err)
		}
		return nil, d.commandsAPI.Update(ctx, extID, details)

	default:
		return nil, fmt.Errorf("unsupported commands method: %s", method)
	}
}

// emitEventToView emits an event to a specific WebView
func (d *Dispatcher) emitEventToView(view *webkit.WebView, eventName string, data interface{}) {
	if view == nil {
		return
	}

	// Serialize data to JSON
	jsonData, err := json.Marshal(data)
	if err != nil {
		log.Printf("[dispatcher] Failed to marshal event data for %s: %v", eventName, err)
		return
	}

	// Construct JavaScript to emit the event
	script := fmt.Sprintf(`
		if (window.dispatchExtensionEvent) {
			window.dispatchExtensionEvent('%s', %s);
		}
	`, eventName, string(jsonData))

	// Inject the script
	if err := view.InjectScript(script); err != nil {
		log.Printf("[dispatcher] Failed to emit event %s: %v", eventName, err)
	}
}

// handlePermissionsAPI handles permissions.* API calls
func (d *Dispatcher) handlePermissionsAPI(ctx context.Context, extID, method string, args json.RawMessage) (interface{}, error) {
	// Get extension to access its permissions
	ext, ok := d.manager.GetExtension(extID)
	if !ok || ext == nil {
		return nil, fmt.Errorf("extension not found: %s", extID)
	}

	// Create permissions API dispatcher with this extension's permissions
	// In MV2, permissions array contains both regular permissions and URL patterns
	// Separate them into permissions and origins
	var manifestPerms []string
	var manifestOrigins []string

	if ext.Manifest != nil {
		for _, perm := range ext.Manifest.Permissions {
			// URL patterns start with scheme (http://, https://, file://, etc.) or wildcards
			if strings.Contains(perm, "://") || strings.HasPrefix(perm, "*") || strings.HasPrefix(perm, "<all_urls>") {
				manifestOrigins = append(manifestOrigins, perm)
			} else {
				manifestPerms = append(manifestPerms, perm)
			}
		}
	}

	permissionsAPI := api.NewPermissionsAPIDispatcher(manifestPerms, manifestOrigins)

	unwrapped := unwrapJSArgs(args)

	switch method {
	case "contains":
		var details map[string]interface{}
		if len(unwrapped) > 0 && string(unwrapped) != "null" {
			if err := json.Unmarshal(unwrapped, &details); err != nil {
				return nil, fmt.Errorf("invalid arguments for permissions.contains: %w", err)
			}
		}
		if details == nil {
			details = make(map[string]interface{})
		}
		return permissionsAPI.Contains(ctx, details)

	case "getAll":
		return permissionsAPI.GetAll(ctx)

	case "request":
		var details map[string]interface{}
		if len(unwrapped) > 0 && string(unwrapped) != "null" {
			if err := json.Unmarshal(unwrapped, &details); err != nil {
				return nil, fmt.Errorf("invalid arguments for permissions.request: %w", err)
			}
		}
		if details == nil {
			details = make(map[string]interface{})
		}
		return permissionsAPI.Request(ctx, details)

	case "remove":
		var details map[string]interface{}
		if len(unwrapped) > 0 && string(unwrapped) != "null" {
			if err := json.Unmarshal(unwrapped, &details); err != nil {
				return nil, fmt.Errorf("invalid arguments for permissions.remove: %w", err)
			}
		}
		if details == nil {
			details = make(map[string]interface{})
		}
		return permissionsAPI.Remove(ctx, details)

	default:
		return nil, fmt.Errorf("unsupported permissions method: %s", method)
	}
}

// sendSuccessReply sends a successful response to the web process
func (d *Dispatcher) sendSuccessReply(message *webkitgtk.UserMessage, data interface{}) {
	response := APIResponse{Data: data}
	jsonData, err := json.Marshal(response)
	if err != nil {
		log.Printf("[dispatcher] Failed to marshal response: %v", err)
		d.sendErrorReply(message, fmt.Sprintf("failed to marshal response: %v", err))
		return
	}

	variant := glib.NewVariantString(string(jsonData))
	reply := webkitgtk.NewUserMessage("", variant)
	message.SendReply(reply)
}

// sendErrorReply sends an error response to the web process
func (d *Dispatcher) sendErrorReply(message *webkitgtk.UserMessage, errorMsg string) {
	response := APIResponse{Error: errorMsg}
	jsonData, err := json.Marshal(response)
	if err != nil {
		log.Printf("[dispatcher] Failed to marshal error response: %v", err)
		// Fall back to simple error message
		variant := glib.NewVariantString(fmt.Sprintf(`{"error":"%s"}`, errorMsg))
		reply := webkitgtk.NewUserMessage("error", variant)
		message.SendReply(reply)
		return
	}

	variant := glib.NewVariantString(string(jsonData))
	reply := webkitgtk.NewUserMessage("", variant)
	message.SendReply(reply)
}
