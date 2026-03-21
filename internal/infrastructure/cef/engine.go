package cef

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	purecef "github.com/bnema/purego-cef/cef"
	"github.com/bnema/puregotk/v4/glib"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/logging"
)

// Compile-time interface check.
var _ port.Engine = (*Engine)(nil)

// Engine implements port.Engine for the CEF browser backend.
// It manages the CEF lifecycle and provides access to all engine subsystems.
type Engine struct {
	ctx     context.Context
	gl      *glLoader
	factory *WebViewFactory
	pool    *WebViewPool

	messageRouter *MessageRouter
	schemeHandler *dumbSchemeHandler
	contentInj    *contentInjector

	// activeWebViews tracks all live webviews for CSS broadcast.
	activeWebViews sync.Map // map[port.WebViewID]*WebView

	multiThreadedMessageLoop bool
	manualPumpInterval       int64

	// Pump lifecycle (manual pump mode only).
	pumpEnabled     atomic.Bool
	pumpClosing     atomic.Bool
	pumpTimerSource atomic.Uint64

	// Diagnostic counters.
	pumpWorkCount atomic.Uint64
	scheduleCount atomic.Uint64 // OnScheduleMessagePumpWork call count (currently always 0)

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
	lastStallLoggedCreateSeq atomic.Uint64
}

// Factory returns the WebViewFactory for creating new WebView instances.
func (e *Engine) Factory() port.WebViewFactory {
	return &webViewFactoryAdapter{factory: e.factory}
}

// Pool returns the WebViewPool for acquiring pre-warmed WebView instances.
func (e *Engine) Pool() port.WebViewPool {
	return &webViewPoolAdapter{pool: e.pool, ctx: e.ctx}
}

