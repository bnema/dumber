package cef

import (
	"context"
	"strings"

	purecef "github.com/bnema/purego-cef/cef"
	"github.com/bnema/puregotk/v4/gobject"
	"github.com/bnema/puregotk/v4/gtk"

	"github.com/bnema/dumber/internal/logging"
)

type viewportSyncBrowserHost interface {
	resizeNotifiableBrowserHost
	NotifyScreenInfoChanged()
	WasHidden(int32)
}

// SyncViewport requests an explicit viewport resync for CEF OSR browsers.
//
// This is a hardening path for UI lifecycle transitions that may not emit a
// fresh size change (reparenting, stacked visibility toggles, floating pane
// updates, popup show, sibling promotion after close, etc.). Calls are
// coalesced and executed on the GTK thread before notifying the CEF host.
func (wv *WebView) SyncViewport(ctx context.Context, reason string) {
	if wv == nil || wv.destroyed.Load() {
		return
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "manual"
	}

	wv.viewportSyncMu.Lock()
	wv.viewportSyncReason = reason
	if wv.viewportSyncPending {
		wv.viewportSyncMu.Unlock()
		return
	}
	wv.viewportSyncPending = true
	wv.viewportSyncMu.Unlock()

	wv.runOnGTK(func() {
		wv.syncViewportOnGTK(wv.viewportSyncContext(ctx))
	})
}

func (wv *WebView) viewportSyncContext(ctx context.Context) context.Context {
	if ctx != nil {
		return ctx
	}
	if wv != nil && wv.ctx != nil {
		return wv.ctx
	}
	return context.Background()
}

func (wv *WebView) takeViewportSyncReason() string {
	wv.viewportSyncMu.Lock()
	defer wv.viewportSyncMu.Unlock()
	reason := wv.viewportSyncReason
	wv.viewportSyncReason = ""
	wv.viewportSyncPending = false
	return reason
}

func (wv *WebView) syncViewportOnGTK(ctx context.Context) {
	if wv == nil {
		return
	}
	wv.syncViewportNowOnGTK(ctx, wv.takeViewportSyncReason())
}

func (wv *WebView) syncViewportNowOnGTK(ctx context.Context, reason string) bool {
	if wv == nil {
		return false
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "manual"
	}
	if wv.destroyed.Load() || wv.viewBridge == nil {
		return false
	}

	if wv.tryStartPendingBrowserCreateOnGTKThread(ctx, reason) {
		return false
	}

	wv.mu.RLock()
	host := wv.host
	wv.mu.RUnlock()
	if host == nil {
		logging.FromContext(ctx).Debug().
			Uint64("webview_id", uint64(wv.id)).
			Str("reason", reason).
			Msg("cef: viewport sync skipped; browser host not ready")
		return false
	}

	widget := wv.viewBridge.Widget()
	visible := widget != nil && widget.IsVisible()
	notifyBrowserViewportSync(host, visible)
	logging.FromContext(ctx).Debug().
		Uint64("webview_id", uint64(wv.id)).
		Bool("visible", visible).
		Str("reason", reason).
		Msg("cef: viewport sync requested")
	return true
}

func (wv *WebView) syncResizeViewportOnGTK(ctx context.Context, reason string) bool {
	if !wv.syncViewportNowOnGTK(ctx, reason) {
		return false
	}
	wv.scheduleResizeRepaintPulse(ctx, reason)
	return true
}

func (wv *WebView) scheduleResizeRepaintPulse(ctx context.Context, reason string) {
	if wv == nil || wv.destroyed.Load() {
		return
	}
	seq := wv.viewportResizePulseSeq.Add(1)
	for _, delayMs := range [...]int64{16, 48} {
		task := cefNewTask(cefTaskFunc(func() {
			if wv == nil || wv.destroyed.Load() || wv.viewportResizePulseSeq.Load() != seq {
				return
			}
			wv.mu.RLock()
			host := wv.host
			wv.mu.RUnlock()
			if host == nil {
				return
			}
			host.Invalidate(purecef.PaintElementTypePetView)
			logging.FromContext(ctx).Debug().
				Uint64("webview_id", uint64(wv.id)).
				Int64("delay_ms", delayMs).
				Str("reason", reason).
				Msg("cef: delayed resize repaint pulse")
		}))
		if task == nil {
			continue
		}
		cefPostDelayedTask(purecef.ThreadIDTidUi, task, delayMs)
	}
}

