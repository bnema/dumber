package cef

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"unsafe"

	purecef "github.com/bnema/purego-cef/cef"

	"github.com/bnema/dumber/internal/logging"
)

const (
	rendererBridgeMessageName                = "dumber.renderer_bridge"
	rendererBridgeActionExplicitTextCopy     = "explicit_text_copy"
	rendererBridgeActionFocusSync            = "focus_sync"
	rendererBridgeActionEditableFocusChanged = "editable_focus_changed"
	rendererBridgeActionPopupOpen            = "popup_open"
	rendererBridgeActionPopupNavigate        = "popup_navigate"
	rendererBridgeActionPopupClose           = "popup_close"
	rendererBridgeActionReady                = "bridge_ready"
	rendererBridgeExtensionName              = "dumber.renderer_bridge"
)

var newRendererBridgeProcessMessage = purecef.ProcessMessageCreate

type rendererBridgeProcessHandler struct {
	registerOnce  sync.Once
	bridgeHandler purecef.V8Handler
}

func (h *rendererBridgeProcessHandler) ensureExtensionRegistered() {
	if h == nil {
		return
	}
	h.registerOnce.Do(func() {
		if h.bridgeHandler == nil {
			h.bridgeHandler = &rendererBridgeV8Handler{}
		}
		if result := purecef.RegisterExtension(rendererBridgeExtensionName, rendererBridgeExtensionJS, h.bridgeHandler); result != 1 {
			logging.FromContext(context.Background()).Error().Int32("result", result).Msg("cef: failed to register renderer bridge extension")
		}
	})
}

func (h *rendererBridgeProcessHandler) OnWebKitInitialized() {
	h.ensureExtensionRegistered()
}

func (*rendererBridgeProcessHandler) OnBrowserCreated(_ purecef.Browser, _ purecef.DictionaryValue) {}
func (*rendererBridgeProcessHandler) OnBrowserDestroyed(_ purecef.Browser)                          {}
func (*rendererBridgeProcessHandler) GetLoadHandler() purecef.LoadHandler                           { return nil }
func (*rendererBridgeProcessHandler) OnContextCreated(_ purecef.Browser, _ purecef.Frame, _ purecef.V8Context) {
}
func (*rendererBridgeProcessHandler) OnContextReleased(_ purecef.Browser, _ purecef.Frame, _ purecef.V8Context) {
}
func (*rendererBridgeProcessHandler) OnUncaughtException(
	_ purecef.Browser, _ purecef.Frame, _ purecef.V8Context, _ purecef.V8Exception, _ purecef.V8StackTrace,
) {
}
func (*rendererBridgeProcessHandler) OnFocusedNodeChanged(_ purecef.Browser, frame purecef.Frame, node purecef.Domnode) {
	if frame == nil {
		return
	}
	message := newRendererBridgeProcessMessage(rendererBridgeMessageName)
	if message == nil {
		return
	}
	args := message.GetArgumentList()
	if args == nil {
		return
	}
	args.SetString(0, rendererBridgeActionEditableFocusChanged)
	if node != nil && node.IsEditable() {
		args.SetString(1, "1")
	} else {
		args.SetString(1, "0")
	}
	frame.SendProcessMessage(purecef.ProcessIDPidBrowser, message)
}
func (*rendererBridgeProcessHandler) OnProcessMessageReceived(
	_ purecef.Browser, _ purecef.Frame, _ purecef.ProcessID, _ purecef.ProcessMessage,
) int32 {
	return 0
}

type rendererBridgeV8Handler struct{}

func (*rendererBridgeV8Handler) Execute(
	name string, _ purecef.V8Value, arguments []purecef.V8Value, _ unsafe.Pointer, _ uintptr,
) int32 {
	if name != "Dispatch" || len(arguments) < 2 {
		return 0
	}
	if arguments[0] == nil || arguments[1] == nil || !arguments[0].IsString() || !arguments[1].IsString() {
		return 0
	}
	action := arguments[0].GetStringValue()
	payload := arguments[1].GetStringValue()
	if action == "" {
		return 0
	}

	ctx := purecef.V8ContextGetCurrentContext()
	if ctx == nil || !ctx.IsValid() {
		return 0
	}
	frame := ctx.GetFrame()
	if frame == nil {
		return 0
	}
	message := purecef.ProcessMessageCreate(rendererBridgeMessageName)
	if message == nil {
		return 0
	}
	args := message.GetArgumentList()
	if args == nil {
		return 0
	}
	args.SetString(0, action)
	args.SetString(1, payload)
	frame.SendProcessMessage(purecef.ProcessIDPidBrowser, message)
	return 1
}

