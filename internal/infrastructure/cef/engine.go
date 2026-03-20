package cef

import (
	"context"
	"os"
	"sync/atomic"
	"time"

	purecef "github.com/bnema/purego-cef/cef"
	"github.com/jwijenbergh/puregotk/v4/glib"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/logging"
)

// Compile-time interface check.
var _ port.Engine = (*Engine)(nil)

// cefFallbackPumpDelayMS mirrors the upstream external message pump fallback:
// even when no new callback arrives we still re-enter CefDoMessageLoopWork
// periodically so async browser creation and IPC continue to make progress.
const cefFallbackPumpDelayMS int64 = 1000 / 30

// Engine implements port.Engine for the CEF browser backend.
// It manages the CEF lifecycle and provides access to all engine subsystems.
type Engine struct {
	ctx     context.Context
	gl      *glLoader
	factory *WebViewFactory
	pool    *WebViewPool

	externalMessagePump bool
	manualPumpInterval  int64

	// nextWorkAtMs is the next deadline, in Unix milliseconds, when
	// CefDoMessageLoopWork should run. It is written from any Chromium thread
	// via OnScheduleMessagePumpWork and read on the GTK main thread.
	// Value: 0 means "nothing scheduled right now".
	nextWorkAtMs atomic.Int64

	// Pump lifecycle. Scheduling callbacks can originate from any Chromium
	// thread, while source callbacks execute on the GTK main thread.
	pumpEnabled atomic.Bool
	pumpClosing atomic.Bool

	pumpSignalPending atomic.Bool
	pumpSignalSource  atomic.Uint64
	pumpSignalRead    *os.File
	pumpSignalWrite   *os.File

	pumpTimerDueAtMs atomic.Int64
	pumpTimerSource  atomic.Uint64

	// Reentrancy guard for DoMessageLoopWork.
	pumpActive  bool
	pumpReentry bool

	// Diagnostic counters (temporary).
	pumpWakeCount   uint64
	pumpWorkCount   uint64
	pumpWorkTotalNs uint64
	pumpWorkMaxNs   int64
	pumpSlowCount   uint64
	scheduleCount   atomic.Uint64 // incremented from any thread — read from main thread only

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

	pumpDiagLastLogAt       time.Time
	pumpDiagLastWakeCount   uint64
	pumpDiagLastWorkCount   uint64
	pumpDiagLastSchedule    uint64
	pumpDiagLastWorkTotalNs uint64
	pumpDiagLastSlowCount   uint64
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
	log.Debug().
		Bool("external_message_pump", e.externalMessagePump).
		Int64("manual_pump_interval_ms", e.manualPumpInterval).
		Msg("cef: OnToolkitReady called, starting message pump")
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
		Uint64("pump_wakes", e.pumpWakeCount).
		Uint64("pump_work", e.pumpWorkCount).
		Uint64("schedule_calls", e.scheduleCount.Load()).
		Uint64("schedule_immediate", e.scheduleImmediateCount.Load()).
		Uint64("schedule_delayed", e.scheduleDelayedCount.Load()).
		Int64("last_schedule_delay_ms", e.scheduleLastDelayMs.Load()).
		Int64("max_schedule_delay_ms", e.scheduleMaxDelayMs.Load()).
		Int64("next_work_in_ms", e.nextWorkAtMs.Load()-time.Now().UnixMilli()).
		Uint64("context_initialized", e.contextInitializedCount.Load()).
		Uint64("renderer_launches", e.childLaunchRenderer.Load()).
		Uint64("gpu_launches", e.childLaunchGPU.Load()).
		Uint64("other_launches", e.childLaunchOther.Load()).
		Msg("cef: browser creation appears stalled")
}

// ---------------------------------------------------------------------------
// CEF message pump — thread-safe via atomic deadlines
//
// OnScheduleMessagePumpWork can be called from ANY Chromium thread (UI, IO,
// GPU compositor, etc.). It updates the next due time atomically and signals a
// persistent GLib fd watch via a wakeup pipe. This closely follows the Linux
// external-message-pump strategy used by upstream cefclient and avoids
// creating/destroying GLib sources from Chromium threads.
// ---------------------------------------------------------------------------

