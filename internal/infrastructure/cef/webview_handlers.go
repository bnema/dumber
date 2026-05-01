package cef

import (
	"context"
	"strings"
	"sync"

	purecef "github.com/bnema/purego-cef/cef"
	cef2gtk "github.com/bnema/purego-cef2gtk"

	"github.com/bnema/dumber/internal/application/port"
	downloadutil "github.com/bnema/dumber/internal/domain/download"
	"github.com/bnema/dumber/internal/logging"
)

// Console message markers used by injected JavaScript for log filtering.
const (
	consoleMarkerVideoDiag = "[VIDEO-DIAG]"
	consoleMarkerAutoCopy  = "[AUTO-COPY]"
)

// handlerSet implements all CEF handler interfaces and dispatches events to the
// owning WebView. A single struct is used so that the Client's Get*Handler
// methods can return the same receiver, avoiding extra allocations.
type handlerSet struct {
	wv                *WebView
	renderHandlerOnce sync.Once
	renderHandler     purecef.RenderHandler
}

// Compile-time interface checks.
var (
	_ purecef.Client             = (*handlerSet)(nil)
	_ purecef.DisplayHandler     = (*handlerSet)(nil)
	_ purecef.LoadHandler        = (*handlerSet)(nil)
	_ purecef.LifeSpanHandler    = (*handlerSet)(nil)
	_ purecef.RequestHandler     = (*handlerSet)(nil)
	_ purecef.AudioHandler       = (*handlerSet)(nil)
	_ purecef.ContextMenuHandler = (*handlerSet)(nil)
	_ purecef.DownloadHandler    = (*handlerSet)(nil)
	_ purecef.FindHandler        = (*handlerSet)(nil)
)

// ===========================================================================
// Client
// ===========================================================================

func (h *handlerSet) GetAudioHandler() purecef.AudioHandler {
	// Always return the audio handler — without it, CEF has no audio output
	// path and refuses to start media decoding, breaking all video playback.
	// The handler accepts audio but discards packets (no-op) until a real
	// audio backend (PipeWire/PulseAudio) is wired up.
	return h
}
func (h *handlerSet) GetCommandHandler() purecef.CommandHandler         { return nil }
func (h *handlerSet) GetContextMenuHandler() purecef.ContextMenuHandler { return h }
func (h *handlerSet) GetDialogHandler() purecef.DialogHandler           { return nil }
func (h *handlerSet) GetDisplayHandler() purecef.DisplayHandler         { return h }
func (h *handlerSet) GetDownloadHandler() purecef.DownloadHandler {
	if h.downloadHandler() != nil {
		return h
	}
	return nil
}
func (h *handlerSet) GetDragHandler() purecef.DragHandler             { return nil }
func (h *handlerSet) GetFindHandler() purecef.FindHandler             { return h }
func (h *handlerSet) GetFocusHandler() purecef.FocusHandler           { return nil }
func (h *handlerSet) GetFrameHandler() purecef.FrameHandler           { return nil }
func (h *handlerSet) GetPermissionHandler() purecef.PermissionHandler { return nil }
func (h *handlerSet) GetJsdialogHandler() purecef.JsdialogHandler     { return nil }
func (h *handlerSet) GetKeyboardHandler() purecef.KeyboardHandler     { return nil }
func (h *handlerSet) GetLifeSpanHandler() purecef.LifeSpanHandler     { return h }
func (h *handlerSet) GetLoadHandler() purecef.LoadHandler             { return h }
func (h *handlerSet) GetPrintHandler() purecef.PrintHandler           { return nil }
func (h *handlerSet) GetRenderHandler() purecef.RenderHandler {
	if h == nil {
		return nil
	}
	h.renderHandlerOnce.Do(func() {
		h.renderHandler = newDumberRenderHandler(h.wv)
	})
	return h.renderHandler
}
func (h *handlerSet) GetRequestHandler() purecef.RequestHandler { return h }

