package cef

import (
	"context"
	"sync"

	purecef "github.com/bnema/purego-cef/cef"

	"github.com/bnema/dumber/internal/logging"
)

const (
	popupOpenerExtraInfoEnabledKey     = "dumber.popup_opener.enabled"
	popupOpenerExtraInfoParentURIKey   = "dumber.popup_opener.parent_uri"
	popupOpenerExtraInfoBridgeNonceKey = "dumber.popup_opener.bridge_nonce"
	popupOpenerRenderScriptURL         = "dumber://popup-opener-renderer"
)

type popupOpenerRenderMetadata struct {
	ParentURI   string
	BridgeNonce string
}

var popupOpenerRenderExtraInfoBuilder = buildPopupOpenerRenderExtraInfo

func buildPopupOpenerRenderExtraInfo(parentURI, bridgeNonce string) purecef.DictionaryValue {
	if parentURI == "" || bridgeNonce == "" {
		return nil
	}
	extraInfo := purecef.DictionaryValueCreate()
	if extraInfo == nil {
		return nil
	}
	extraInfo.SetBool(popupOpenerExtraInfoEnabledKey, 1)
	extraInfo.SetString(popupOpenerExtraInfoParentURIKey, parentURI)
	extraInfo.SetString(popupOpenerExtraInfoBridgeNonceKey, bridgeNonce)
	return extraInfo
}

func decodePopupOpenerRenderMetadata(extraInfo purecef.DictionaryValue) (popupOpenerRenderMetadata, bool) {
	var metadata popupOpenerRenderMetadata
	if extraInfo == nil || extraInfo.GetBool(popupOpenerExtraInfoEnabledKey) != 1 {
		return metadata, false
	}
	metadata.ParentURI = extraInfo.GetString(popupOpenerExtraInfoParentURIKey)
	metadata.BridgeNonce = extraInfo.GetString(popupOpenerExtraInfoBridgeNonceKey)
	if metadata.ParentURI == "" || metadata.BridgeNonce == "" {
		return popupOpenerRenderMetadata{}, false
	}
	return metadata, true
}

type popupOpenerRenderProcessHandler struct {
	mu          sync.RWMutex
	byBrowserID map[int32]popupOpenerRenderMetadata
}

func newPopupOpenerRenderProcessHandler() *popupOpenerRenderProcessHandler {
	return &popupOpenerRenderProcessHandler{byBrowserID: make(map[int32]popupOpenerRenderMetadata)}
}

func (*popupOpenerRenderProcessHandler) OnWebKitInitialized() {}

func (h *popupOpenerRenderProcessHandler) OnBrowserCreated(browser purecef.Browser, extraInfo purecef.DictionaryValue) {
	if h == nil || browser == nil {
		return
	}
	metadata, ok := decodePopupOpenerRenderMetadata(extraInfo)
	if !ok {
		return
	}
	browserID := browser.GetIdentifier()
	h.mu.Lock()
	h.byBrowserID[browserID] = metadata
	h.mu.Unlock()
}

func (h *popupOpenerRenderProcessHandler) OnBrowserDestroyed(browser purecef.Browser) {
	if h == nil || browser == nil {
		return
	}
	browserID := browser.GetIdentifier()
	h.mu.Lock()
	delete(h.byBrowserID, browserID)
	h.mu.Unlock()
}

func (*popupOpenerRenderProcessHandler) GetLoadHandler() purecef.LoadHandler { return nil }

func (h *popupOpenerRenderProcessHandler) OnContextCreated(browser purecef.Browser, frame purecef.Frame, ctx purecef.V8Context) {
	if h == nil || browser == nil || frame == nil || !frame.IsMain() || ctx == nil || !ctx.IsValid() {
		return
	}
	browserID := browser.GetIdentifier()
	h.mu.RLock()
	metadata, ok := h.byBrowserID[browserID]
	h.mu.RUnlock()
	if !ok {
		return
	}

	script := buildPopupOpenerBridgeJS(metadata.BridgeNonce, metadata.ParentURI)
	if script == "" {
		return
	}
	if ctx.Enter() != 1 {
		logging.FromContext(context.Background()).Warn().
			Int32("browser_id", browserID).
			Msg("cef: popup opener renderer bridge could not enter V8 context")
		return
	}
	defer ctx.Exit()
	if ctx.Eval(script, popupOpenerRenderScriptURL, 0, nil, nil) != 1 {
		logging.FromContext(context.Background()).Warn().
			Int32("browser_id", browserID).
			Msg("cef: popup opener renderer bridge eval failed")
	}
}

func (*popupOpenerRenderProcessHandler) OnContextReleased(purecef.Browser, purecef.Frame, purecef.V8Context) {
}

func (*popupOpenerRenderProcessHandler) OnUncaughtException(
	purecef.Browser,
	purecef.Frame,
	purecef.V8Context,
	purecef.V8Exception,
	purecef.V8StackTrace,
) {
}

func (*popupOpenerRenderProcessHandler) OnFocusedNodeChanged(purecef.Browser, purecef.Frame, purecef.Domnode) {
}

func (*popupOpenerRenderProcessHandler) OnProcessMessageReceived(
	purecef.Browser,
	purecef.Frame,
	purecef.ProcessID,
	purecef.ProcessMessage,
) int32 {
	return 0
}
