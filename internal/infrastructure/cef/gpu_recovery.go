package cef

import (
	"time"

	purecef "github.com/bnema/purego-cef/cef"
	"github.com/rs/zerolog"

	"github.com/bnema/dumber/internal/logging"
)

// GPU process crash recovery.
//
// When Chromium's GPU process dies and is relaunched, existing OSR browsers
// stop receiving OnAcceleratedPaint even though the CEF UI/IO threads and GTK
// stay alive. Recovery is two-staged per live view:
//
//  1. a render pulse (the same one used by the stall watchdog) on the CEF UI
//     thread, which is cheap and often enough to re-establish the compositor
//     output;
//  2. if no accelerated paint arrived within gpuRecoveryVerifyDelay, a page
//     reload, which recreates the renderer's compositor frame sink.
//
// Unlike the stall watchdog, this path is always on and not env-gated.
//
// Concurrency model:
//   - Every recovery attempt takes a per-view generation (gpuRecoveryGen). The
//     pulse task and verify callback of an older attempt bail once a newer one
//     started (a new crash, or a deferred view being shown), so a stale verify
//     never reloads early and attempts never stack up.
//   - Visibility (effectiveVisible), the deferral flag (gpuRecoveryPending) and
//     the generation bump are all read/written under wv.mu. "Is the view hidden?
//     then mark it pending" and "view became visible: consume pending" are
//     therefore atomic with respect to each other: a deferral cannot be lost to
//     a concurrent show.
//   - The view bridge is cleared under wv.mu by teardown; recovery only reads
//     it through renderPulseTarget/liveBridge snapshots taken under wv.mu.
//   - The pulse runs on the CEF UI thread, the same thread that applies
//     visibility changes, so the hidden check in the pulse task is also
//     exact with respect to its WasHidden(0).

const (
	// gpuRecoveryDelay lets the relaunched GPU process initialize (GL/Vulkan
	// context, GPU channel hand-off to the browser and renderers) before the
	// first pulse. OnBeforeChildProcessLaunch fires when the process is spawned,
	// not when it is ready; initialization takes a few hundred ms, so 1s leaves
	// margin without a visibly long freeze. It also acts as the debounce window:
	// every relaunch notification arriving while a pass is pending is absorbed.
	gpuRecoveryDelay = 1 * time.Second
	// gpuRecoveryVerifyDelay is how long a pulse gets to yield a new paint.
	gpuRecoveryVerifyDelay = renderRecoveryVerifyDelay
)

// gpuRecoveryPaintCount reads the accelerated paint counter of a bridge. It is
// a variable so tests can simulate paints without a live GTK/GL bridge.
var gpuRecoveryPaintCount = func(bridge *Cef2gtkAdapter) int {
	return bridge.Diagnostics().AcceleratedPaints
}

// noteGPUProcessLaunch is called for every GPU process launch with the
// running launch count. The first launch is normal startup; later ones mean a
// crash and relaunch. It only schedules work, so it is safe on the CEF
// browser-process launcher thread.
func (e *Engine) noteGPUProcessLaunch(launchCount uint64) {
	if e == nil || launchCount <= 1 {
		return
	}
	if !e.gpuRecoveryScheduled.CompareAndSwap(false, true) {
		return
	}
	logging.FromContext(e.currentContext()).Warn().
		Uint64("gpu_launches", launchCount).
		Dur("delay", gpuRecoveryDelay).
		Msg("cef: gpu process relaunched, scheduling render recovery")
	cefScheduleAfter(gpuRecoveryDelay, e.runGPURecoveryPass)
}

// runGPURecoveryPass starts recovery for every live view.
func (e *Engine) runGPURecoveryPass() {
	e.gpuRecoveryScheduled.Store(false)
	e.activeWebViews.Range(func(_, value any) bool {
		if wv, ok := value.(*WebView); ok {
			wv.startGPURecovery()
		}
		return true
	})
}