// scheduleMessagePumpWork is called by OnScheduleMessagePumpWork from any
// thread. It updates the earliest due deadline atomically and signals the GTK
// main thread when it needs to re-evaluate timer state.
func (e *Engine) scheduleMessagePumpWork(delayMs int64) {
	if !e.externalMessagePump {
		return
	}

	nowMs := time.Now().UnixMilli()
	dueAtMs := nowMs
	if delayMs > 0 {
		dueAtMs += delayMs
	}

	for {
		current := e.nextWorkAtMs.Load()
		if current != 0 && current <= dueAtMs {
			dueAtMs = current
			break
		}
		if e.nextWorkAtMs.CompareAndSwap(current, dueAtMs) {
			break
		}
	}

	e.scheduleCount.Add(1)
	e.scheduleLastDelayMs.Store(delayMs)
	if delayMs <= 0 {
		e.scheduleImmediateCount.Add(1)
	} else {
		e.scheduleDelayedCount.Add(1)
		for {
			current := e.scheduleMaxDelayMs.Load()
			if delayMs <= current {
				break
			}
			if e.scheduleMaxDelayMs.CompareAndSwap(current, delayMs) {
				break
			}
		}
	}

	if !e.pumpEnabled.Load() || e.pumpClosing.Load() {
		return
	}

	armedDueAtMs := e.pumpTimerDueAtMs.Load()
	if dueAtMs <= nowMs || armedDueAtMs == 0 || dueAtMs < armedDueAtMs {
		e.signalPump()
	}
}

func (e *Engine) startMessagePump() {
	if e.externalMessagePump {
		e.startExternalMessagePump()
		return
	}
	e.startManualMessagePump()
}

func (e *Engine) startExternalMessagePump() {
	if e.pumpEnabled.Load() {
		return
	}

	readFile, writeFile, err := os.Pipe()
	if err != nil {
		logging.FromContext(e.ctx).Error().Err(err).Msg("cef: failed to create wakeup pipe")
		return
	}

	cb := glib.UnixFDSourceFunc(func(_ int, _ glib.IOCondition, _ uintptr) bool {
		e.pumpSignalPending.Store(false)
		e.drainPumpSignal()
		if e.pumpClosing.Load() || !e.pumpEnabled.Load() {
			return glib.SOURCE_REMOVE
		}
		e.onPumpWake()
		return glib.SOURCE_CONTINUE
	})

	sourceID := glib.UnixFdAddFull(
		glib.PRIORITY_DEFAULT_IDLE,
		int(readFile.Fd()),
		glib.GIoInValue|glib.GIoHupValue|glib.GIoErrValue,
		&cb,
		0,
		nil,
	)

	if sourceID == 0 {
		_ = readFile.Close()
		_ = writeFile.Close()
		logging.FromContext(e.ctx).Error().Msg("cef: failed to attach wakeup pipe source")
		return
	}

	e.pumpSignalRead = readFile
	e.pumpSignalWrite = writeFile
	e.pumpSignalSource.Store(uint64(sourceID))
	e.pumpClosing.Store(false)
	e.pumpEnabled.Store(true)
	e.nextWorkAtMs.Store(time.Now().UnixMilli())
	e.signalPump()
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

		nowMs := time.Now().UnixMilli()
		e.pumpWakeCount++
		e.pumpWorkCount++
		e.maybeLogBrowserCreateStall()
		if e.pumpWorkCount <= 20 || e.pumpWorkCount%100 == 0 {
			logging.FromContext(e.ctx).Debug().
				Uint64("wake", e.pumpWakeCount).
				Uint64("work", e.pumpWorkCount).
				Int64("interval_ms", intervalMs).
				Int64("now_ms", nowMs).
				Msg("cef: manual pump doing work")
		}

		workStartedAt := time.Now()
		purecef.DoMessageLoopWork()
		workDuration := time.Since(workStartedAt)
		e.pumpWorkTotalNs += uint64(workDuration)
		if workDuration.Nanoseconds() > e.pumpWorkMaxNs {
			e.pumpWorkMaxNs = workDuration.Nanoseconds()
		}
		if workDuration >= 2*time.Millisecond {
			e.pumpSlowCount++
		}
		e.maybeLogPumpDiagnostics()

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
	e.pumpTimerDueAtMs.Store(time.Now().UnixMilli() + intervalMs)
	logging.FromContext(e.ctx).Info().
		Int64("interval_ms", intervalMs).
		Msg("cef: started manual CEF pump")
}

