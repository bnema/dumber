package cef

import (
	"context"
	"strings"
	"time"

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

type pendingBrowserCreateObservedSizeBridge interface {
	Size() (int32, int32)
	RefreshObservedSizeOnGTKThread() (int32, int32)
}

const (
	// pendingBrowserCreateObservedSizeRetryDelay spaces retries while waiting
	// for the GTK bridge to observe the widget's allocated size.
	pendingBrowserCreateObservedSizeRetryDelay = 16 * time.Millisecond
	// pendingBrowserCreateObservedSizeMaxRetries caps observed-size retries
	// before browser creation proceeds despite a remaining size mismatch.
	pendingBrowserCreateObservedSizeMaxRetries = 8
)

type pendingBrowserCreateObservedSizeRetryAction uint8

const (
	pendingBrowserCreateObservedSizeRetryUnavailable pendingBrowserCreateObservedSizeRetryAction = iota
	pendingBrowserCreateObservedSizeRetryAlreadyScheduled
	pendingBrowserCreateObservedSizeRetryScheduled
	pendingBrowserCreateObservedSizeRetryProceedWithoutDelay
)

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
	wv.notifyViewportSyncOnCEFUIThread(host, visible)
	wv.syncZoomForBackingScaleOnCEFUIThread(host, reason)
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

	observedWidth, observedHeight, observedReady := pendingBrowserCreateObservedSize(wv.viewBridge, width, height)
	if !observedReady {
		widget.QueueAllocate()
		widget.QueueResize()
		attempt, action := wv.preparePendingBrowserCreateObservedSizeRetry(ctx, reason)
		switch action {
		case pendingBrowserCreateObservedSizeRetryUnavailable:
			return false
		case pendingBrowserCreateObservedSizeRetryAlreadyScheduled:
			logging.FromContext(ctx).Debug().
				Uint64("webview_id", uint64(wv.id)).
				Int32("observed_width", observedWidth).
				Int32("observed_height", observedHeight).
				Int32("width", width).
				Int32("height", height).
				Int("attempt", attempt).
				Str("reason", reason).
				Msg("cef: pending browser creation retry already scheduled while awaiting bridge size")
			return false
		case pendingBrowserCreateObservedSizeRetryScheduled:
			logging.FromContext(ctx).Debug().
				Uint64("webview_id", uint64(wv.id)).
				Int32("observed_width", observedWidth).
				Int32("observed_height", observedHeight).
				Int32("width", width).
				Int32("height", height).
				Int("attempt", attempt).
				Int("max_attempts", pendingBrowserCreateObservedSizeMaxRetries).
				Str("reason", reason).
				Msg("cef: delaying pending browser creation until bridge observes widget size")
			return false
		case pendingBrowserCreateObservedSizeRetryProceedWithoutDelay:
			// Creating the browser here is intentional: after bounded retries we
			// fall back to the current GTK allocation rather than stalling popup
			// creation.
			logging.FromContext(ctx).Debug().
				Uint64("webview_id", uint64(wv.id)).
				Int32("observed_width", observedWidth).
				Int32("observed_height", observedHeight).
				Int32("width", width).
				Int32("height", height).
				Int("attempt", attempt).
				Str("reason", reason).
				Msg("cef: bridge size observation did not settle before browser creation; continuing")
		}
	}

	if wv.shouldDeferPendingBrowserCreateFromViewportSync() {
		logging.FromContext(ctx).Debug().
			Uint64("webview_id", uint64(wv.id)).
			Int32("width", width).
			Int32("height", height).
			Str("reason", reason).
			Msg("cef: viewport sync deferred pending browser creation while awaiting native popup attach")
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

	wv.markInitialBrowserCreateResizeHandled()
	wv.factory.postPendingBrowserCreate(ctx, wv, width, height)
	logging.FromContext(ctx).Debug().
		Uint64("webview_id", uint64(wv.id)).
		Int32("width", width).
		Int32("height", height).
		Str("reason", reason).
		Msg("cef: viewport sync nudged pending browser creation")
	return true
}

func (wv *WebView) shouldDeferPendingBrowserCreateFromViewportSync() bool {
	return wv != nil && wv.awaitsNativePopupAttachment()
}

func pendingBrowserCreateObservedSize(
	bridge pendingBrowserCreateObservedSizeBridge,
	allocatedWidth, allocatedHeight int32,
) (observedWidth, observedHeight int32, ready bool) {
	if bridge == nil {
		return 0, 0, false
	}
	observedWidth, observedHeight = bridge.Size()
	if observedViewportSizeReady(observedWidth, observedHeight, allocatedWidth, allocatedHeight) {
		return observedWidth, observedHeight, true
	}
	observedWidth, observedHeight = bridge.RefreshObservedSizeOnGTKThread()
	return observedWidth, observedHeight, observedViewportSizeReady(observedWidth, observedHeight, allocatedWidth, allocatedHeight)
}

// observedViewportSizeReady reports whether observedWidth/observedHeight match
// allocatedWidth/allocatedHeight within a ±1px tolerance.
//
// Fallback observed sizes (<=1) and non-positive allocated dimensions are
// treated as not ready.
func observedViewportSizeReady(observedWidth, observedHeight, allocatedWidth, allocatedHeight int32) bool {
	if allocatedWidth <= 0 || allocatedHeight <= 0 {
		return false
	}
	if observedWidth <= 1 || observedHeight <= 1 {
		return false
	}
	return abs32(observedWidth-allocatedWidth) <= 1 && abs32(observedHeight-allocatedHeight) <= 1
}

// abs32 returns the absolute value of v.
func abs32(v int32) int32 {
	if v < 0 {
		return -v
	}
	return v
}

// preparePendingBrowserCreateObservedSizeRetry updates the observed-size retry
// state machine for a pending browser create.
//
// It returns the current attempt count and the resulting action. Scheduling is
// coalesced so only one delayed retry is pending at a time, while the max-retry
// path proceeds without leaving the scheduled flag set.
func (wv *WebView) preparePendingBrowserCreateObservedSizeRetry(
	ctx context.Context,
	reason string,
) (attempt int, action pendingBrowserCreateObservedSizeRetryAction) {
	if wv == nil {
		return 0, pendingBrowserCreateObservedSizeRetryUnavailable
	}

	shouldSchedule := false
	wv.mu.Lock()
	if wv.pendingCreate == nil {
		wv.mu.Unlock()
		return 0, pendingBrowserCreateObservedSizeRetryUnavailable
	}
	if wv.pendingCreate.observedSizeRetryScheduled {
		attempt = wv.pendingCreate.observedSizeRetries
		wv.mu.Unlock()
		return attempt, pendingBrowserCreateObservedSizeRetryAlreadyScheduled
	}
	wv.pendingCreate.observedSizeRetries++
	attempt = wv.pendingCreate.observedSizeRetries
	if attempt <= pendingBrowserCreateObservedSizeMaxRetries {
		wv.pendingCreate.observedSizeRetryScheduled = true
		shouldSchedule = true
	}
	wv.mu.Unlock()

	if !shouldSchedule {
		return attempt, pendingBrowserCreateObservedSizeRetryProceedWithoutDelay
	}
	wv.schedulePendingBrowserCreateObservedSizeRetry(ctx, reason)
	return attempt, pendingBrowserCreateObservedSizeRetryScheduled
}

// clearPendingBrowserCreateObservedSizeRetry clears the scheduled-retry flag
// so a future observed-size reservation can schedule another attempt.
func (wv *WebView) clearPendingBrowserCreateObservedSizeRetry() {
	if wv == nil {
		return
	}
	wv.mu.Lock()
	defer wv.mu.Unlock()
	if wv.pendingCreate != nil {
		wv.pendingCreate.observedSizeRetryScheduled = false
	}
}

// schedulePendingBrowserCreateObservedSizeRetry schedules a delayed viewport
// sync retry using ctx after pendingBrowserCreateObservedSizeRetryDelay.
//
// It clears the scheduled flag before re-invoking syncViewportNowOnGTK with a
// retry-specific suffix appended to reason.
func (wv *WebView) schedulePendingBrowserCreateObservedSizeRetry(ctx context.Context, reason string) {
	if wv == nil || wv.destroyed.Load() {
		return
	}
	cefScheduleAfter(pendingBrowserCreateObservedSizeRetryDelay, func() {
		if wv == nil || wv.destroyed.Load() {
			return
		}
		wv.clearPendingBrowserCreateObservedSizeRetry()
		wv.runOnGTK(func() {
			wv.syncViewportNowOnGTK(ctx, reason+"-await-observed-size")
		})
	})
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

func (wv *WebView) notifyViewportSyncOnCEFUIThread(host viewportSyncBrowserHost, visible bool) {
	if host == nil {
		return
	}
	task := cefNewTask(cefTaskFunc(func() {
		if wv == nil || wv.destroyed.Load() {
			return
		}
		wv.mu.RLock()
		currentHost := wv.host
		wv.mu.RUnlock()
		if currentHost != host {
			return
		}
		notifyBrowserViewportSync(host, visible)
	}))
	if task == nil {
		return
	}
	cefPostDelayedTask(purecef.ThreadIDTidUi, task, 0)
}

func (wv *WebView) syncZoomForBackingScaleOnCEFUIThread(host purecef.BrowserHost, reason string) {
	if host == nil || wv == nil {
		return
	}
	if !wv.shouldReapplyZoomForBackingScale(wv.osrBackingScaleFactor()) {
		return
	}
	task := cefNewTask(cefTaskFunc(func() {
		if wv == nil || wv.destroyed.Load() {
			return
		}
		wv.mu.RLock()
		currentHost := wv.host
		wv.mu.RUnlock()
		if currentHost != host {
			return
		}
		backingScale := wv.osrBackingScaleFactor()
		if !wv.shouldReapplyZoomForBackingScale(backingScale) {
			return
		}
		wv.reapplyCurrentZoomForBackingScale(reason)
	}))
	if task == nil {
		return
	}
	cefPostDelayedTask(purecef.ThreadIDTidUi, task, 0)
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