// startGPURecovery begins a new recovery attempt for the view (superseding any
// in-flight one): stage 1 posts the pulse, then stage 2 is scheduled.
//
// Hidden views legitimately produce no paints, so they can be neither pulsed
// (the pulse would leave CEF believing the view is visible) nor judged. They
// are marked pending instead and recovered when they become visible again.
func (wv *WebView) startGPURecovery() {
	target, ok := wv.renderPulseTarget()
	if !ok || !target.hasBrowser {
		return
	}
	gen, deferred := wv.claimGPURecovery()
	log := logging.FromContext(wv.ctx)
	if deferred {
		log.Warn().
			Uint64("webview_id", uint64(wv.id)).
			Int32("browser_id", target.browserID).
			Str("action", "defer").
			Msg("cef: gpu recovery deferred until view is visible")
		return
	}
	browserID := target.browserID
	const msg = "cef: gpu recovery pulse"
	wv.postRenderPulseTask(msg, func(host purecef.BrowserHost, bridge *Cef2gtkAdapter) {
		if wv.gpuRecoveryGen.Load() != gen {
			return // superseded by a newer attempt
		}
		// Runs on the CEF UI thread, where visibility changes are applied, so
		// the view cannot be shown or hidden between this check and the pulse.
		if wv.deferGPURecoveryIfHidden() {
			logging.FromContext(wv.ctx).Warn().
				Uint64("webview_id", uint64(wv.id)).
				Int32("browser_id", browserID).
				Str("action", "defer").
				Msg(msg + " skipped: view hidden, deferred until visible")
			return
		}
		// The baseline is taken here, right before the pulse, so paints that
		// landed while the task was queued do not count as pulse recovery.
		before := gpuRecoveryPaintCount(bridge)
		pulseRender(host)
		logging.FromContext(wv.ctx).Warn().
			Uint64("webview_id", uint64(wv.id)).
			Int32("browser_id", browserID).
			Int("accelerated_paints_before", before).
			Msg(msg + " executed")
		// The verify window starts after the pulse ran, and only for a pulse
		// that actually ran (not for a skipped or stale one).
		cefScheduleAfter(gpuRecoveryVerifyDelay, func() { wv.verifyGPURecovery(gen, browserID, before) })
	})
}

// verifyGPURecovery is stage 2: reload the view if the pulse produced no paint.
func (wv *WebView) verifyGPURecovery(gen uint64, browserID int32, before int) {
	if wv.gpuRecoveryGen.Load() != gen {
		return // superseded by a newer attempt
	}
	bridge := wv.liveBridge()
	if bridge == nil || wv.ctx == nil {
		return
	}
	after := gpuRecoveryPaintCount(bridge)
	event := func() *zerolog.Event {
		return logging.FromContext(wv.ctx).Warn().
			Uint64("webview_id", uint64(wv.id)).
			Int32("browser_id", browserID).
			Int("accelerated_paints_before", before).
			Int("accelerated_paints_after", after)
	}
	if after > before {
		event().Str("action", "none").Msg("cef: gpu recovery pulse recovered view")
		return
	}
	// Reload waits on a synchronous GTK dispatch, during which the view can be
	// hidden, destroyed or superseded. The guard revalidates all of that on the
	// GTK thread right before navigating; no lock is held across the dispatch.
	reloaded := false
	deferred := false
	err := wv.reloadGuarded(func() bool {
		if wv.destroyed.Load() || wv.gpuRecoveryGen.Load() != gen {
			return false
		}
		if wv.deferGPURecoveryIfHidden() {
			deferred = true
			return false
		}
		reloaded = true
		return true
	})
	switch {
	case err != nil:
		logging.FromContext(wv.ctx).Warn().
			Err(err).
			Uint64("webview_id", uint64(wv.id)).
			Int32("browser_id", browserID).
			Msg("cef: gpu recovery reload failed")
	case reloaded:
		event().Str("action", "reload").Msg("cef: gpu recovery reload")
	case deferred:
		event().Str("action", "defer").Msg("cef: gpu recovery deferred until view is visible")
	}
}

// claimGPURecovery starts a new attempt generation. If the view is hidden it
// marks recovery pending instead; otherwise it clears any stale pending flag.
// Everything happens under wv.mu so it is atomic with visibility changes.
func (wv *WebView) claimGPURecovery() (gen uint64, deferred bool) {
	wv.mu.Lock()
	defer wv.mu.Unlock()
	gen = wv.gpuRecoveryGen.Add(1)
	if wv.hiddenLocked() {
		wv.gpuRecoveryPending = true
		return gen, true
	}
	wv.gpuRecoveryPending = false
	return gen, false
}

// deferGPURecoveryIfHidden atomically checks visibility and, when hidden,
// marks recovery pending so the next show resumes it.
func (wv *WebView) deferGPURecoveryIfHidden() bool {
	wv.mu.Lock()
	defer wv.mu.Unlock()
	if !wv.hiddenLocked() {
		return false
	}
	wv.gpuRecoveryPending = true
	return true
}

// effectivelyHidden reports whether CEF was last told the view is hidden. A
// view whose visibility was never reported is treated as visible.
func (wv *WebView) effectivelyHidden() bool {
	wv.mu.RLock()
	defer wv.mu.RUnlock()
	return wv.hiddenLocked()
}

// hiddenLocked is effectivelyHidden for callers already holding wv.mu.
func (wv *WebView) hiddenLocked() bool {
	return wv.effectiveVisibilityKnown && !wv.effectiveVisible
}
