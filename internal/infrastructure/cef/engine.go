package cef

import (
	"context"
	"sync/atomic"
	"time"

	purecef "github.com/bnema/purego-cef/cef"
	"github.com/jwijenbergh/puregotk/v4/glib"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/logging"
)

// Compile-time interface check.
var _ port.Engine = (*Engine)(nil)

// pumpTickMS is the interval of the persistent glib timer that checks
// for pending CEF work. This is a polling interval, NOT the frame rate —
// when there's no pending work, the tick is a single atomic load (near-zero
// CPU). When work is pending, it calls DoMessageLoopWork immediately.
const pumpTickMS uint = 4

// Engine implements port.Engine for the CEF browser backend.
// It manages the CEF lifecycle and provides access to all engine subsystems.
type Engine struct {
	ctx     context.Context
	gl      *glLoader
	factory *WebViewFactory
	pool    *WebViewPool

	// pumpRequested is set atomically by OnScheduleMessagePumpWork (which
	// can be called from ANY thread — GPU, IO, renderer, etc.). The
	// persistent glib timer on the main thread reads and clears this flag.
	// Value: 0 = no work, 1 = work requested.
	pumpRequested atomic.Int32

	// Pump lifecycle managed only from the main GTK thread.
	pumpCB       glib.SourceFunc
	pumpSourceID uint
	pumpStarted  bool
	pumpClosing  bool

	// Reentrancy guard for DoMessageLoopWork.
	pumpActive  bool
	pumpReentry bool

	// Diagnostic counters (temporary).
	pumpTickCount uint64
	pumpWorkCount uint64
	scheduleCount atomic.Uint64 // incremented from any thread — read from main thread only

	scheduleImmediateCount atomic.Uint64
	scheduleDelayedCount   atomic.Uint64
	scheduleLastDelayMs    atomic.Int64
	scheduleMaxDelayMs     atomic.Int64

	contextInitializedCount atomic.Uint64
	childLaunchRenderer     atomic.Uint64
	childLaunchGPU          atomic.Uint64
	childLaunchOther        atomic.Uint64

	browserCreateRequests    atomic.Uint64
	browserAfterCreated      atomic.Uint64
	browserCreateLastAtMs    atomic.Int64
	browserCreateLastResult  atomic.Int32
	browserCreateLastWidth   atomic.Int32
	browserCreateLastHeight  atomic.Int32
	lastStallLoggedCreateSeq uint64
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
// Starts the persistent pump timer on the GTK main thread.
func (e *Engine) OnToolkitReady(_ context.Context) error {
	log := logging.FromContext(e.ctx)
	log.Debug().Msg("cef: OnToolkitReady called, starting message pump")
	e.startMessagePump()
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

const browserCreateStallWarnMS int64 = 1500

func (e *Engine) recordContextInitialized() {
	count := e.contextInitializedCount.Add(1)
	logging.FromContext(e.ctx).Debug().
		Uint64("count", count).
		Msg("cef: OnContextInitialized")
}

func (e *Engine) recordChildProcessLaunch(processType, useAngle, ozonePlatform, commandLine string) {
	switch processType {
	case "renderer":
		e.childLaunchRenderer.Add(1)
	case "gpu-process":
		e.childLaunchGPU.Add(1)
	default:
		e.childLaunchOther.Add(1)
	}

	logging.FromContext(e.ctx).Debug().
		Str("process_type", processType).
		Str("use_angle", useAngle).
		Str("ozone_platform", ozonePlatform).
		Str("command_line", commandLine).
		Uint64("renderer_launches", e.childLaunchRenderer.Load()).
		Uint64("gpu_launches", e.childLaunchGPU.Load()).
		Uint64("other_launches", e.childLaunchOther.Load()).
		Msg("cef: OnBeforeChildProcessLaunch")
}

func (e *Engine) recordBrowserCreateRequest(width, height, result int32) {
	count := e.browserCreateRequests.Add(1)
	e.browserCreateLastAtMs.Store(time.Now().UnixMilli())
	e.browserCreateLastResult.Store(result)
	e.browserCreateLastWidth.Store(width)
	e.browserCreateLastHeight.Store(height)

	log := logging.FromContext(e.ctx).Debug().
		Uint64("request_count", count).
		Int32("width", width).
		Int32("height", height).
		Int32("result", result).
		Msg
	if result != 1 {
		logging.FromContext(e.ctx).Warn().
			Uint64("request_count", count).
			Int32("width", width).
			Int32("height", height).
			Int32("result", result).
			Msg("cef: BrowserHostCreateBrowser returned non-success")
		return
	}
	log("cef: BrowserHostCreateBrowser returned")
}

func (e *Engine) recordBrowserAfterCreated(browser purecef.Browser) {
	count := e.browserAfterCreated.Add(1)
	browserID := int32(0)
	if browser != nil {
		browserID = browser.GetIdentifier()
	}
	logging.FromContext(e.ctx).Debug().
		Uint64("after_created_count", count).
		Uint64("create_requests", e.browserCreateRequests.Load()).
		Int32("browser_id", browserID).
		Msg("cef: browser created")
}

func (e *Engine) maybeLogBrowserCreateStall() {
	if e.pumpTickCount%250 != 0 {
		return
	}

	createCount := e.browserCreateRequests.Load()
	if createCount == 0 || createCount == e.browserAfterCreated.Load() || createCount == e.lastStallLoggedCreateSeq {
		return
	}

	createdAt := e.browserCreateLastAtMs.Load()
	if createdAt == 0 {
		return
	}

	ageMs := time.Now().UnixMilli() - createdAt
	if ageMs < browserCreateStallWarnMS {
		return
	}

	e.lastStallLoggedCreateSeq = createCount
	logging.FromContext(e.ctx).Warn().
		Uint64("create_requests", createCount).
		Uint64("after_created", e.browserAfterCreated.Load()).
		Int64("create_age_ms", ageMs).
		Int32("last_create_result", e.browserCreateLastResult.Load()).
		Int32("last_create_width", e.browserCreateLastWidth.Load()).
		Int32("last_create_height", e.browserCreateLastHeight.Load()).
		Uint64("pump_ticks", e.pumpTickCount).
		Uint64("pump_work", e.pumpWorkCount).
		Uint64("schedule_calls", e.scheduleCount.Load()).
		Uint64("schedule_immediate", e.scheduleImmediateCount.Load()).
		Uint64("schedule_delayed", e.scheduleDelayedCount.Load()).
		Int64("last_schedule_delay_ms", e.scheduleLastDelayMs.Load()).
		Int64("max_schedule_delay_ms", e.scheduleMaxDelayMs.Load()).
		Uint64("context_initialized", e.contextInitializedCount.Load()).
		Uint64("renderer_launches", e.childLaunchRenderer.Load()).
		Uint64("gpu_launches", e.childLaunchGPU.Load()).
		Uint64("other_launches", e.childLaunchOther.Load()).
		Msg("cef: browser creation appears stalled")
}

// ---------------------------------------------------------------------------
// CEF message pump — thread-safe via atomic signaling
//
// OnScheduleMessagePumpWork can be called from ANY Chromium thread (UI, IO,
// GPU compositor, etc.). It MUST NOT call any glib/GTK functions. Instead it
// sets an atomic flag. A persistent glib timer on the main GTK thread polls
// this flag and calls DoMessageLoopWork when work is pending.
//
// CPU cost when idle: one atomic load per tick (4ms) = negligible.
// Latency: max 4ms before CEF work is processed.
// ---------------------------------------------------------------------------

// scheduleMessagePumpWork is called by OnScheduleMessagePumpWork from any
// thread. It only sets an atomic flag — no glib calls, no allocations.
// IMPORTANT: No logging, no context access, no allocations here — this can
// be called from native CEF threads (GPU, IO, renderer).
func (e *Engine) scheduleMessagePumpWork(delayMs int64) {
	e.pumpRequested.Store(1)
	e.scheduleCount.Add(1)
	e.scheduleLastDelayMs.Store(delayMs)
	if delayMs <= 0 {
		e.scheduleImmediateCount.Add(1)
		return
	}

	e.scheduleDelayedCount.Add(1)
	for {
		current := e.scheduleMaxDelayMs.Load()
		if delayMs <= current {
			return
		}
		if e.scheduleMaxDelayMs.CompareAndSwap(current, delayMs) {
			return
		}
	}
}

func (e *Engine) startMessagePump() {
	if e.pumpStarted || e.pumpClosing {
		return
	}

	// Ensure there's a pending request so the first tick does work.
	e.pumpRequested.Store(1)

	e.pumpCB = glib.SourceFunc(func(_ uintptr) bool {
		if e.pumpClosing {
			return false
		}
		e.pumpTick()
		return true // keep the timer running
	})

	e.pumpSourceID = glib.TimeoutAdd(pumpTickMS, &e.pumpCB, 0)
	e.pumpStarted = true
}

func (e *Engine) stopMessagePump() {
	e.pumpClosing = true
	if e.pumpSourceID != 0 {
		glib.SourceRemove(e.pumpSourceID)
		e.pumpSourceID = 0
	}
}

// pumpTick is called on the GTK main thread by the persistent timer.
// It checks if CEF requested work and processes it.
func (e *Engine) pumpTick() {
	e.pumpTickCount++
	e.maybeLogBrowserCreateStall()
	// Check if work was requested (from any thread).
	if e.pumpRequested.Swap(0) == 0 {
		return // nothing to do
	}

	e.pumpWorkCount++
	if e.pumpWorkCount <= 20 || e.pumpWorkCount%100 == 0 {
		logging.FromContext(e.ctx).Debug().
			Uint64("tick", e.pumpTickCount).
			Uint64("work", e.pumpWorkCount).
			Uint64("schedule_calls", e.scheduleCount.Load()).
			Msg("cef: pump doing work")
	}
	e.performMessageLoopWork()
}

// performMessageLoopWork calls DoMessageLoopWork with reentrancy protection.
func (e *Engine) performMessageLoopWork() {
	if e.pumpActive {
		e.pumpReentry = true
		return
	}

	e.pumpActive = true
	e.pumpReentry = false
	purecef.DoMessageLoopWork()
	e.pumpActive = false

	// If CEF re-entered during the call (called OnScheduleMessagePumpWork
	// from within DoMessageLoopWork), the atomic flag is already set.
	// The next timer tick (within 4ms) will pick it up.
	// For immediate re-entrant work, do one more pass now.
	if e.pumpReentry {
		e.pumpActive = true
		e.pumpReentry = false
		purecef.DoMessageLoopWork()
		e.pumpActive = false
	}
}