func (h *handlerSet) OnProcessMessageReceived(
	browser purecef.Browser,
	_ purecef.Frame,
	_ purecef.ProcessID,
	message purecef.ProcessMessage,
) int32 {
	if h == nil || h.wv == nil || h.wv.engine == nil {
		return 0
	}
	action, payload, ok := decodeRendererBridgeProcessMessage(message)
	if !ok {
		return 0
	}

	log := logging.FromContext(h.wv.ctx)
	log.Debug().
		Str("action", action).
		Int("payload_len", len(payload)).
		Msg("cef: renderer bridge message received")

	switch action {
	case rendererBridgeActionExplicitTextCopy:
		req, err := decodeRendererBridgeExplicitTextCopyPayload([]byte(payload))
		if err != nil {
			log.Debug().Str("action", action).Msg("cef: invalid explicit copy payload")
			return 1
		}
		h.wv.engine.handleExplicitClipboardBridgeText(h.wv.id, req.Action, req.Text)
	case rendererBridgeActionEditableFocusChanged:
		h.wv.setEditableFocus(payload == "1" || strings.EqualFold(payload, "true"))
	case rendererBridgeActionFocusSync:
		h.wv.engine.handleEditableFocusBridge(browser)
	case rendererBridgeActionPopupOpen:
		req, err := decodeRendererBridgePopupOpenPayload([]byte(payload))
		if err != nil {
			log.Debug().Str("action", action).Msg("cef: invalid popup_open payload")
			return 1
		}
		h.wv.handleSyntheticPopupOpen(req.URL, req.FrameName, req.ProxyID, req.UserGesture, req.NoJavaScriptAccess)
	case rendererBridgeActionPopupNavigate:
		req, err := decodeRendererBridgePopupNavigatePayload([]byte(payload))
		if err != nil {
			log.Debug().Str("action", action).Msg("cef: invalid popup_navigate payload")
			return 1
		}
		h.wv.handleSyntheticPopupNavigate(req.ProxyID, req.URL)
	case rendererBridgeActionPopupClose:
		req, err := decodeRendererBridgePopupClosePayload([]byte(payload))
		if err != nil {
			log.Debug().Str("action", action).Msg("cef: invalid popup_close payload")
			return 1
		}
		h.wv.handleSyntheticPopupClose(req.ProxyID)
	case rendererBridgeActionReady:
		log.Debug().
			Str("frame_url", logging.TruncateURL(payload, logging.PermissionLogURLMaxLen)).
			Msg("cef: renderer bridge ready")
	default:
		log.Debug().Str("action", action).Msg("cef: unknown renderer bridge action")
	}
	return 1
}

// ===========================================================================
// RenderHandler methods live in render_handler_adapter.go. handlerSet keeps
// Dumber's non-render CEF handlers and returns a delegating render handler from
// GetRenderHandler.
// ===========================================================================

// ===========================================================================
// DisplayHandler (13 methods)
// ===========================================================================

// OnAddressChange updates the cached URI when the main frame navigates.
func (h *handlerSet) OnAddressChange(_ purecef.Browser, frame purecef.Frame, url string) {
	if frame != nil && frame.IsMain() {
		if h.wv != nil && h.wv.ctx != nil {
			logging.FromContext(h.wv.ctx).Debug().
				Str("url", logging.TruncateURL(url, logging.PermissionLogURLMaxLen)).
				Msg("cef: OnAddressChange")
		}
		h.wv.updateURI(url)
	}
}

// OnTitleChange updates the cached title.
func (h *handlerSet) OnTitleChange(_ purecef.Browser, title string) {
	h.wv.updateTitle(title)
}

func (h *handlerSet) OnFaviconUrlchange(_ purecef.Browser, _ purecef.StringList) {}

// OnFullscreenModeChange toggles the fullscreen atomic and fires callbacks.
func (h *handlerSet) OnFullscreenModeChange(_ purecef.Browser, fullscreen int32) {
	entering := fullscreen != 0
	h.wv.fullscreen.Store(entering)

	h.wv.mu.RLock()
	cb := h.wv.callbacks
	h.wv.mu.RUnlock()
	if cb == nil {
		return
	}
	if entering && cb.OnEnterFullscreen != nil {
		h.wv.runOnGTK(func() {
			cb.OnEnterFullscreen()
		})
	}
	if !entering && cb.OnLeaveFullscreen != nil {
		h.wv.runOnGTK(func() {
			cb.OnLeaveFullscreen()
		})
	}
}

func (h *handlerSet) OnTooltip(_ purecef.Browser, _ uintptr) int32 { return 0 }

// OnStatusMessage fires the OnLinkHover callback with the status text
// and caches the hover URI for middle-click interception.
func (h *handlerSet) OnStatusMessage(_ purecef.Browser, value string) {
	h.wv.updateHoverURI(value)

	h.wv.mu.RLock()
	cb := h.wv.callbacks
	h.wv.mu.RUnlock()
	if cb != nil && cb.OnLinkHover != nil {
		h.wv.runOnGTK(func() {
			cb.OnLinkHover(value)
		})
	}
}

func (h *handlerSet) OnConsoleMessage(
	_ purecef.Browser,
	level purecef.LogSeverity,
	message, source string,
	line int32,
) int32 {
	if h.wv != nil && h.wv.ctx != nil &&
		(strings.Contains(message, consoleMarkerVideoDiag) ||
			strings.Contains(message, consoleMarkerAutoCopy)) {
		log := logging.FromContext(h.wv.ctx).With().
			Str("component", "cef-console").
			Str("source", source).
			Int32("line", line).
			Logger()
		switch level {
		case purecef.LogSeverityLogseverityError, purecef.LogSeverityLogseverityFatal:
			log.Error().Str("message", message).Msg("cef: console message")
		case purecef.LogSeverityLogseverityWarning:
			log.Warn().Str("message", message).Msg("cef: console message")
		default:
			log.Info().Str("message", message).Msg("cef: console message")
		}
	}
	return 0
}

func (h *handlerSet) OnAutoResize(_ purecef.Browser, _ *purecef.Size) int32 { return 0 }

// OnLoadingProgressChange updates the cached progress value.
func (h *handlerSet) OnLoadingProgressChange(_ purecef.Browser, progress float64) {
	if h.wv != nil && h.wv.ctx != nil {
		logging.FromContext(h.wv.ctx).Trace().
			Float64("progress", progress).
			Msg("cef: OnLoadingProgressChange")
	}
	h.wv.updateProgress(progress)
}