// ContentInjector returns the CEF content injector for script/style injection.
func (e *Engine) ContentInjector() port.ContentInjector {
	if e.contentInj != nil {
		return e.contentInj
	}
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

// SetColorResolver sets the color scheme resolver on the content injector.
// This allows dark mode detection for internal pages. Safe to call after
// engine creation (e.g., from bootstrap wiring).
func (e *Engine) SetColorResolver(resolver port.ColorSchemeResolver) {
	if e.contentInj != nil {
		e.contentInj.setColorResolver(resolver)
	}
}

// registerWebView adds a webview to the active tracking map.
func (e *Engine) registerWebView(wv *WebView) {
	e.activeWebViews.Store(wv.id, wv)
}

// unregisterWebView removes a webview from the active tracking map.
func (e *Engine) unregisterWebView(wv *WebView) {
	e.activeWebViews.Delete(wv.id)
}

// internalSchemePath is the URI scheme used for internal app resources.
const internalSchemePath = "dumb://"

// InternalSchemePath returns the URI scheme used for internal app resources.
func (e *Engine) InternalSchemePath() string {
	return internalSchemePath
}

// Close releases all resources held by the engine.
func (e *Engine) Close() error {
	e.stopMessagePump()
	e.pool.Close()
	purecef.Shutdown()
	return nil
}

// RegisterHandlers registers all WebUI message bridge handlers with the message router.
// Handler registration follows the same pattern as WebKit's handlers/register.go:
// each handler is registered by message type with the router, which dispatches
// incoming /api/message POSTs to the correct handler based on Message.Type.
func (e *Engine) RegisterHandlers(ctx context.Context, deps port.HandlerDependencies) error {
	if e.messageRouter == nil {
		return nil
	}
	log := logging.FromContext(ctx)
	// TODO: Wire homepage, config, keybindings, and clipboard handlers
	// into e.messageRouter. Each handler implements MessageHandler and is
	// registered by message type (e.g. "get_recent_history", "get_favorites").
	// The CEF MessageRouter dispatches based on Message.Type from the JS bridge.
	_ = deps

	log.Warn().Msg("cef: RegisterHandlers stub — handler wiring not yet implemented")
	return nil
}

// RegisterAccentHandlers registers accent/dead-key input handlers with the message router.
func (e *Engine) RegisterAccentHandlers(ctx context.Context, handler port.AccentKeyHandler) error {
	if e.messageRouter == nil || handler == nil {
		return nil
	}
	log := logging.FromContext(ctx)

	if err := e.messageRouter.RegisterHandler("accent_key_press", MessageHandlerFunc(
		func(ctx context.Context, _ uint64, payload json.RawMessage) (any, error) {
			var p struct {
				Char  string `json:"char"`
				Shift bool   `json:"shift"`
			}
			if err := json.Unmarshal(payload, &p); err != nil {
				return nil, err
			}
			if r, _ := utf8.DecodeRuneInString(p.Char); r != utf8.RuneError && utf8.RuneCountInString(p.Char) == 1 {
				handler.OnKeyPressed(ctx, r, p.Shift)
			}
			return nil, nil
		},
	)); err != nil {
		return err
	}

	if err := e.messageRouter.RegisterHandler("accent_key_release", MessageHandlerFunc(
		func(ctx context.Context, _ uint64, payload json.RawMessage) (any, error) {
			var p struct {
				Char string `json:"char"`
			}
			if err := json.Unmarshal(payload, &p); err != nil {
				return nil, err
			}
			if r, _ := utf8.DecodeRuneInString(p.Char); r != utf8.RuneError && utf8.RuneCountInString(p.Char) == 1 {
				handler.OnKeyReleased(ctx, r)
			}
			return nil, nil
		},
	)); err != nil {
		return err
	}

	log.Info().Msg("cef: accent handlers registered")
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
	log.Debug().
		Bool("multi_threaded_message_loop", e.multiThreadedMessageLoop).
		Int64("manual_pump_interval_ms", e.manualPumpInterval).
		Msg("cef: OnToolkitReady called, starting message pump")
	e.startMessagePump()
	return nil
}

// UpdateAppearance updates the default background color for new WebViews.
func (e *Engine) UpdateAppearance(_ context.Context, r, g, b, alpha float64) error {
	if e.factory != nil {
		e.factory.setDefaultBackgroundColor(r, g, b, alpha)
	}
	return nil
}

// UpdateSettings applies runtime config changes to engine internals (Phase 1 stub).
func (e *Engine) UpdateSettings(_ context.Context, _ port.EngineSettingsUpdate) error {
	return nil
}

// SetHandlerContext sets the base context for message handler dispatch.
func (e *Engine) SetHandlerContext(ctx context.Context) {
	e.ctx = ctx
	if e.messageRouter != nil {
		e.messageRouter.SetBaseContext(ctx)
	}
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

	logging.FromContext(e.ctx).Debug().
		Uint64("request_count", count).
		Int32("width", width).
		Int32("height", height).
		Int32("result", result).
		Msg("cef: BrowserHostCreateBrowser returned")
	if result != 1 {
		logging.FromContext(e.ctx).Warn().
			Uint64("request_count", count).
			Int32("width", width).
			Int32("height", height).
			Int32("result", result).
			Msg("cef: BrowserHostCreateBrowser returned non-success")
	}
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
	createCount := e.browserCreateRequests.Load()
	if createCount == 0 || createCount == e.browserAfterCreated.Load() || createCount == e.lastStallLoggedCreateSeq.Load() {
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

	e.lastStallLoggedCreateSeq.Store(createCount)
	logging.FromContext(e.ctx).Warn().
		Uint64("create_requests", createCount).
		Uint64("after_created", e.browserAfterCreated.Load()).
		Int64("create_age_ms", ageMs).
		Int32("last_create_result", e.browserCreateLastResult.Load()).
		Int32("last_create_width", e.browserCreateLastWidth.Load()).
		Int32("last_create_height", e.browserCreateLastHeight.Load()).
		Uint64("pump_work", e.pumpWorkCount.Load()).
		Uint64("schedule_calls", e.scheduleCount.Load()).
		Uint64("context_initialized", e.contextInitializedCount.Load()).
		Uint64("renderer_launches", e.childLaunchRenderer.Load()).
		Uint64("gpu_launches", e.childLaunchGPU.Load()).
		Uint64("other_launches", e.childLaunchOther.Load()).
		Msg("cef: browser creation appears stalled")
}

// ---------------------------------------------------------------------------
// CEF message pump
// ---------------------------------------------------------------------------

// scheduleMessagePumpWork is called by OnScheduleMessagePumpWork from any
// thread. External message pump is disabled (purego-cef bug), so this is a
// no-op with a diagnostic counter.
func (e *Engine) scheduleMessagePumpWork(_ int64) {
	e.scheduleCount.Add(1)
}

func (e *Engine) startMessagePump() {
	if e.multiThreadedMessageLoop {
		logging.FromContext(e.ctx).Info().Msg("cef: multi-threaded message loop enabled, skipping manual pump")
		return
	}
	e.startManualMessagePump()
}

func (e *Engine) startManualMessagePump() {
	if e.pumpEnabled.Load() {
		return
	}

	intervalMs := e.manualPumpInterval
	if intervalMs <= 0 {
		intervalMs = 10
	}

	cb := glib.SourceFunc(func(_ uintptr) bool {
		if e.pumpClosing.Load() || !e.pumpEnabled.Load() {
			return glib.SOURCE_REMOVE
		}

		count := e.pumpWorkCount.Add(1)
		e.maybeLogBrowserCreateStall()
		if count <= 20 || count%100 == 0 {
			logging.FromContext(e.ctx).Debug().
				Uint64("work", count).
				Int64("interval_ms", intervalMs).
				Msg("cef: manual pump doing work")
		}

		purecef.DoMessageLoopWork()
		return glib.SOURCE_CONTINUE
	})

	sourceID := glib.TimeoutAdd(uint(intervalMs), &cb, 0)
	if sourceID == 0 {
		logging.FromContext(e.ctx).Error().
			Int64("interval_ms", intervalMs).
			Msg("cef: failed to start manual pump timer")
		return
	}

	e.pumpClosing.Store(false)
	e.pumpEnabled.Store(true)
	e.pumpTimerSource.Store(uint64(sourceID))
	logging.FromContext(e.ctx).Info().
		Int64("interval_ms", intervalMs).
		Msg("cef: started manual CEF pump")
}

func (e *Engine) stopMessagePump() {
	if e.multiThreadedMessageLoop {
		return
	}
	e.pumpClosing.Store(true)
	e.pumpEnabled.Store(false)
	if sourceID := uint(e.pumpTimerSource.Swap(0)); sourceID != 0 {
		glib.SourceRemove(sourceID)
	}
}
