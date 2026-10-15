package cef

import (
	"context"
	"sync"
	"testing"
	"time"

	purecef "github.com/bnema/purego-cef/cef"
	"github.com/stretchr/testify/require"

	"github.com/bnema/dumber/internal/application/port"
)

// gpuRecoveryHost records render-host calls. pulses counts Invalidate calls
// (two per pulse); ops records the full call sequence.
type gpuRecoveryHost struct {
	purecef.BrowserHost
	pulses int
	ops    []string
}

func (h *gpuRecoveryHost) Invalidate(purecef.PaintElementType) {
	h.pulses++
	h.ops = append(h.ops, "Invalidate")
}

func (h *gpuRecoveryHost) WasHidden(hidden int32) {
	if hidden != 0 {
		h.ops = append(h.ops, "WasHidden(1)")
		return
	}
	h.ops = append(h.ops, "WasHidden(0)")
}

func (h *gpuRecoveryHost) NotifyScreenInfoChanged() {
	h.ops = append(h.ops, "NotifyScreenInfoChanged")
}

func (h *gpuRecoveryHost) WasResized() { h.ops = append(h.ops, "WasResized") }

type gpuRecoveryBrowser struct {
	purecef.Browser
	id      int32
	reloads int
}

func (b *gpuRecoveryBrowser) GetIdentifier() int32 { return b.id }
func (b *gpuRecoveryBrowser) Reload()              { b.reloads++ }

type scheduledCall struct {
	delay time.Duration
	fn    func()
}

type gpuRecoveryHarness struct {
	mu        sync.Mutex
	scheduled []scheduledCall
	paints    map[*Cef2gtkAdapter]int
	// beforePost, when set, runs right before a posted UI task executes. It
	// lets tests change view state between scheduling and execution.
	beforePost func()
	// holdPosts queues posted UI tasks instead of running them; runHeld runs
	// them later, like the CEF UI thread draining its queue.
	holdPosts bool
	held      []purecef.Task
}

func (h *gpuRecoveryHarness) runHeld() {
	h.mu.Lock()
	tasks := h.held
	h.held = nil
	h.mu.Unlock()
	for _, task := range tasks {
		task.Execute()
	}
}

func (h *gpuRecoveryHarness) setPaints(wv *WebView, n int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.paints[wv.viewBridge] = n
}

func (h *gpuRecoveryHarness) pending() []scheduledCall {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]scheduledCall(nil), h.scheduled...)
}

// newGPURecoveryHarness replaces the CEF task/scheduler seams. Posted UI
// tasks run inline; scheduled callbacks are queued until flush is called.
func newGPURecoveryHarness(t *testing.T) *gpuRecoveryHarness {
	t.Helper()
	h := &gpuRecoveryHarness{paints: map[*Cef2gtkAdapter]int{}}
	oldNew, oldPost, oldAfter, oldPaints := cefNewTask, cefPostTask, cefScheduleAfter, gpuRecoveryPaintCount
	t.Cleanup(func() {
		cefNewTask, cefPostTask, cefScheduleAfter, gpuRecoveryPaintCount = oldNew, oldPost, oldAfter, oldPaints
	})
	cefNewTask = func(task purecef.Task) purecef.Task { return task }
	cefPostTask = func(_ purecef.ThreadID, task purecef.Task) int32 {
		if h.holdPosts {
			h.mu.Lock()
			h.held = append(h.held, task)
			h.mu.Unlock()
			return 1
		}
		if h.beforePost != nil {
			h.beforePost()
		}
		task.Execute()
		return 1
	}
	cefScheduleAfter = func(delay time.Duration, fn func()) {
		h.mu.Lock()
		defer h.mu.Unlock()
		h.scheduled = append(h.scheduled, scheduledCall{delay: delay, fn: fn})
	}
	gpuRecoveryPaintCount = func(bridge *Cef2gtkAdapter) int {
		h.mu.Lock()
		defer h.mu.Unlock()
		return h.paints[bridge]
	}
	return h
}

// flush runs and clears everything currently scheduled.
func (h *gpuRecoveryHarness) flush() {
	h.mu.Lock()
	calls := h.scheduled
	h.scheduled = nil
	h.mu.Unlock()
	for _, c := range calls {
		c.fn()
	}
}