func (h *handlerSet) OnCursorChange(
	_ purecef.Browser,
	_ uintptr,
	cursorType purecef.CursorType,
	_ *purecef.CursorInfo,
) int32 {
	if h == nil || h.wv == nil {
		return 0
	}
	name := cefCursorToGDKName(cursorType)
	h.wv.runOnGTK(func() {
		if h.wv.viewBridge != nil {
			h.wv.viewBridge.SetCursorFromName(name)
		}
	})
	return 1 // handled
}

func (h *handlerSet) OnMediaAccessChange(_ purecef.Browser, _, _ int32) {}

func (h *handlerSet) OnContentsBoundsChange(_ purecef.Browser, _ *purecef.Rect) int32 { return 0 }

func (h *handlerSet) GetRootWindowScreenRect(_ purecef.Browser, _ *purecef.Rect) int32 { return 0 }

// ===========================================================================
// LoadHandler (4 methods)
// ===========================================================================

// OnLoadingStateChange updates the loading/navigation state cache and fires callbacks.
func (h *handlerSet) OnLoadingStateChange(_ purecef.Browser, isloading, cangoback, cangoforward int32) {
	loading := isloading != 0
	h.wv.updateLoadState(loading, cangoback != 0, cangoforward != 0)
	if h.wv != nil && h.wv.ctx != nil {
		h.wv.mu.RLock()
		uri := h.wv.uri
		progress := h.wv.progress
		pendingURI := h.wv.pendingURI
		h.wv.mu.RUnlock()
		logging.FromContext(h.wv.ctx).Debug().
			Bool("loading", loading).
			Bool("can_go_back", cangoback != 0).
			Bool("can_go_forward", cangoforward != 0).
			Float64("progress", progress).
			Str("uri", logging.TruncateURL(uri, logging.PermissionLogURLMaxLen)).
			Str("pending_uri", logging.TruncateURL(pendingURI, logging.PermissionLogURLMaxLen)).
			Msg("cef: OnLoadingStateChange")
	}

	if !loading && h.wv.viewBridge != nil {
		h.wv.runOnGTK(func() {
			if h.wv.viewBridge != nil && h.wv.viewBridge.HasFocus() {
				h.wv.mu.RLock()
				host := h.wv.host
				h.wv.mu.RUnlock()
				if host != nil {
					syncWindowlessBrowserFocus(host)
				}
			}
		})
	}

	h.wv.mu.RLock()
	cb := h.wv.callbacks
	h.wv.mu.RUnlock()
	if cb != nil && cb.OnLoadChanged != nil {
		if loading {
			h.wv.runOnGTK(func() {
				cb.OnLoadChanged(port.LoadStarted)
			})
		} else {
			h.wv.runOnGTK(func() {
				cb.OnLoadChanged(port.LoadFinished)
			})
		}
	}
}

// OnLoadStart fires LoadCommitted for main frame navigations.
// In CEF this callback runs after navigation commit, so it is the closest
// equivalent to WebKit's LoadCommitted event.
func (h *handlerSet) OnLoadStart(_ purecef.Browser, frame purecef.Frame, _ purecef.TransitionType) {
	if frame == nil || !frame.IsMain() {
		return
	}
	bridgeNonce := h.wv.ensureBridgeNonce()
	if openerBridgeScript := h.wv.popupOpenerBridgeScript(bridgeNonce); openerBridgeScript != "" {
		if h.wv != nil && h.wv.ctx != nil {
			parentURI, active, blocked := h.wv.popupOpenerBridgeState()
			logging.FromContext(h.wv.ctx).Debug().
				Uint64("webview_id", uint64(h.wv.id)).
				Str("url", logging.TruncateURL(frame.GetURL(), logging.PermissionLogURLMaxLen)).
				Str("parent_uri", logging.TruncateURL(parentURI, logging.PermissionLogURLMaxLen)).
				Bool("bridge_active", active).
				Bool("bridge_blocked", blocked).
				Msg("cef: injecting popup opener bridge")
		}
		frame.ExecuteJavaScript(openerBridgeScript, frame.GetURL(), 0)
	}
	if h.wv != nil && h.wv.ctx != nil {
		logging.FromContext(h.wv.ctx).Debug().
			Str("url", logging.TruncateURL(frame.GetURL(), logging.PermissionLogURLMaxLen)).
			Msg("cef: OnLoadStart")
	}
	h.wv.updateURI(frame.GetURL())
	h.wv.mu.RLock()
	cb := h.wv.callbacks
	h.wv.mu.RUnlock()
	if cb != nil && cb.OnLoadChanged != nil {
		h.wv.runOnGTK(func() {
			cb.OnLoadChanged(port.LoadCommitted)
		})
	}
}