func (e *Engine) stopMessagePump() {
	e.pumpClosing.Store(true)
	e.pumpEnabled.Store(false)
	e.nextWorkAtMs.Store(0)
	e.pumpTimerDueAtMs.Store(0)
	if sourceID := uint(e.pumpTimerSource.Swap(0)); sourceID != 0 {
		glib.SourceRemove(sourceID)
	}
	if sourceID := uint(e.pumpSignalSource.Swap(0)); sourceID != 0 {
		glib.SourceRemove(sourceID)
	}
	if e.pumpSignalRead != nil {
		_ = e.pumpSignalRead.Close()
		e.pumpSignalRead = nil
	}
	if e.pumpSignalWrite != nil {
		_ = e.pumpSignalWrite.Close()
		e.pumpSignalWrite = nil
	}
}

func (e *Engine) scheduleFallbackPumpWork(nowMs int64) {
	if e.nextWorkAtMs.Load() != 0 {
		return
	}
	e.nextWorkAtMs.CompareAndSwap(0, nowMs+cefFallbackPumpDelayMS)
}

func (e *Engine) signalPump() {
	if e.pumpClosing.Load() || !e.pumpEnabled.Load() {
		return
	}
	if !e.pumpSignalPending.CompareAndSwap(false, true) {
		return
	}

	if e.pumpSignalWrite == nil {
		e.pumpSignalPending.Store(false)
		return
	}

	var wakeByte [1]byte
	wakeByte[0] = 1
	if _, err := e.pumpSignalWrite.Write(wakeByte[:]); err != nil {
		e.pumpSignalPending.Store(false)
	}
}

func (e *Engine) drainPumpSignal() {
	if e.pumpSignalRead == nil {
		return
	}

	var buf [64]byte
	_, _ = e.pumpSignalRead.Read(buf[:])
}

func (e *Engine) armPumpTimer(dueAtMs, nowMs int64) {
	if e.pumpClosing.Load() || !e.pumpEnabled.Load() {
		return
	}

	currentDueAtMs := e.pumpTimerDueAtMs.Load()
	if currentDueAtMs != 0 && currentDueAtMs <= dueAtMs {
		return
	}

	if sourceID := uint(e.pumpTimerSource.Swap(0)); sourceID != 0 {
		glib.SourceRemove(sourceID)
	}

	delayMs := dueAtMs - nowMs
	if delayMs <= 0 {
		delayMs = 1
	}
	if delayMs > cefFallbackPumpDelayMS {
		delayMs = cefFallbackPumpDelayMS
	}

	cb := glib.SourceOnceFunc(func(_ uintptr) {
		e.pumpTimerSource.Store(0)
		e.pumpTimerDueAtMs.Store(0)
		if e.pumpClosing.Load() || !e.pumpEnabled.Load() {
			return
		}
		e.onPumpWake()
	})

	sourceID := glib.TimeoutAddOnce(uint(delayMs), &cb, 0)
	e.pumpTimerSource.Store(uint64(sourceID))
	e.pumpTimerDueAtMs.Store(nowMs + delayMs)
}

func (e *Engine) disarmPumpTimer() {
	e.pumpTimerDueAtMs.Store(0)
	if sourceID := uint(e.pumpTimerSource.Swap(0)); sourceID != 0 {
		glib.SourceRemove(sourceID)
	}
}