func newGPURecoveryView(id port.WebViewID) (*WebView, *gpuRecoveryHost, *gpuRecoveryBrowser) {
	host := &gpuRecoveryHost{}
	browser := &gpuRecoveryBrowser{id: int32(id)}
	wv := &WebView{
		id:         id,
		ctx:        context.Background(),
		host:       host,
		browser:    browser,
		viewBridge: &Cef2gtkAdapter{},
	}
	return wv, host, browser
}

func newGPURecoveryEngine(views ...*WebView) *Engine {
	e := &Engine{ctx: context.Background()}
	for _, wv := range views {
		e.activeWebViews.Store(wv.id, wv)
	}
	return e
}

func TestGPURecovery_FirstLaunchDoesNothing(t *testing.T) {
	h := newGPURecoveryHarness(t)
	e := newGPURecoveryEngine()

	e.recordChildProcessLaunch("gpu-process", "", "", "", "")

	require.Empty(t, h.pending())
}

func TestGPURecovery_NonGPULaunchDoesNothing(t *testing.T) {
	h := newGPURecoveryHarness(t)
	e := newGPURecoveryEngine()

	e.recordChildProcessLaunch("renderer", "", "", "", "")
	e.recordChildProcessLaunch("renderer", "", "", "", "")

	require.Empty(t, h.pending())
}

func TestGPURecovery_SecondLaunchSchedulesOnePass(t *testing.T) {
	h := newGPURecoveryHarness(t)
	e := newGPURecoveryEngine()

	e.recordChildProcessLaunch("gpu-process", "", "", "", "")
	e.recordChildProcessLaunch("gpu-process", "", "", "", "")

	require.Len(t, h.pending(), 1)
	require.Equal(t, gpuRecoveryDelay, h.pending()[0].delay)
}

func TestGPURecovery_BurstIsDebouncedToOnePass(t *testing.T) {
	h := newGPURecoveryHarness(t)
	wv, host, _ := newGPURecoveryView(1)
	e := newGPURecoveryEngine(wv)

	for i := 0; i < 5; i++ {
		e.recordChildProcessLaunch("gpu-process", "", "", "", "")
	}
	require.Len(t, h.pending(), 1, "launches 2..5 must share one pass")

	h.flush() // run the pass: one pulse per view
	require.Equal(t, 2, host.pulses, "one pulse = two Invalidate calls")

	// After the pass ran, a new crash is allowed to schedule another pass.
	e.recordChildProcessLaunch("gpu-process", "", "", "", "")
	require.Len(t, h.pending(), 2, "verify callback plus a new pass")
}

func TestGPURecovery_ViewThatPaintsIsNotReloaded(t *testing.T) {
	h := newGPURecoveryHarness(t)
	wv, host, browser := newGPURecoveryView(1)
	e := newGPURecoveryEngine(wv)
	h.setPaints(wv, 10)

	e.runGPURecoveryPass()
	require.Equal(t, []string{
		"Invalidate", "WasHidden(1)", "WasHidden(0)",
		"NotifyScreenInfoChanged", "WasResized", "Invalidate",
	}, host.ops, "pulse must reset visibility and screen info, then repaint")
	require.Len(t, h.pending(), 1)
	require.Equal(t, gpuRecoveryVerifyDelay, h.pending()[0].delay)

	h.setPaints(wv, 11) // a frame arrived after the pulse
	h.flush()

	require.Zero(t, browser.reloads)
}

func TestGPURecovery_ViewThatDoesNotPaintIsReloaded(t *testing.T) {
	h := newGPURecoveryHarness(t)
	wv, _, browser := newGPURecoveryView(1)
	e := newGPURecoveryEngine(wv)
	h.setPaints(wv, 10)

	e.runGPURecoveryPass()
	h.flush() // paints unchanged

	require.Equal(t, 1, browser.reloads)
}

func TestGPURecovery_OnlyStalledViewsAreReloaded(t *testing.T) {
	h := newGPURecoveryHarness(t)
	healthy, _, healthyBrowser := newGPURecoveryView(1)
	frozen, _, frozenBrowser := newGPURecoveryView(2)
	e := newGPURecoveryEngine(healthy, frozen)

	e.runGPURecoveryPass()
	h.setPaints(healthy, 1)
	h.flush()

	require.Zero(t, healthyBrowser.reloads)
	require.Equal(t, 1, frozenBrowser.reloads)
}

func TestGPURecovery_DestroyedViewsAreSkipped(t *testing.T) {
	h := newGPURecoveryHarness(t)
	wv, host, browser := newGPURecoveryView(1)
	wv.destroyed.Store(true)
	e := newGPURecoveryEngine(wv)

	e.runGPURecoveryPass()
	h.flush()

	require.Zero(t, host.pulses)
	require.Zero(t, browser.reloads)
}