// OnLoadEnd resets crash count and injects content scripts for the main frame.
func (h *handlerSet) OnLoadEnd(_ purecef.Browser, frame purecef.Frame, httpStatusCode int32) {
	log := logging.FromContext(h.wv.ctx)
	log.Debug().
		Bool("frame_nil", frame == nil).
		Bool("is_main", frame != nil && frame.IsMain()).
		Int32("http_status", httpStatusCode).
		Msg("cef: OnLoadEnd")
	if frame == nil || !frame.IsMain() {
		return
	}
	// Successful load — reset the consecutive crash counter.
	h.wv.crashCount.Store(0)

	// If a queued startup navigation is still pending after about:blank finished,
	// replay it now that the initial main-frame load completed.
	frameURL := frame.GetURL()
	if pendingURI := h.wv.pendingNavigationURI(); pendingURI != "" && !pendingURIEquivalent(frameURL, pendingURI) {
		if strings.EqualFold(strings.TrimSpace(frameURL), "about:blank") {
			log.Debug().
				Str("pending_uri", logging.TruncateURL(pendingURI, logging.PermissionLogURLMaxLen)).
				Msg("cef: replaying pending navigation after about:blank load end")
			h.wv.schedulePendingNavigationReplay(0)
		}
	}

	// Inject scripts and styles after page load.
	// Must run on GTK thread — OnLoadEnd fires on the CEF IO thread,
	// and JavaScript injection requires the main thread.
	if h.wv.engine != nil && h.wv.engine.contentInj != nil {
		h.wv.runOnGTK(func() {
			h.wv.engine.contentInj.onLoadEnd(h.wv)
		})
	}

	// NOTE: We intentionally do NOT dispatch LoadFinished or ProgressChanged(1.0)
	// here. OnLoadEnd is a per-frame event, and during cross-site navigations
	// (process swap), CEF fires OnLoadEnd for the OLD main frame before the new
	// page finishes loading. Dispatching LoadFinished here caused the progress
	// bar to hide prematurely while the new page was still loading.
	//
	// OnLoadingStateChange(isLoading=false) is the correct browser-level signal
	// for load completion — CEF guarantees it fires AFTER all OnLoadEnd calls.
	// CEF's OnLoadingProgressChange also provides progress=1.0 at completion.
}

// OnLoadError is a no-op.
func (h *handlerSet) OnLoadError(_ purecef.Browser, _ purecef.Frame, _ purecef.Errorcode, _, _ string) {
}

// ===========================================================================
// LifeSpanHandler (6 methods)
// ===========================================================================

// OnBeforePopup intercepts popup requests (target="_blank", window.open).
// When the coordinator returns a related CEF popup shell we hand that shell's
// client back to CEF and allow native popup creation so opener semantics are
// preserved. Otherwise we keep blocking and let the coordinator's fallback
// pane handle the navigation.
func (h *handlerSet) OnBeforePopup(
	_ purecef.Browser, _ purecef.Frame, popupID int32, targetURL, targetFrameName string,
	_ purecef.WindowOpenDisposition, userGesture int32, _ *purecef.PopupFeatures,
	windowInfo *purecef.WindowInfo, clientSlot *purecef.RawClientWriteSlot, settings *purecef.BrowserSettings,
	_ *purecef.DictionaryValue, noJavaScriptAccess *bool,
) bool {
	if targetURL == "" {
		return true
	}

	h.wv.mu.RLock()
	cb := h.wv.callbacks
	h.wv.mu.RUnlock()
	if cb == nil || cb.OnCreate == nil {
		return true
	}

	requestNoJavaScriptAccess := false
	if noJavaScriptAccess != nil {
		requestNoJavaScriptAccess = *noJavaScriptAccess
	}
	var popup port.WebView
	h.wv.runOnGTKSync(func() {
		popup = cb.OnCreate(port.PopupRequest{
			TargetURI:          targetURL,
			FrameName:          targetFrameName,
			IsUserGesture:      userGesture != 0,
			NoJavaScriptAccess: requestNoJavaScriptAccess,
			ParentViewID:       h.wv.id,
		})
	})

	cefPopup, ok := popup.(*WebView)
	if !ok || cefPopup == nil {
		return true
	}
	cefPopup.setPopupNoJavaScriptAccess(requestNoJavaScriptAccess)
	if cefPopup.prepareNativePopup(popupID, targetURL, windowInfo, clientSlot, settings) {
		return false
	}

	cefPopup.discardNativePopupCandidate()
	logging.FromContext(h.currentContext()).Debug().
		Int32("popup_id", popupID).
		Str("target_url", logging.TruncateURL(targetURL, logging.PermissionLogURLMaxLen)).
		Msg("cef: native popup bridge unavailable, blocking CEF popup and using popup shell browser creation")
	return true
}

func (h *handlerSet) OnBeforePopupAborted(_ purecef.Browser, popupID int32) {
	if popup := h.wv.takePendingNativePopup(popupID); popup != nil {
		popup.handleNativePopupAborted()
	}
}

func (h *handlerSet) OnBeforeDevToolsPopup(
	_ purecef.Browser, _ *purecef.WindowInfo, _ *purecef.RawClientWriteSlot,
	_ *purecef.BrowserSettings, _ *purecef.DictionaryValue, _ *bool,
) {
}