func decodeRendererBridgeProcessMessage(message purecef.ProcessMessage) (string, string, bool) {
	if message == nil || message.GetName() != rendererBridgeMessageName {
		return "", "", false
	}
	args := message.GetArgumentList()
	if args == nil || args.GetSize() < 2 {
		return "", "", false
	}
	action := args.GetString(0)
	payload := args.GetString(1)
	if action == "" {
		return "", "", false
	}
	return action, payload, true
}

type rendererBridgeExplicitTextCopyPayload struct {
	Text   string `json:"text"`
	Action string `json:"action"`
}

type rendererBridgePopupOpenPayload struct {
	ProxyID            string `json:"proxy_id"`
	URL                string `json:"url"`
	FrameName          string `json:"frame_name"`
	UserGesture        bool   `json:"user_gesture"`
	NoJavaScriptAccess bool   `json:"no_javascript_access"`
}

type rendererBridgePopupNavigatePayload struct {
	ProxyID string `json:"proxy_id"`
	URL     string `json:"url"`
}

type rendererBridgePopupClosePayload struct {
	ProxyID string `json:"proxy_id"`
}

func decodeRendererBridgeExplicitTextCopyPayload(payload []byte) (rendererBridgeExplicitTextCopyPayload, error) {
	var req rendererBridgeExplicitTextCopyPayload
	if len(payload) == 0 {
		return req, fmt.Errorf("empty payload")
	}
	if err := json.Unmarshal(payload, &req); err != nil {
		return req, err
	}
	if req.Action == "" {
		req.Action = "copy"
	}
	if req.Text == "" {
		return req, fmt.Errorf("missing text")
	}
	return req, nil
}

func decodeRendererBridgePopupPayload[T any](payload []byte, proxyID func(T) string) (T, error) {
	var req T
	if len(payload) == 0 {
		return req, fmt.Errorf("empty payload")
	}
	if err := json.Unmarshal(payload, &req); err != nil {
		return req, err
	}
	if proxyID(req) == "" {
		return req, fmt.Errorf("missing proxy_id")
	}
	return req, nil
}

func decodeRendererBridgePopupOpenPayload(payload []byte) (rendererBridgePopupOpenPayload, error) {
	return decodeRendererBridgePopupPayload(payload, func(req rendererBridgePopupOpenPayload) string {
		return req.ProxyID
	})
}

func decodeRendererBridgePopupNavigatePayload(payload []byte) (rendererBridgePopupNavigatePayload, error) {
	return decodeRendererBridgePopupPayload(payload, func(req rendererBridgePopupNavigatePayload) string {
		return req.ProxyID
	})
}

func decodeRendererBridgePopupClosePayload(payload []byte) (rendererBridgePopupClosePayload, error) {
	return decodeRendererBridgePopupPayload(payload, func(req rendererBridgePopupClosePayload) string {
		return req.ProxyID
	})
}

func (e *Engine) handleEditableFocusBridge(browser purecef.Browser) {
	logger := logging.FromContext(e.currentContext())
	task := purecef.NewTask(cefTaskFunc(func() {
		if browser == nil {
			logger.Debug().Msg("cef: editable focus sync skipped — browser nil")
			return
		}
		host := browser.GetHost()
		if host == nil {
			logger.Debug().Msg("cef: editable focus sync skipped — host nil")
			return
		}
		syncWindowlessBrowserFocus(host)
		logger.Debug().Msg("cef: editable focus synchronized")
	}))
	if result := purecef.PostTask(purecef.ThreadIDTidUi, task); result != 1 {
		logger.Warn().Int32("result", result).Msg("cef: failed to post editable focus sync to UI thread")
	}
}