func TestGPURecovery_ViewDestroyedBeforeVerifyIsNotReloaded(t *testing.T) {
	h := newGPURecoveryHarness(t)
	wv, _, browser := newGPURecoveryView(1)
	e := newGPURecoveryEngine(wv)

	e.runGPURecoveryPass()
	wv.destroyed.Store(true)
	h.flush()

	require.Zero(t, browser.reloads)
}

func TestGPURecovery_ViewWithoutHostOrBrowserIsSkipped(t *testing.T) {
	h := newGPURecoveryHarness(t)
	noHost, _, _ := newGPURecoveryView(1)
	noHost.host = nil
	noBrowser, host, _ := newGPURecoveryView(2)
	noBrowser.browser = nil
	e := newGPURecoveryEngine(noHost, noBrowser)

	e.runGPURecoveryPass()

	require.Empty(t, h.pending())
	require.Zero(t, host.pulses)
}

func TestGPURecovery_HiddenViewIsDeferredUntilVisible(t *testing.T) {
	h := newGPURecoveryHarness(t)
	wv, host, browser := newGPURecoveryView(1)
	wv.effectiveVisibilityKnown = true
	wv.effectiveVisible = false
	e := newGPURecoveryEngine(wv)

	e.runGPURecoveryPass()
	h.flush()
	require.Zero(t, host.pulses, "hidden views are not pulsed")
	require.Zero(t, browser.reloads, "hidden views are not reloaded")
	require.True(t, pendingOf(wv))

	// Becoming visible restarts recovery for that view only.
	wv.applyEffectiveVisibility(host, true)
	require.False(t, pendingOf(wv))
	require.Equal(t, 2, host.pulses)
	h.flush() // no paint arrived
	require.Equal(t, 1, browser.reloads)
}

func TestGPURecovery_ViewHiddenDuringVerifyIsDeferredNotReloaded(t *testing.T) {
	h := newGPURecoveryHarness(t)
	wv, _, browser := newGPURecoveryView(1)
	e := newGPURecoveryEngine(wv)

	e.runGPURecoveryPass()
	wv.effectiveVisibilityKnown = true
	wv.effectiveVisible = false
	h.flush()

	require.Zero(t, browser.reloads)
	require.True(t, pendingOf(wv))
}

func TestGPURecovery_IsNotGatedByStallRecoveryEnv(t *testing.T) {
	h := newGPURecoveryHarness(t)
	t.Setenv(cefRenderStallRecoveryEnvVar, "0")
	wv, host, _ := newGPURecoveryView(1)
	e := newGPURecoveryEngine(wv)

	e.runGPURecoveryPass()

	require.False(t, renderStallRecoveryEnabled())
	require.Equal(t, 2, host.pulses)
	require.Len(t, h.pending(), 1)
}

func pendingOf(wv *WebView) bool {
	wv.mu.RLock()
	defer wv.mu.RUnlock()
	return wv.gpuRecoveryPending
}

func markHidden(wv *WebView) {
	wv.mu.Lock()
	wv.effectiveVisibilityKnown = true
	wv.effectiveVisible = false
	wv.mu.Unlock()
}

// GO-001: the old code could observe "hidden", lose the race to a show that
// consumed a still-false pending flag, then store pending=true on a visible
// view. Check and store are now one critical section, so both orderings of
// "defer" vs "show" end with the view recovered exactly once.

func TestGPURecovery_LostWakeup_DeferThenShowResumes(t *testing.T) {
	h := newGPURecoveryHarness(t)
	wv, host, browser := newGPURecoveryView(1)
	markHidden(wv)

	require.True(t, wv.deferGPURecoveryIfHidden())
	require.True(t, pendingOf(wv))

	wv.applyEffectiveVisibility(host, true)
	require.False(t, pendingOf(wv))
	require.Equal(t, 2, host.pulses, "show consumed pending and restarted recovery")
	h.flush()
	require.Equal(t, 1, browser.reloads)
}

func TestGPURecovery_LostWakeup_ShowThenDeferDoesNotLeavePending(t *testing.T) {
	h := newGPURecoveryHarness(t)
	wv, host, _ := newGPURecoveryView(1)
	markHidden(wv)

	wv.applyEffectiveVisibility(host, true) // show wins the race
	require.False(t, wv.deferGPURecoveryIfHidden(), "visible view must not be deferred")
	require.False(t, pendingOf(wv), "no stranded pending flag on a visible view")

	_, deferred := wv.claimGPURecovery()
	require.False(t, deferred)
	require.False(t, pendingOf(wv))
	require.Empty(t, h.pending())
}