// OnAfterCreated stores the browser and host references and enables input.
func (h *handlerSet) OnAfterCreated(browser purecef.Browser) {
	log := logging.FromContext(h.wv.ctx)
	if browser == nil {
		log.Warn().Msg("cef: OnAfterCreated called with nil browser")
		return
	}
	browserID := browser.GetIdentifier()
	log.Debug().Int32("browser_id", browserID).Msg("cef: OnAfterCreated")

	host := browser.GetHost()
	if host == nil {
		log.Warn().Msg("cef: OnAfterCreated returned nil host")
		return
	}

	state := h.attachAfterCreatedBrowser(browser, host, browserID)
	if state.closeDuplicate {
		log.Warn().
			Int32("browser_id", browserID).
			Int32("existing_browser_id", state.duplicateBrowserID).
			Msg("cef: duplicate popup browser attached after shell already had a browser; closing duplicate")
		host.CloseBrowser(1)
		return
	}

	h.finishAfterCreated(browser, host, state)
}

func (h *handlerSet) attachAfterCreatedBrowser(
	browser purecef.Browser,
	host purecef.BrowserHost,
	browserID int32,
) afterCreatedState {
	state := afterCreatedState{}
	h.wv.mu.Lock()

	if existing := h.wv.browser; existing != nil {
		existingID := existing.GetIdentifier()
		if existingID != 0 && existingID != browserID {
			state.closeDuplicate = true
			state.duplicateBrowserID = existingID
			h.wv.mu.Unlock()
			return state
		}
	}

	h.wv.browser = browser
	h.wv.host = host
	h.wv.pendingCreate = nil
	bridge := h.wv.viewBridge
	h.wv.inputAttached = bridge == nil
	state.inputAttached = h.wv.inputAttached
	if h.wv.findCtrl != nil {
		h.wv.findCtrl.setHost(host)
	}
	state.nativePopupParent = h.wv.nativePopupParent
	state.nativePopupID = h.wv.nativePopupID
	h.wv.nativePopupParent = nil
	h.wv.nativePopupID = 0
	h.wv.nativePopupFallbackStarted = false
	state.hasPendingNavigation = strings.TrimSpace(h.wv.pendingURI) != ""
	h.wv.mu.Unlock()

	if bridge != nil {
		wv := h.wv
		wv.runOnGTK(func() {
			if wv.destroyed.Load() || wv.viewBridge == nil {
				wv.handleInputAttachFailure(ErrAdapterDestroyed, host)
				return
			}
			if err := wv.viewBridge.AttachInput(host, cef2gtk.InputOptions{
				Scale: wv.viewBridgeScale(),
				OnMiddleClick: func(_, _ float64) bool {
					return wv.handleMiddleClickFromBridge()
				},
				SelectionText: wv.selectedTextSnapshot,
				OnClipboardShortcut: func(action, text string) {
					if wv.engine != nil {
						wv.engine.handleExplicitClipboardBridgeText(wv.id, action, text)
					}
				},
			}); err != nil {
				wv.handleInputAttachFailure(err, host)
				return
			}
			if wv.viewBridge != nil && wv.viewBridge.HasFocus() {
				syncWindowlessBrowserFocus(host)
			} else {
				host.Invalidate(purecef.PaintElementTypePetView)
			}
			wv.markInputAttached()
		})
	}

	// Mark browser as visible — CEF OSR starts in hidden state and suppresses
	// painting/caret updates until explicitly told the browser is shown.
	host.WasHidden(0)
	return state
}

func (wv *WebView) handleInputAttachFailure(err error, host purecef.BrowserHost) {
	if wv == nil || err == nil {
		return
	}
	if wv.ctx != nil {
		logging.FromContext(wv.ctx).Warn().Err(err).Msg("cef: failed to attach input to cef2gtk bridge")
	}
	if host != nil && !wv.destroyed.Load() {
		host.CloseBrowser(1)
		return
	}
	wv.runCloseCallbacks()
}

func (h *handlerSet) finishAfterCreated(
	browser purecef.Browser,
	host purecef.BrowserHost,
	state afterCreatedState,
) {
	if h.wv.engine != nil {
		h.wv.engine.recordBrowserAfterCreated(browser)
		h.wv.engine.registerWebView(h.wv)
		h.wv.engine.bindBrowserWebView(browser, h.wv)
	}
	h.wv.stopNativePopupFallbackTimer()
	if state.inputAttached {
		// Request an initial OSR frame even before the first real navigation
		// commits. This restores the about:blank warm-up paint that the stable
		// startup path relied on and prevents the rendering bridge from staying idle.
		host.Invalidate(purecef.PaintElementTypePetView)
	}
	if state.hasPendingNavigation {
		h.wv.schedulePendingNavigationReplay(0)
	}
	if state.nativePopupParent != nil && state.nativePopupID != 0 {
		state.nativePopupParent.clearPendingNativePopup(state.nativePopupID, h.wv)
	}
	h.wv.scheduleStartBeginFrameLoop()
	h.wv.fireReadyToShow()
}

type afterCreatedState struct {
	inputAttached        bool
	hasPendingNavigation bool
	nativePopupParent    *WebView
	nativePopupID        int32
	duplicateBrowserID   int32
	closeDuplicate       bool
}

// DoClose returns false to allow the default close behavior.
func (h *handlerSet) DoClose(_ purecef.Browser) bool {
	return false
}

