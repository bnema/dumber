package cef

import (
	"context"

	purecef "github.com/bnema/purego-cef/cef"
	"github.com/jwijenbergh/puregotk/v4/glib"
	"github.com/rs/zerolog"

	"github.com/bnema/dumber/internal/application/port"
)

// Compile-time interface check.
var _ port.Engine = (*Engine)(nil)

// cefPumpIntervalMS matches CEF's own kMaxTimerDelay (1000/30 ≈ 33ms) used in
// the reference external message pump implementation. This is a conservative
// fallback; Phase 2 will use OnScheduleMessagePumpWork for precise scheduling.
const cefPumpIntervalMS uint = 33

// Engine implements port.Engine for the CEF browser backend.
// It manages the CEF lifecycle and provides access to all engine subsystems.
type Engine struct {
	ctx     context.Context
	gl      *glLoader
	factory *WebViewFactory
	pool    *WebViewPool
	logger  zerolog.Logger

	// Message pump state. The pump is a repeating glib.TimeoutAdd source
	// started from OnToolkitReady (not before GTK init).
	pumpCB       glib.SourceFunc
	pumpSourceID uint
	pumpStarted  bool
	pumpClosing  bool

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
// This is the earliest safe point to start the CEF message pump — GTK and
// libadwaita are fully initialized, and the GLib main context is running.
func (e *Engine) OnToolkitReady(_ context.Context) error {
	e.logger.Debug().Msg("cef: OnToolkitReady called, starting message pump")
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

// ---------------------------------------------------------------------------
// CEF message pump (fixed-interval fallback, Phase 1)
// ---------------------------------------------------------------------------

func (e *Engine) startMessagePump() {
	if e.pumpStarted || e.pumpClosing {
		return
	}

	e.pumpCB = glib.SourceFunc(func(_ uintptr) bool {
		if e.pumpClosing {
			e.pumpSourceID = 0
			return false // remove the source
		}
		e.performMessageLoopWork()
		return !e.pumpClosing
	})

	e.pumpSourceID = glib.TimeoutAdd(cefPumpIntervalMS, &e.pumpCB, 0)
	if e.pumpSourceID == 0 {
		e.logger.Error().Msg("cef: failed to install message pump timer")
		return
	}

	e.pumpStarted = true
	e.logger.Debug().
		Uint("interval_ms", cefPumpIntervalMS).
		Msg("cef: started message pump")
}

func (e *Engine) stopMessagePump() {
	e.pumpClosing = true
	if e.pumpSourceID != 0 {
		glib.SourceRemove(e.pumpSourceID)
		e.pumpSourceID = 0
	}
}

// performMessageLoopWork calls DoMessageLoopWork with reentrancy protection.
// Matches the pattern from CEF's main_message_loop_external_pump.cc:
// if we're already inside DoMessageLoopWork, just flag reentrancy and return.
var pumpCallCount uint64

func (e *Engine) performMessageLoopWork() {
	if e.pumpActive {
		e.logger.Warn().Msg("cef: pump reentrancy detected, deferring")
		e.pumpReentry = true
		return
	}

	pumpCallCount++
	if pumpCallCount <= 5 {
		e.logger.Debug().Uint64("call", pumpCallCount).Msg("cef: DoMessageLoopWork enter")
	}

	e.pumpActive = true
	e.pumpReentry = false
	purecef.DoMessageLoopWork()
	e.pumpActive = false

	if pumpCallCount <= 5 {
		e.logger.Debug().Uint64("call", pumpCallCount).Bool("reentry", e.pumpReentry).Msg("cef: DoMessageLoopWork exit")
	}

	// If CEF re-entered during the call, do one more pass immediately.
	// Cap at one extra pass to avoid unbounded recursion.
	if e.pumpReentry {
		e.logger.Debug().Msg("cef: pump reentry follow-up pass")
		e.pumpReentry = false
		e.pumpActive = true
		purecef.DoMessageLoopWork()
		e.pumpActive = false
	}
}