func TestGPURecovery_ConcurrentShowAndDeferNeverStrandsPending(t *testing.T) {
	newGPURecoveryHarness(t)
	wv, host, _ := newGPURecoveryView(1)
	markHidden(wv)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			wv.applyEffectiveVisibility(host, false)
			wv.applyEffectiveVisibility(host, true)
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			wv.deferGPURecoveryIfHidden()
			wv.claimGPURecovery()
		}
	}()
	wg.Wait()

	// The visibility goroutine ended visible. Whatever the interleaving, a
	// visible view must not keep a pending flag nobody will ever consume.
	wv.applyEffectiveVisibility(host, true)
	require.False(t, pendingOf(wv))
}

// GO-003: visibility is revalidated when the pulse task runs on the UI thread.

func TestGPURecovery_PulseSkipsViewHiddenAfterScheduling(t *testing.T) {
	h := newGPURecoveryHarness(t)
	wv, host, browser := newGPURecoveryView(1)
	e := newGPURecoveryEngine(wv)
	h.beforePost = func() { markHidden(wv) } // hidden after scheduling

	e.runGPURecoveryPass()

	require.Zero(t, host.pulses, "hidden view must not be pulsed (WasHidden(0) would show it)")
	require.True(t, pendingOf(wv))
	h.beforePost = nil
	h.flush() // verify of the same attempt: still hidden
	require.Zero(t, browser.reloads)

	wv.applyEffectiveVisibility(host, true)
	require.Equal(t, 2, host.pulses)
}

func TestRenderStallPulseSkipsViewHiddenAfterScheduling(t *testing.T) {
	h := newGPURecoveryHarness(t)
	t.Setenv(cefRenderStallRecoveryEnvVar, "1")
	wv, host, _ := newGPURecoveryView(1)
	h.beforePost = func() { markHidden(wv) }

	wv.maybeRunRenderStallRecovery(time.Now(), nil, 5)

	require.Zero(t, host.pulses)
}

func TestRenderStallRecoveryStillPulsesVisibleViewWhenEnabled(t *testing.T) {
	h := newGPURecoveryHarness(t)
	t.Setenv(cefRenderStallRecoveryEnvVar, "1")
	wv, host, _ := newGPURecoveryView(1)

	wv.maybeRunRenderStallRecovery(time.Now(), nil, 5)

	require.Equal(t, 2, host.pulses)
	require.Len(t, h.pending(), 1)
	require.Equal(t, renderRecoveryVerifyDelay, h.pending()[0].delay)
}

func TestRenderStallRecoveryDisabledByDefault(t *testing.T) {
	h := newGPURecoveryHarness(t)
	t.Setenv(cefRenderStallRecoveryEnvVar, "0")
	wv, host, _ := newGPURecoveryView(1)

	wv.maybeRunRenderStallRecovery(time.Now(), nil, 5)

	require.Zero(t, host.pulses)
	require.Empty(t, h.pending())
}

// GO-004: per-view recovery generation.

func TestGPURecovery_StaleVerifyIgnoredAfterNewerAttempt(t *testing.T) {
	h := newGPURecoveryHarness(t)
	wv, host, browser := newGPURecoveryView(1)
	e := newGPURecoveryEngine(wv)

	e.runGPURecoveryPass() // attempt 1
	first := h.pending()
	require.Len(t, first, 1)
	e.runGPURecoveryPass() // a new crash supersedes it: attempt 2
	require.Equal(t, 4, host.pulses)
	require.Len(t, h.pending(), 2)

	first[0].fn() // stale verify of attempt 1 fires "too early"
	require.Zero(t, browser.reloads, "stale verify must not reload")

	h.flush() // attempt 1 again (stale) plus attempt 2
	require.Equal(t, 1, browser.reloads, "only the newest attempt reloads, once")
}