// OnBeforeClose fires the OnClose callback.
func (h *handlerSet) OnBeforeClose(browser purecef.Browser) {
	if h.wv.engine != nil {
		browserID := int32(0)
		if browser != nil {
			browserID = browser.GetIdentifier()
		}
		h.wv.engine.unregisterWebView(h.wv, browserID)
	}
	h.wv.mu.Lock()
	h.wv.browser = nil
	h.wv.host = nil
	h.wv.inputAttached = false
	if h.wv.findCtrl != nil {
		h.wv.findCtrl.setHost(nil)
	}
	h.wv.mu.Unlock()
	if h.wv.destroyed.Load() {
		h.wv.destroyViewBridgeOnGTKAsync()
	} else if h.wv.viewBridge != nil {
		wv := h.wv
		wv.runOnGTK(func() {
			if wv.viewBridge == nil {
				return
			}
			if err := wv.viewBridge.SetInputHost(nil); err != nil && wv.ctx != nil {
				logging.FromContext(wv.ctx).Warn().Err(err).Msg("cef: failed to clear cef2gtk input host")
			}
		})
	}
	h.wv.scheduleStopBeginFrameLoop()

	h.wv.mu.RLock()
	cb := h.wv.callbacks
	h.wv.mu.RUnlock()
	if cb != nil && cb.OnClose != nil {
		h.wv.runOnGTK(func() {
			cb.OnClose()
		})
	}
	h.wv.runCloseCallbacks()
}

// ===========================================================================
// RequestHandler (11 methods)
// ===========================================================================

func (h *handlerSet) OnBeforeBrowse(browser purecef.Browser, frame purecef.Frame, request purecef.Request, _, _ int32) bool {
	if frame == nil || !frame.IsMain() || request == nil {
		return false
	}

	handler := h.downloadHandler()
	if handler == nil || !strings.EqualFold(request.GetMethod(), "GET") {
		return false
	}

	url := request.GetURL()
	if !downloadutil.ShouldForceDownloadForURI(url) || browser == nil {
		return false
	}

	host := browser.GetHost()
	if host == nil {
		return false
	}

	logging.FromContext(h.currentContext()).Debug().
		Str("url", logging.TruncateURL(url, maxSchemeTruncatedURLLength)).
		Msg("cef: forcing download for navigation")

	host.StartDownload(url)
	return true
}

func (h *handlerSet) CanDownload(browser purecef.Browser, url, requestMethod string) bool {
	handler := h.downloadHandler()
	if handler == nil {
		return false
	}
	return handler.canDownload(browser, url, requestMethod)
}

func (h *handlerSet) OnBeforeDownload(
	browser purecef.Browser,
	downloadItem purecef.DownloadItem,
	suggestedName string,
	callback purecef.BeforeDownloadCallback,
) bool {
	handler := h.downloadHandler()
	if handler == nil {
		return false
	}
	return handler.onBeforeDownload(h.currentContext(), browser, downloadItem, suggestedName, callback)
}

func (h *handlerSet) OnDownloadUpdated(
	_ purecef.Browser,
	downloadItem purecef.DownloadItem,
	callback purecef.DownloadItemCallback,
) {
	handler := h.downloadHandler()
	if handler == nil {
		return
	}
	handler.onDownloadUpdated(h.currentContext(), downloadItem, callback)
}

func (h *handlerSet) downloadHandler() *downloadHandler {
	if h == nil || h.wv == nil || h.wv.engine == nil {
		return nil
	}
	return h.wv.engine.currentDownloadHandler()
}

func (h *handlerSet) currentContext() context.Context {
	if h == nil || h.wv == nil || h.wv.ctx == nil {
		return context.Background()
	}
	return h.wv.ctx
}

func (h *handlerSet) OnOpenUrlfromTab(
	_ purecef.Browser,
	_ purecef.Frame,
	_ string,
	_ purecef.WindowOpenDisposition,
	_ int32,
) int32 {
	return 0
}

func (h *handlerSet) GetResourceRequestHandler(
	_ purecef.Browser, _ purecef.Frame, _ purecef.Request,
	_, _ int32, _ string, _ *int32,
) purecef.ResourceRequestHandler {
	return nil
}

func (h *handlerSet) GetAuthCredentials(
	_ purecef.Browser, _ string, _ int32, _ string, _ int32,
	_, _ string, _ purecef.AuthCallback,
) int32 {
	return 0
}

func (h *handlerSet) OnCertificateError(
	_ purecef.Browser,
	_ purecef.Errorcode,
	_ string,
	_ purecef.Sslinfo,
	_ purecef.Callback,
) int32 {
	return 0
}

func (h *handlerSet) OnSelectClientCertificate(
	_ purecef.Browser, _ int32, _ string, _ int32,
	_ []purecef.X509Certificate, _ purecef.SelectClientCertificateCallback,
) int32 {
	return 0
}

func (h *handlerSet) OnRenderViewReady(_ purecef.Browser) {}

func (h *handlerSet) OnRenderProcessUnresponsive(_ purecef.Browser, _ purecef.UnresponsiveProcessCallback) int32 {
	return 0
}

