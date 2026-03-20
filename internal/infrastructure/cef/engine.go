package cef

import (
	"context"
	"sync"

	purecef "github.com/bnema/purego-cef/cef"
	"github.com/jwijenbergh/puregotk/v4/glib"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/logging"
)

// Compile-time interface check.
var _ port.Engine = (*Engine)(nil)

// cefMaxTimerDelayMS is the maximum delay between pump ticks.
// Set to 33ms (30fps) as a balance between responsiveness and CPU cost.
// With --disable-gpu-compositing, Chromium rasterizes in software so
// every pump tick has significant CPU cost.
const cefMaxTimerDelayMS int64 = 33

// Engine implements port.Engine for the CEF browser backend.
// It manages the CEF lifecycle and provides access to all engine subsystems.
type Engine struct {
	ctx     context.Context
	gl      *glLoader
	factory *WebViewFactory
	pool    *WebViewPool

	// Adaptive message pump state. OnScheduleMessagePumpWork schedules
	// one-shot glib sources; pumpSourceID tracks the pending source so it
	// can be cancelled when CEF requests a different delay.
	pumpMu         sync.Mutex
	pumpCB         glib.SourceFunc // kept alive for GC
	pumpSourceID   uint
	pumpReady      bool // set true once GTK main loop is running
	pumpClosing    bool
	pumpHasPending bool  // an OnScheduleMessagePumpWork arrived before pumpReady
	pumpPendingMs  int64 // delay from the last pre-ready request

	// Reentrancy guard: DoMessageLoopWork can trigger callbacks that
	// re-enter the pump. The reference implementation detects this and
	// schedules an immediate follow-up instead of recursing.
	pumpActive  bool
	pumpReentry bool
}

// Factory returns the WebViewFactory for creating new WebView instances.
func (e *Engine) Factory() port.WebViewFactory {
	return &webViewFactoryAdapter{factory: e.factory}
}

// Pool returns the WebViewPool for acquiring pre-warmed WebView instances.
func (e *Engine) Pool() port.WebViewPool {
	return &webViewPoolAdapter{pool: e.pool, ctx: e.ctx}
}

// ContentInjector returns a no-op injector (Phase 1 stub).
func (e *Engine) ContentInjector() port.ContentInjector {
	return &noopContentInjector{}
}

// SettingsApplier returns a no-op applier (Phase 1 stub).
func (e *Engine) SettingsApplier() port.SettingsApplier {
	return &noopSettingsApplier{}
}

// FilterApplier returns nil (content filtering not yet supported for CEF).
func (e *Engine) FilterApplier() port.FilterApplier {
	return nil
}

// FaviconDatabase returns a no-op database (Phase 1 stub).
func (e *Engine) FaviconDatabase() port.FaviconDatabase {
	return &noopFaviconDatabase{}
}

// InternalSchemePath returns the URI scheme used for internal app resources.
func (e *Engine) InternalSchemePath() string {
	return "cef://dumber/"
}

// Close releases all resources held by the engine.
func (e *Engine) Close() error {
	e.stopMessagePump()
	e.pool.Close()
	purecef.Shutdown()
	return nil
}

// RegisterHandlers registers all WebUI message bridge handlers (Phase 1 stub).
func (e *Engine) RegisterHandlers(_ context.Context, _ port.HandlerDependencies) error {
	return nil
}

// RegisterAccentHandlers registers accent/dead-key input handlers (Phase 1 stub).
func (e *Engine) RegisterAccentHandlers(_ context.Context, _ port.AccentKeyHandler) error {
	return nil
}

// ConfigureDownloads sets up download handling (Phase 1 stub).
func (e *Engine) ConfigureDownloads(_ context.Context, _ string, _ port.DownloadEventHandler, _ port.DownloadPreparer) error {
	return nil
}