func TestGPURecovery_ShowAfterNewCrashSupersedesInFlightAttempt(t *testing.T) {
	h := newGPURecoveryHarness(t)
	wv, host, browser := newGPURecoveryView(1)
	e := newGPURecoveryEngine(wv)

	e.runGPURecoveryPass() // visible attempt 1
	stale := h.pending()[0]
	markHidden(wv)
	e.runGPURecoveryPass() // hidden: attempt 2 deferred, pending
	require.True(t, pendingOf(wv))

	wv.applyEffectiveVisibility(host, true) // attempt 3 starts, clears pending
	require.False(t, pendingOf(wv))

	stale.fn()
	require.Zero(t, browser.reloads, "verify of attempt 1 is stale")
	h.flush()
	require.Equal(t, 1, browser.reloads, "exactly one reload for the newest attempt")
}

func TestGPURecovery_StalePulseTaskSkipped(t *testing.T) {
	h := newGPURecoveryHarness(t)
	wv, host, _ := newGPURecoveryView(1)
	e := newGPURecoveryEngine(wv)
	// A newer attempt starts between posting and executing the pulse task.
	h.beforePost = func() { wv.gpuRecoveryGen.Add(1) }

	e.runGPURecoveryPass()

	require.Zero(t, host.pulses)
}

// GO-002: the bridge is cleared under wv.mu by teardown.

func clearBridge(wv *WebView) {
	wv.mu.Lock()
	wv.viewBridge = nil
	wv.mu.Unlock()
}

func TestGPURecovery_BridgeClearedBeforePulseTaskRuns(t *testing.T) {
	h := newGPURecoveryHarness(t)
	wv, host, _ := newGPURecoveryView(1)
	e := newGPURecoveryEngine(wv)
	h.beforePost = func() { clearBridge(wv) }

	e.runGPURecoveryPass()

	require.Zero(t, host.pulses)
}

func TestGPURecovery_BridgeClearedBeforeVerify(t *testing.T) {
	h := newGPURecoveryHarness(t)
	wv, _, browser := newGPURecoveryView(1)
	e := newGPURecoveryEngine(wv)
	e.runGPURecoveryPass()

	clearBridge(wv)
	h.flush()

	require.Zero(t, browser.reloads)
}

func TestGPURecovery_BridgeClearedConcurrentlyIsRaceFree(t *testing.T) {
	h := newGPURecoveryHarness(t)
	wv, _, _ := newGPURecoveryView(1)
	e := newGPURecoveryEngine(wv)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			e.runGPURecoveryPass()
			h.flush()
		}
	}()
	go func() {
		defer wg.Done()
		clearBridge(wv)
	}()
	wg.Wait()

	// After teardown nothing may touch the bridge again.
	e.runGPURecoveryPass()
	h.flush()
	require.Nil(t, wv.liveBridge())
}

// queuedGTK makes wv dispatch synchronous GTK work onto a queue the test
// drains explicitly, so state can change while a reload is "waiting on GTK".
type queuedGTK struct{ ops chan func() }

func queueGTKDispatch(wv *WebView) *queuedGTK {
	q := &queuedGTK{ops: make(chan func(), 4)}
	wv.gtkSyncIsOwner = func() bool { return false }
	wv.gtkSyncDispatch = func(fn func()) { q.ops <- fn }
	wv.gtkSyncTimeout = time.Minute
	return q
}

// verifyAsync runs the single scheduled verify callback on its own goroutine
// (it blocks inside the synchronous GTK dispatch) and returns once the reload
// operation is queued. The returned func waits for the verify to finish.
func (q *queuedGTK) runQueued(t *testing.T) {
	t.Helper()
	select {
	case fn := <-q.ops:
		fn()
	default:
		t.Fatal("no GTK operation queued")
	}
}

func startVerifyBlockedOnGTK(t *testing.T, h *gpuRecoveryHarness, q *queuedGTK) (wait func()) {
	t.Helper()
	pending := h.pending()
	require.Len(t, pending, 1)
	h.mu.Lock()
	h.scheduled = nil
	h.mu.Unlock()
	done := make(chan struct{})
	go func() {
		defer close(done)
		pending[0].fn()
	}()
	// Wait (without sleeping) until the reload operation is queued on GTK.
	// Fail fast if verify returns without queuing one, instead of hanging.
	select {
	case op := <-q.ops:
		q.ops <- op
	case <-done:
		t.Fatal("verify finished without queuing a GTK reload operation")
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the GTK reload operation")
	}
	return func() {
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for verify to finish")
		}
	}
}