func (h *handlerSet) OnRenderProcessResponsive(_ purecef.Browser) {}

// maxConsecutiveCrashes caps how many times OnRenderProcessTerminated will
// forward the event before suppressing further notifications. This prevents
// infinite crash → redirect → crash loops.
const maxConsecutiveCrashes = 3

// OnRenderProcessTerminated fires the OnWebProcessTerminated callback with a mapped reason.
func (h *handlerSet) OnRenderProcessTerminated(_ purecef.Browser, status purecef.TerminationStatus, _ int32, _ string) {
	if h.wv.crashCount.Add(1) > maxConsecutiveCrashes {
		return // suppress to break the loop
	}

	var reason port.WebProcessTerminationReason
	var label string
	switch status {
	case purecef.TerminationStatusTsAbnormalTermination:
		reason = port.WebProcessTerminationCrashed
		label = "abnormal termination"
	case purecef.TerminationStatusTsProcessWasKilled:
		reason = port.WebProcessTerminationByAPI
		label = "killed"
	case purecef.TerminationStatusTsProcessCrashed:
		reason = port.WebProcessTerminationCrashed
		label = "crashed"
	case purecef.TerminationStatusTsProcessOom:
		reason = port.WebProcessTerminationExceededMemory
		label = "out of memory"
	default:
		reason = port.WebProcessTerminationUnknown
		label = "unknown"
	}

	h.wv.mu.RLock()
	cb := h.wv.callbacks
	uri := h.wv.uri
	h.wv.mu.RUnlock()
	if cb != nil && cb.OnWebProcessTerminated != nil {
		h.wv.runOnGTK(func() {
			cb.OnWebProcessTerminated(reason, label, uri)
		})
	}
}

func (h *handlerSet) OnDocumentAvailableInMainFrame(_ purecef.Browser) {}

// ===========================================================================
// AudioHandler (5 methods)
// ===========================================================================

func (h *handlerSet) GetAudioParameters(_ purecef.Browser, _ *purecef.AudioParameters) int32 {
	return 1 // proceed with defaults
}

// OnAudioStreamStarted handles the start of an audio stream.
// The third callback argument is the channel count (not frames-per-buffer);
// frames-per-buffer comes from params.FramesPerBuffer.
func (h *handlerSet) OnAudioStreamStarted(_ purecef.Browser, params *purecef.AudioParameters, channels int32) {
	// Validate params and factory first
	if params == nil || h.wv.audioOutputFactory == nil {
		return
	}

	// Build format from CEF parameters
	format := h.buildAudioStreamFormat(params, channels)

	if h.wv.ctx != nil {
		logging.FromContext(h.wv.ctx).Info().
			Int("sample_rate", format.SampleRate).
			Int("channels", format.ChannelCount).
			Int("frames_per_buffer", format.FramesPerBuffer).
			Int32("channel_layout", params.ChannelLayout).
			Msg("cef: OnAudioStreamStarted")
	}

	// Reset packet counters for the new stream
	h.wv.audioPacketCount.Store(0)
	h.wv.audioWriteCount.Store(0)

	// Close any existing stream first
	h.wv.closeAudioStream()

	// Create new stream
	ctx := h.wv.ctx
	if ctx == nil {
		ctx = context.Background()
	}

	stream, err := h.wv.audioOutputFactory.NewStream(ctx, format)
	if err != nil {
		// Log error but don't panic - audio is non-critical.
		// Do NOT set audioPlaying — the stream was never created.
		if h.wv.ctx != nil {
			logging.FromContext(h.wv.ctx).Warn().
				Err(err).
				Int("sample_rate", format.SampleRate).
				Int("channels", format.ChannelCount).
				Msg("cef: failed to create audio output stream")
		}
		return
	}

	// Set playing state only after stream creation succeeds
	h.wv.setAudioPlaying(true)

	if h.wv.ctx != nil {
		logging.FromContext(h.wv.ctx).Info().
			Int("sample_rate", format.SampleRate).
			Int("channels", format.ChannelCount).
			Int("frames_per_buffer", format.FramesPerBuffer).
			Msg("cef: audio output stream created successfully")
	}

	// Store the new stream
	h.wv.audioStreamMu.Lock()
	h.wv.activeAudioStream = stream
	h.wv.audioStreamMu.Unlock()
}

// buildAudioStreamFormat converts CEF AudioParameters to port.AudioStreamFormat.
// channels is the authoritative channel count from the callback; params.FramesPerBuffer
// supplies the buffer size.
func (h *handlerSet) buildAudioStreamFormat(params *purecef.AudioParameters, channels int32) port.AudioStreamFormat {
	return port.AudioStreamFormat{
		SampleRate:      int(params.SampleRate),
		ChannelCount:    int(channels),
		FramesPerBuffer: int(params.FramesPerBuffer),
	}
}

