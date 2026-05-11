package cef

import (
	"context"
	"strings"

	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/puregotk/v4/gtk"
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
	reason := wv.takeViewportSyncReason()
	if wv.destroyed.Load() || wv.viewBridge == nil {
		return
	}

	if wv.tryStartPendingBrowserCreateOnGTKThread(ctx, reason) {
		return
	}

	wv.mu.RLock()
	host := wv.host
	wv.mu.RUnlock()
	if host == nil {
		logging.FromContext(ctx).Debug().
			Uint64("webview_id", uint64(wv.id)).
			Str("reason", reason).
			Msg("cef: viewport sync skipped; browser host not ready")
		return
	}

	widget := wv.viewBridge.Widget()
	visible := widget != nil && widget.IsVisible()
	notifyBrowserViewportSync(host, visible)
	logging.FromContext(ctx).Debug().
		Uint64("webview_id", uint64(wv.id)).
		Bool("visible", visible).
		Str("reason", reason).
		Msg("cef: viewport sync requested")
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

	widget.ConnectMap(&wv.viewportMapFunc)
	widget.ConnectShow(&wv.viewportShowFunc)
	widget.ConnectRealize(&wv.viewportRealizeFunc)
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
