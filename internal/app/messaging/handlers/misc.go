package handlers

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/bnema/dumber/internal/logging"
)

// ThemeMessage contains fields for theme operations.
type ThemeMessage struct {
	Value string `json:"value"`
}

// HandleTheme processes theme-related messages.
func HandleTheme(lastTheme *string, msg ThemeMessage) {
	if msg.Value != "" && msg.Value != *lastTheme {
		logging.Debug(fmt.Sprintf("[handlers] color-scheme changed: %s", msg.Value))
		*lastTheme = msg.Value
	}
}

// ConsoleMessage contains fields for console message operations.
type ConsoleMessage struct {
	Payload json.RawMessage `json:"payload"`
}

// HandleConsoleMessage processes console-message from JavaScript.
func HandleConsoleMessage(captureEnabled bool, msg ConsoleMessage) {
	if !captureEnabled {
		return
	}

	var consolePayload struct {
		Level   string `json:"level"`
		Message string `json:"message"`
		URL     string `json:"url"`
	}

	if err := json.Unmarshal(msg.Payload, &consolePayload); err != nil {
		logging.Error(fmt.Sprintf("[handlers] Failed to unmarshal console-message: %v", err))
		return
	}

	logging.CaptureWebKitLog(consolePayload.Message)
}

// HandleWebViewIDRequest responds to JavaScript requests for the webview ID.
func HandleWebViewIDRequest(c *Context) {
	if !c.IsWebViewReady() {
		logging.Warn("[handlers] Cannot provide webview ID - webview not available or destroyed")
		return
	}

	webViewID := c.WebView.ID()
	logging.Debug(fmt.Sprintf("[handlers] Sending webview ID %d to JavaScript", webViewID))

	if err := c.WebView.DispatchCustomEvent("dumber:webview-id", map[string]any{
		"webviewId": webViewID,
		"timestamp": time.Now().UnixMilli(),
	}); err != nil {
		logging.Error(fmt.Sprintf("[handlers] Failed to send webview ID: %v", err))
	}
}

// HandleGetSearchShortcuts sends search shortcuts configuration to JavaScript.
func HandleGetSearchShortcuts(c *Context) {
	if !c.IsWebViewReady() || c.BrowserService == nil {
		return
	}

	shortcuts, err := c.BrowserService.GetSearchShortcuts(c.Ctx())
	if err != nil {
		logging.Error(fmt.Sprintf("[handlers] Failed to get search shortcuts: %v", err))
		_ = c.InjectError("__dumber_search_shortcuts_error", "Failed to get search shortcuts")
		return
	}

	logging.Debug("[handlers] Sending search shortcuts to JavaScript")
	_ = c.InjectJSON("__dumber_search_shortcuts", shortcuts)
}

// HandleGetColorPalettes sends color palettes configuration to JavaScript.
func HandleGetColorPalettes(c *Context) {
	if !c.IsWebViewReady() || c.BrowserService == nil {
		return
	}

	palettes := c.BrowserService.GetColorPalettesForMessaging()

	logging.Debug("[handlers] Sending color palettes to JavaScript")
	_ = c.InjectJSON("__dumber_color_palettes", palettes)
}

// KeyboardBlockingMessage contains fields for keyboard blocking operations.
type KeyboardBlockingMessage struct {
	Action string `json:"action"`
}

// HandleKeyboardBlocking enables or disables keyboard event blocking for omnibox.
func HandleKeyboardBlocking(c *Context, msg KeyboardBlockingMessage) {
	if c.WebView == nil {
		return
	}

	var script string
	switch msg.Action {
	case "enable":
		script = "window.__dumber_enableKeyboardBlocking?.()"
	default:
		script = "window.__dumber_disableKeyboardBlocking?.()"
	}

	if err := c.WebView.InjectScript(script); err != nil {
		logging.Error(fmt.Sprintf("[handlers] Failed to inject keyboard blocking script: %v", err))
	}
}

// WailsBridgeMessage contains fields for Wails bridge operations.
type WailsBridgeMessage struct {
	ID      string          `json:"id"`
	Payload json.RawMessage `json:"payload"`
}

// WailsBridgePayload represents the inner payload structure.
type WailsBridgePayload struct {
	MethodID   uint32          `json:"methodID"`
	MethodName string          `json:"methodName"`
	Args       json.RawMessage `json:"args"`
}

// HandleWailsBridge processes Wails runtime bridge calls for homepage.
func HandleWailsBridge(c *Context, msg WailsBridgeMessage, hashGetRecentHistory, hashGetSearchShortcuts uint32) {
	if c.WebView == nil || c.BrowserService == nil {
		return
	}

	var p WailsBridgePayload
	if err := json.Unmarshal(msg.Payload, &p); err != nil {
		return
	}

	switch p.MethodID {
	case hashGetRecentHistory:
		var args []interface{}
		_ = json.Unmarshal(p.Args, &args)
		limit := 50
		if len(args) > 0 {
			if f, ok := args[0].(float64); ok {
				limit = int(f)
			}
		}
		entries, err := c.BrowserService.GetRecentHistory(c.Ctx(), limit)
		if err != nil {
			return
		}
		resp, _ := json.Marshal(entries)
		_ = c.WebView.InjectScript("window.__dumber_wails_resolve('" + msg.ID + "', " + string(resp) + ")")

	case hashGetSearchShortcuts:
		shortcuts, err := c.BrowserService.GetSearchShortcuts(c.Ctx())
		if err != nil {
			return
		}
		resp, _ := json.Marshal(shortcuts)
		_ = c.WebView.InjectScript("window.__dumber_wails_resolve('" + msg.ID + "', " + string(resp) + ")")

	default:
		_ = c.WebView.InjectScript("window.__dumber_wails_resolve('" + msg.ID + "', '{}')")
	}
}