// OnAudioStreamPacket receives audio packets and forwards them to the output stream.
// The mutex is held across both the stream snapshot and Write to prevent
// closeAudioStream from closing the stream mid-write.
func (h *handlerSet) OnAudioStreamPacket(_ purecef.Browser, data [][]float32, frames int32, pts int64) {
	if len(data) == 0 || frames <= 0 {
		return
	}

	// Copy the data before acquiring the stream lock because CEF can reuse the
	// buffer as soon as this callback returns.
	copiedData := make([][]float32, len(data))
	for i, channel := range data {
		if len(channel) < int(frames) {
			continue
		}
		copiedData[i] = make([]float32, frames)
		copy(copiedData[i], channel[:frames])
	}

	// Hold the lock across the stream read and Write so closeAudioStream
	// cannot close the stream between snapshot and write.
	h.wv.audioStreamMu.Lock()
	stream := h.wv.activeAudioStream
	if stream == nil {
		h.wv.audioStreamMu.Unlock()
		return
	}

	pktNum := h.wv.audioPacketCount.Add(1)
	if pktNum == 1 && h.wv.ctx != nil {
		logging.FromContext(h.wv.ctx).Info().
			Int("channels", len(data)).
			Int32("frames", frames).
			Int64("pts", pts).
			Msg("cef: first audio packet received")
	}

	err := stream.Write(copiedData)
	h.wv.audioStreamMu.Unlock()

	if err != nil {
		if h.wv.ctx != nil {
			logging.FromContext(h.wv.ctx).Debug().
				Err(err).
				Uint64("packets_received", pktNum).
				Msg("cef: audio stream write failed")
		}
		return
	}
	h.wv.audioWriteCount.Add(1)
}

// OnAudioStreamStopped handles stream stop.
func (h *handlerSet) OnAudioStreamStopped(_ purecef.Browser) {
	if h.wv.ctx != nil {
		logging.FromContext(h.wv.ctx).Info().
			Uint64("packets_received", h.wv.audioPacketCount.Load()).
			Uint64("writes_succeeded", h.wv.audioWriteCount.Load()).
			Msg("cef: OnAudioStreamStopped")
	}
	h.wv.setAudioPlaying(false)
	h.wv.closeAudioStream()
}

// OnAudioStreamError handles stream errors.
func (h *handlerSet) OnAudioStreamError(_ purecef.Browser, message string) {
	if h.wv.ctx != nil {
		logging.FromContext(h.wv.ctx).Warn().
			Str("error", message).
			Uint64("packets_received", h.wv.audioPacketCount.Load()).
			Uint64("writes_succeeded", h.wv.audioWriteCount.Load()).
			Msg("cef: audio stream error")
	}
	h.wv.setAudioPlaying(false)
	h.wv.closeAudioStream()
}

// ===========================================================================
// FindHandler (1 method)
// ===========================================================================

// OnFindResult dispatches CEF find results to the WebView's FindController.
func (h *handlerSet) OnFindResult(
	_ purecef.Browser,
	identifier, count int32,
	_ *purecef.Rect,
	activematchordinal, finalupdate int32,
) {
	if h.wv.findCtrl != nil {
		h.wv.findCtrl.handleFindResult(identifier, count, activematchordinal, finalupdate)
	}
}

// cefCursorNames maps CEF cursor types to GDK/CSS cursor names.
var cefCursorNames = map[purecef.CursorType]string{
	purecef.CursorTypeCtPointer:                  "default",
	purecef.CursorTypeCtCross:                    "crosshair",
	purecef.CursorTypeCtHand:                     "pointer",
	purecef.CursorTypeCtIbeam:                    "text",
	purecef.CursorTypeCtWait:                     "wait",
	purecef.CursorTypeCtHelp:                     "help",
	purecef.CursorTypeCtEastresize:               "e-resize",
	purecef.CursorTypeCtNorthresize:              "n-resize",
	purecef.CursorTypeCtNortheastresize:          "ne-resize",
	purecef.CursorTypeCtNorthwestresize:          "nw-resize",
	purecef.CursorTypeCtSouthresize:              "s-resize",
	purecef.CursorTypeCtSoutheastresize:          "se-resize",
	purecef.CursorTypeCtSouthwestresize:          "sw-resize",
	purecef.CursorTypeCtWestresize:               "w-resize",
	purecef.CursorTypeCtNorthsouthresize:         "ns-resize",
	purecef.CursorTypeCtEastwestresize:           "ew-resize",
	purecef.CursorTypeCtNortheastsouthwestresize: "nesw-resize",
	purecef.CursorTypeCtNorthwestsoutheastresize: "nwse-resize",
	purecef.CursorTypeCtColumnresize:             "col-resize",
	purecef.CursorTypeCtRowresize:                "row-resize",
	purecef.CursorTypeCtMove:                     "move",
	purecef.CursorTypeCtProgress:                 "progress",
	purecef.CursorTypeCtNodrop:                   "no-drop",
	purecef.CursorTypeCtNotallowed:               "not-allowed",
	purecef.CursorTypeCtGrab:                     "grab",
	purecef.CursorTypeCtGrabbing:                 "grabbing",
	purecef.CursorTypeCtZoomin:                   "zoom-in",
	purecef.CursorTypeCtZoomout:                  "zoom-out",
}

// cefCursorToGDKName maps a CEF cursor type to a GDK/CSS cursor name.
func cefCursorToGDKName(ct purecef.CursorType) string {
	if name, ok := cefCursorNames[ct]; ok {
		return name
	}
	return "default"
}