// OnToolkitReady is called after the UI toolkit has initialized.
// Marks the pump as ready so that OnScheduleMessagePumpWork callbacks
// (which may have arrived during cef_initialize) start scheduling work.
func (e *Engine) OnToolkitReady(_ context.Context) error {
	log := logging.FromContext(e.ctx)
	log.Debug().Msg("cef: OnToolkitReady called, pump is now ready")

	e.pumpMu.Lock()
	e.pumpReady = true
	hasPending := e.pumpHasPending
	pendingMs := e.pumpPendingMs
	e.pumpHasPending = false
	e.pumpMu.Unlock()

	// Replay any buffered OnScheduleMessagePumpWork that arrived before GTK
	// was ready, or kick an immediate pump if none was buffered.
	if hasPending {
		e.scheduleMessagePumpWork(pendingMs)
	} else {
		e.scheduleMessagePumpWork(0)
	}
	return nil
}

// UpdateAppearance updates default background color for new WebViews (Phase 1 stub).
func (e *Engine) UpdateAppearance(_ context.Context, _, _, _, _ float64) error {
	return nil
}

// UpdateSettings applies runtime config changes to engine internals (Phase 1 stub).
func (e *Engine) UpdateSettings(_ context.Context, _ port.EngineSettingsUpdate) error {
	return nil
}

// SetHandlerContext sets the base context for message handler dispatch.
func (e *Engine) SetHandlerContext(ctx context.Context) {
	e.ctx = ctx
}

// ---------------------------------------------------------------------------
// Adaptive CEF message pump (demand-driven via OnScheduleMessagePumpWork)
// ---------------------------------------------------------------------------

// scheduleMessagePumpWork is called by the BrowserProcessHandler when CEF
// has work to process, and by performMessageLoopWork to re-schedule after
// each pump tick. It replaces any pending one-shot timer with a new one
// matching the requested delay.
func (e *Engine) scheduleMessagePumpWork(delayMs int64) {
	e.pumpMu.Lock()
	defer e.pumpMu.Unlock()

	if e.pumpClosing {
		return
	}

	// Buffer requests that arrive before the GTK main loop is running.
	if !e.pumpReady {
		e.pumpHasPending = true
		e.pumpPendingMs = delayMs
		return
	}

	// Cancel any pending scheduled work.
	if e.pumpSourceID != 0 {
		glib.SourceRemove(e.pumpSourceID)
		e.pumpSourceID = 0
	}

	// Cap the delay at the maximum tick interval.
	if delayMs > cefMaxTimerDelayMS {
		delayMs = cefMaxTimerDelayMS
	}

	// The callback is one-shot (returns false).
	e.pumpCB = glib.SourceFunc(func(_ uintptr) bool {
		e.pumpMu.Lock()
		e.pumpSourceID = 0
		e.pumpMu.Unlock()

		e.performMessageLoopWork()
		return false
	})

	if delayMs <= 0 {
		e.pumpSourceID = glib.IdleAdd(&e.pumpCB, 0)
	} else {
		e.pumpSourceID = glib.TimeoutAdd(uint(delayMs), &e.pumpCB, 0)
	}
}

func (e *Engine) stopMessagePump() {
	e.pumpMu.Lock()
	defer e.pumpMu.Unlock()

	e.pumpClosing = true
	if e.pumpSourceID != 0 {
		glib.SourceRemove(e.pumpSourceID)
		e.pumpSourceID = 0
	}
}

// performMessageLoopWork calls DoMessageLoopWork with reentrancy protection
// and re-schedules follow-up work. Matches the pattern from CEF's
// main_message_loop_external_pump.cc: the host must always re-schedule
// after each DoMessageLoopWork call because CEF does not call
// OnScheduleMessagePumpWork after returning from DoMessageLoopWork.
func (e *Engine) performMessageLoopWork() {
	if e.pumpActive {
		e.pumpReentry = true
		return
	}

	e.pumpActive = true
	e.pumpReentry = false
	purecef.DoMessageLoopWork()
	e.pumpActive = false

	if e.pumpReentry {
		// CEF called OnScheduleMessagePumpWork re-entrantly during
		// DoMessageLoopWork — schedule immediate follow-up.
		e.scheduleMessagePumpWork(0)
	} else {
		// No re-entrant request — schedule at the max timer delay
		// to keep the pump alive for pending work.
		e.scheduleMessagePumpWork(cefMaxTimerDelayMS)
	}
}