func TestGPURecovery_ReloadSkippedWhenSupersededWhileWaitingOnGTK(t *testing.T) {
	h := newGPURecoveryHarness(t)
	wv, _, browser := newGPURecoveryView(1)
	q := queueGTKDispatch(wv)
	e := newGPURecoveryEngine(wv)
	e.runGPURecoveryPass() // attempt 1 pulses, verify scheduled
	wait := startVerifyBlockedOnGTK(t, h, q)

	wv.claimGPURecovery() // a newer attempt starts while the reload is queued
	q.runQueued(t)        // GTK finally drains the old reload
	wait()

	require.Zero(t, browser.reloads, "reload of a superseded attempt must not navigate")
}

func TestGPURecovery_ReloadSkippedAndDeferredWhenHiddenWhileWaitingOnGTK(t *testing.T) {
	h := newGPURecoveryHarness(t)
	wv, host, browser := newGPURecoveryView(1)
	q := queueGTKDispatch(wv)
	e := newGPURecoveryEngine(wv)
	e.runGPURecoveryPass()
	wait := startVerifyBlockedOnGTK(t, h, q)

	markHidden(wv) // hidden while the reload is queued
	q.runQueued(t)
	wait()

	require.Zero(t, browser.reloads)
	require.True(t, pendingOf(wv), "hidden view must be marked pending, not dropped")

	// Showing it later resumes recovery.
	pulsesBefore := host.pulses
	wv.applyEffectiveVisibility(host, true)
	require.Equal(t, pulsesBefore+2, host.pulses)
}

func TestGPURecovery_ReloadSkippedWhenDestroyedWhileWaitingOnGTK(t *testing.T) {
	h := newGPURecoveryHarness(t)
	wv, _, browser := newGPURecoveryView(1)
	q := queueGTKDispatch(wv)
	e := newGPURecoveryEngine(wv)
	e.runGPURecoveryPass()
	wait := startVerifyBlockedOnGTK(t, h, q)

	wv.destroyed.Store(true)
	q.runQueued(t)
	wait()

	require.Zero(t, browser.reloads)
}

func TestGPURecovery_ReloadProceedsWhenStillValidAfterGTKWait(t *testing.T) {
	h := newGPURecoveryHarness(t)
	wv, _, browser := newGPURecoveryView(1)
	q := queueGTKDispatch(wv)
	e := newGPURecoveryEngine(wv)
	e.runGPURecoveryPass()
	wait := startVerifyBlockedOnGTK(t, h, q)

	q.runQueued(t)
	wait()

	require.Equal(t, 1, browser.reloads)
}

// GO-005: baseline and verify timer are taken when the pulse runs.

func TestGPURecovery_VerifyNotScheduledUntilPulseRuns(t *testing.T) {
	h := newGPURecoveryHarness(t)
	wv, host, _ := newGPURecoveryView(1)
	e := newGPURecoveryEngine(wv)
	h.holdPosts = true

	e.runGPURecoveryPass()
	require.Empty(t, h.pending(), "no verify timer while the pulse task is still queued")
	require.Zero(t, host.pulses)

	h.holdPosts = false
	h.runHeld()
	require.Equal(t, 2, host.pulses)
	require.Len(t, h.pending(), 1)
	require.Equal(t, gpuRecoveryVerifyDelay, h.pending()[0].delay)
}

func TestGPURecovery_BaselineExcludesPaintsBeforePulseRuns(t *testing.T) {
	h := newGPURecoveryHarness(t)
	wv, _, browser := newGPURecoveryView(1)
	e := newGPURecoveryEngine(wv)
	h.setPaints(wv, 10)
	h.holdPosts = true

	e.runGPURecoveryPass()
	h.setPaints(wv, 15) // paints landed while the pulse task was queued
	h.runHeld()         // baseline is captured now: 15
	h.flush()           // no paint since the pulse

	require.Equal(t, 1, browser.reloads, "paints before the pulse must not count as recovery")
}

func TestGPURecovery_NoVerifyForSkippedOrStalePulse(t *testing.T) {
	h := newGPURecoveryHarness(t)
	wv, _, _ := newGPURecoveryView(1)
	e := newGPURecoveryEngine(wv)
	h.holdPosts = true

	e.runGPURecoveryPass()
	markHidden(wv) // hidden before the queued pulse runs: skipped
	h.runHeld()
	require.Empty(t, h.pending(), "skipped pulse schedules no verify")

	wv.mu.Lock()
	wv.effectiveVisible = true
	wv.mu.Unlock()
	e.runGPURecoveryPass() // attempt 3 queued
	wv.claimGPURecovery()  // attempt 4 supersedes it
	h.runHeld()
	require.Empty(t, h.pending(), "stale pulse schedules no verify")
}