// onPumpWake is called on the GTK main thread when the message pump should
// either process due CEF work immediately or arm a one-shot timer for the
// next deadline.
func (e *Engine) onPumpWake() {
	e.pumpWakeCount++
	e.maybeLogBrowserCreateStall()

	dueAtMs := e.nextWorkAtMs.Load()
	if dueAtMs == 0 {
		e.disarmPumpTimer()
		return
	}

	nowMs := time.Now().UnixMilli()
	if nowMs < dueAtMs {
		e.armPumpTimer(dueAtMs, nowMs)
		return
	}

	e.disarmPumpTimer()
	e.nextWorkAtMs.Store(0)
	e.pumpWorkCount++
	if e.pumpWorkCount <= 20 || e.pumpWorkCount%100 == 0 {
		logging.FromContext(e.ctx).Debug().
			Uint64("wake", e.pumpWakeCount).
			Uint64("work", e.pumpWorkCount).
			Uint64("schedule_calls", e.scheduleCount.Load()).
			Int64("due_at_ms", dueAtMs).
			Int64("now_ms", nowMs).
			Msg("cef: pump doing work")
	}
	workStartedAt := time.Now()
	wasReentrant := e.performMessageLoopWork()
	workDuration := time.Since(workStartedAt)
	e.pumpWorkTotalNs += uint64(workDuration)
	if workDuration.Nanoseconds() > e.pumpWorkMaxNs {
		e.pumpWorkMaxNs = workDuration.Nanoseconds()
	}
	if workDuration >= 2*time.Millisecond {
		e.pumpSlowCount++
	}

	// Match the upstream external-pump reference behavior: if CEF did not
	// request another deadline, keep a placeholder wakeup so async work such as
	// browser creation can continue progressing.
	nowMs = time.Now().UnixMilli()
	if wasReentrant {
		e.signalPump()
		return
	}
	e.scheduleFallbackPumpWork(nowMs)

	nextDueAtMs := e.nextWorkAtMs.Load()
	if nextDueAtMs == 0 {
		return
	}
	if nextDueAtMs <= nowMs {
		e.signalPump()
		e.maybeLogPumpDiagnostics()
		return
	}
	e.armPumpTimer(nextDueAtMs, nowMs)
	e.maybeLogPumpDiagnostics()
}

// performMessageLoopWork calls DoMessageLoopWork with reentrancy protection.
// It returns true when re-entrant work was detected and should be reposted.
func (e *Engine) performMessageLoopWork() bool {
	if e.pumpActive {
		e.pumpReentry = true
		return false
	}

	e.pumpActive = true
	e.pumpReentry = false
	purecef.DoMessageLoopWork()
	e.pumpActive = false

	return e.pumpReentry
}

func (e *Engine) maybeLogPumpDiagnostics() {
	now := time.Now()
	if !e.pumpDiagLastLogAt.IsZero() && now.Sub(e.pumpDiagLastLogAt) < 3*time.Second {
		return
	}

	wakeDelta := e.pumpWakeCount - e.pumpDiagLastWakeCount
	workDelta := e.pumpWorkCount - e.pumpDiagLastWorkCount
	scheduleTotal := e.scheduleCount.Load()
	scheduleDelta := scheduleTotal - e.pumpDiagLastSchedule
	workTotalNsDelta := e.pumpWorkTotalNs - e.pumpDiagLastWorkTotalNs
	slowDelta := e.pumpSlowCount - e.pumpDiagLastSlowCount
	if wakeDelta == 0 && workDelta == 0 && scheduleDelta == 0 {
		return
	}

	elapsed := now.Sub(e.pumpDiagLastLogAt)
	if e.pumpDiagLastLogAt.IsZero() || elapsed <= 0 {
		elapsed = time.Second
	}
	elapsedSec := elapsed.Seconds()

	avgWorkUs := float64(0)
	if workDelta > 0 {
		avgWorkUs = float64(workTotalNsDelta) / float64(workDelta) / 1_000
	}

	logging.FromContext(e.ctx).Debug().
		Float64("wake_hz", float64(wakeDelta)/elapsedSec).
		Float64("work_hz", float64(workDelta)/elapsedSec).
		Float64("schedule_hz", float64(scheduleDelta)/elapsedSec).
		Uint64("wake_delta", wakeDelta).
		Uint64("work_delta", workDelta).
		Uint64("schedule_delta", scheduleDelta).
		Float64("avg_do_work_us", avgWorkUs).
		Float64("max_do_work_us", float64(e.pumpWorkMaxNs)/1_000).
		Uint64("slow_do_work_delta", slowDelta).
		Uint64("wake_total", e.pumpWakeCount).
		Uint64("work_total", e.pumpWorkCount).
		Uint64("schedule_total", scheduleTotal).
		Msg("cef: pump activity")

	e.pumpDiagLastLogAt = now
	e.pumpDiagLastWakeCount = e.pumpWakeCount
	e.pumpDiagLastWorkCount = e.pumpWorkCount
	e.pumpDiagLastSchedule = scheduleTotal
	e.pumpDiagLastWorkTotalNs = e.pumpWorkTotalNs
	e.pumpDiagLastSlowCount = e.pumpSlowCount
}