func (wv *WebView) tryStartPendingBrowserCreateOnGTKThread(ctx context.Context, reason string) bool {
	if wv == nil || wv.destroyed.Load() || wv.factory == nil || wv.viewBridge == nil {
		return false
	}

	wv.mu.RLock()
	hasHost := wv.host != nil
	pending := wv.pendingCreate != nil
	wv.mu.RUnlock()
	if hasHost || !pending {
		return false
	}

	widget := wv.viewBridge.Widget()
	if widget == nil {
		return false
	}
	width := int32(widget.GetAllocatedWidth())
	height := int32(widget.GetAllocatedHeight())
	if width <= 0 {
		width = int32(widget.GetWidth())
	}
	if height <= 0 {
		height = int32(widget.GetHeight())
	}
	if width <= 0 || height <= 0 {
		logging.FromContext(ctx).Debug().
			Uint64("webview_id", uint64(wv.id)).
			Int32("width", width).
			Int32("height", height).
			Str("reason", reason).
			Msg("cef: viewport sync skipped pending browser creation; widget has no allocation yet")
		return false
	}
	if err := wv.viewBridge.PrepareOnGTKThread(); err != nil {
		logging.FromContext(ctx).Debug().
			Err(err).
			Uint64("webview_id", uint64(wv.id)).
			Int32("width", width).
			Int32("height", height).
			Str("reason", reason).
			Msg("cef: viewport sync could not prepare bridge for pending browser creation")
		return false
	}

	wv.factory.postPendingBrowserCreate(ctx, wv, width, height)
	logging.FromContext(ctx).Debug().
		Uint64("webview_id", uint64(wv.id)).
		Int32("width", width).
		Int32("height", height).
		Str("reason", reason).
		Msg("cef: viewport sync nudged pending browser creation")
	return true
}

func (wv *WebView) installViewportSyncHooks() {
	if wv == nil || wv.viewBridge == nil {
		return
	}
	wv.runOnGTKSync(func() {
		wv.installViewportSyncHooksOnGTKThread()
	})
}

func (wv *WebView) installViewportSyncHooksOnGTKThread() {
	if wv == nil || wv.viewBridge == nil || wv.viewportMapFunc != nil {
		return
	}
	widget := wv.viewBridge.Widget()
	if widget == nil {
		return
	}

	wv.viewportMapFunc = func(gtk.Widget) {
		wv.SyncViewport(wv.ctx, "gtk-map")
	}
	wv.viewportShowFunc = func(gtk.Widget) {
		wv.SyncViewport(wv.ctx, "gtk-show")
	}
	wv.viewportRealizeFunc = func(gtk.Widget) {
		wv.SyncViewport(wv.ctx, "gtk-realize")
	}

	wv.viewportMapSignalID = widget.ConnectMap(&wv.viewportMapFunc)
	wv.viewportShowSignalID = widget.ConnectShow(&wv.viewportShowFunc)
	wv.viewportRealizeSignalID = widget.ConnectRealize(&wv.viewportRealizeFunc)
}

func (wv *WebView) disconnectViewportSyncHooksOnGTKThread() {
	if wv == nil || wv.viewBridge == nil {
		return
	}
	widget := wv.viewBridge.Widget()
	if widget != nil {
		widgetPtr := widget.GoPointer()
		if widgetPtr != 0 {
			obj := gobject.ObjectNewFromInternalPtr(widgetPtr)
			if wv.viewportMapSignalID != 0 {
				gobject.SignalHandlerDisconnect(obj, wv.viewportMapSignalID)
				wv.viewportMapSignalID = 0
			}
			if wv.viewportShowSignalID != 0 {
				gobject.SignalHandlerDisconnect(obj, wv.viewportShowSignalID)
				wv.viewportShowSignalID = 0
			}
			if wv.viewportRealizeSignalID != 0 {
				gobject.SignalHandlerDisconnect(obj, wv.viewportRealizeSignalID)
				wv.viewportRealizeSignalID = 0
			}
		}
	}
	wv.viewportMapFunc = nil
	wv.viewportShowFunc = nil
	wv.viewportRealizeFunc = nil
}

func notifyBrowserViewportSync(host viewportSyncBrowserHost, visible bool) {
	if host == nil {
		return
	}
	if visible {
		host.WasHidden(0)
	}
	host.NotifyScreenInfoChanged()
	notifyBrowserResize(host)
}
