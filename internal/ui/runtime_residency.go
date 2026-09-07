package ui

import (
	"time"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/infrastructure/process"
	"github.com/bnema/puregotk/v4/glib"
)

// ResidencyTimeoutMaxMs bounds the startup-only idle timeout in milliseconds.
const ResidencyTimeoutMaxMs = 300000

// ResidencyTimeoutFromMillis resolves the effective bounded-residency idle
// timeout for CEF from the startup configuration value in milliseconds.
// Values outside 0..300000 are rejected by config validation; this resolver
// additionally clamps defensively so an unvalidated caller can never arm an
// unbounded or negative deadline.
func ResidencyTimeoutFromMillis(timeoutMs int) time.Duration {
	if timeoutMs <= 0 {
		return 0
	}
	if timeoutMs > ResidencyTimeoutMaxMs {
		timeoutMs = ResidencyTimeoutMaxMs
	}
	return time.Duration(timeoutMs) * time.Millisecond
}

// SetResidencyTimeout latches the effective idle timeout once at process
// startup. Call it before App.Run; later config reloads must use
// ResidencyTimeoutRestartRequired instead of calling this again.
func (a *App) SetResidencyTimeout(timeout time.Duration) {
	if a == nil {
		return
	}
	a.residencyTimeout = timeout
}

// LatchedResidencyTimeout returns the startup-latched idle timeout.
func (a *App) LatchedResidencyTimeout() time.Duration {
	if a == nil {
		return 0
	}
	return a.residencyTimeout
}

// ResidencyTimeoutRestartRequired reports whether a reloaded configuration
// value differs from the startup latch. A true result means the process must
// restart to apply it; residency is never partially hot-applied.
func (a *App) ResidencyTimeoutRestartRequired(timeoutMs int) bool {
	if a == nil {
		return false
	}
	return ResidencyTimeoutFromMillis(timeoutMs) != a.residencyTimeout
}

// residencyApplication is the application-lifetime subset used for bounded
// residency: one hold while opt-in residency is enabled, released exactly
// once on every Run exit. *gtk.Application satisfies it through embedding.
type residencyApplication interface {
	Hold()
	Release()
	Quit()
}

// ResidencyController owns one application hold and the idle timer for
// bounded opt-in residency. It maps engine-neutral policy decisions to
// scheduler operations; the clock and timer handles are injected so tests
// use fakes while production uses the GLib main context. It never sleeps.
type ResidencyController struct {
	policy   *usecase.RuntimeResidency
	clock    func() time.Time
	schedule func(d time.Duration, generation uint64)
	cancel   func()
	quit     func()

	enabled     bool
	lastWindows int
	quitting    bool
	held        bool

	// Native quiescence inputs. Sources report busy/clear transitions;
	// expiry commits to quit only after admitted work drains and every
	// source settles. All methods must run on the GTK thread; cross-thread
	// settle callbacks arrive through dispatchToGTK.
	nativeBusy    map[string]bool
	pendingQuit   bool
	quiescent     func() bool
	dispatchToGTK func(func())

	timerCallback *glib.SourceFunc
	timerTag      uint
}

// NewResidencyController builds the UI-side residency owner. A zero timeout
// keeps the controller disabled: decisions then always resolve to the
// pre-existing last-window exit behavior.
func NewResidencyController(
	timeout time.Duration,
	clock func() time.Time,
	schedule func(time.Duration, uint64),
	cancel, quit func(),
) *ResidencyController {
	if clock == nil {
		clock = time.Now
	}
	return &ResidencyController{
		policy:   usecase.NewRuntimeResidency(timeout),
		enabled:  timeout > 0,
		clock:    clock,
		schedule: schedule,
		cancel:   cancel,
		quit:     quit,
	}
}

// Enabled reports whether opt-in residency is active (nonzero latch).
// A disabled controller resolves every empty state to the pre-existing
// last-window exit behavior.
func (c *ResidencyController) Enabled() bool {
	return c != nil && c.enabled
}

// AcquireHold takes exactly one application hold for enabled residency.
// It is a no-op when disabled, already held, or given a nil application.
// Pair every acquisition with ReleaseHold, typically via defer in Run so
// all exits (including activation failure) match it exactly once.
func (c *ResidencyController) AcquireHold(app residencyApplication) {
	if c == nil || !c.enabled || c.held || app == nil {
		return
	}
	app.Hold()
	c.held = true
}

// ReleaseHold releases a previously acquired hold exactly once.
func (c *ResidencyController) ReleaseHold(app residencyApplication) {
	if c == nil || !c.held || app == nil {
		return
	}
	c.held = false
	app.Release()
}

// SetWindowCount converges the policy to the current user-window count.
// Opens and closes funnel through here so duplicate or reordered events
// cannot strand the count. It returns the final decision for this update.
func (c *ResidencyController) SetWindowCount(count int) usecase.ResidencyDecision {
	if c == nil {
		return usecase.ResidencyDecision{Action: usecase.ResidencyQuit}
	}
	now := c.clock()
	var decision usecase.ResidencyDecision
	events := false
	for c.lastWindows < count {
		c.lastWindows++
		events = true
		decision = c.apply(c.policy.WindowOpened(now))
	}
	for c.lastWindows > count {
		c.lastWindows--
		events = true
		decision = c.apply(c.policy.WindowClosed(now))
	}
	if count == 0 && !events {
		// No windows were ever reported (e.g. activation failure):
		// evaluate once so the process reaches bounded idle or orderly
		// shutdown instead of idling forever on the residency hold.
		decision = c.apply(c.policy.WindowClosed(now))
	}
	return decision
}

// Evaluate forces one idle evaluation for paths that never report windows
// (e.g. initial-shell failure). It returns the resulting decision.
func (c *ResidencyController) Evaluate() usecase.ResidencyDecision {
	if c == nil {
		return usecase.ResidencyDecision{Action: usecase.ResidencyQuit}
	}
	return c.apply(c.policy.WindowClosed(c.clock()))
}

// NoteExplicitQuit records an explicit quit or signal: the timer is
// canceled, the policy resolves Quit immediately, and the deadline is never
// waited on. It never invokes the quit function itself.
func (c *ResidencyController) NoteExplicitQuit() {
	if c == nil {
		return
	}
	if c.cancel != nil {
		c.cancel()
	}
	c.policy.ShutdownRequested()
}

// OnTimerFired processes a fired idle timer for the given generation.
// Stale generations are ignored by the policy.
func (c *ResidencyController) OnTimerFired(generation uint64) {
	if c == nil {
		return
	}
	c.apply(c.policy.HandleExpiry(c.clock(), generation))
}

func (c *ResidencyController) apply(decision usecase.ResidencyDecision) usecase.ResidencyDecision {
	switch decision.Action {
	case usecase.ResidencyArm:
		if c.cancel != nil {
			c.cancel()
		}
		if c.schedule != nil {
			delay := time.Until(decision.Deadline)
			if delay < 0 {
				delay = 0
			}
			c.schedule(delay, decision.Generation)
		}
	case usecase.ResidencyCancel:
		if c.cancel != nil {
			c.cancel()
		}
	case usecase.ResidencyQuit:
		c.requestQuit()
	}
	return decision
}

// requestQuit commits to quit only when every native source has settled.
// Otherwise it defers: the next settling source rechecks via MaybeQuit.
// Disabled residency quits unconditionally, preserving pre-existing behavior.
func (c *ResidencyController) requestQuit() {
	if c.enabled && c.quiescent != nil && !c.quiescent() {
		c.pendingQuit = true
		return
	}
	c.pendingQuit = false
	c.doQuit()
}

// MaybeQuit rechecks a deferred quit after sources settle. It is invoked
// from settle callbacks (already dispatched to GTK) and busy transitions.
func (c *ResidencyController) MaybeQuit() {
	if c == nil || !c.pendingQuit || c.quitting {
		return
	}
	if c.quiescent != nil && !c.quiescent() {
		return
	}
	c.pendingQuit = false
	c.doQuit()
}

// SetQuiescentFunc installs the composed quiescence predicate consulted
// before an expiry commits to quit. A nil predicate means always settled.
func (c *ResidencyController) SetQuiescentFunc(fn func() bool) {
	if c == nil {
		return
	}
	c.quiescent = fn
}

// SetDispatchToGTK installs the cross-thread dispatch used by settle
// callbacks from CEF, relay, and persistence threads. A nil dispatch
// invokes callbacks directly (tests only).
func (c *ResidencyController) SetDispatchToGTK(fn func(func())) {
	if c == nil {
		return
	}
	c.dispatchToGTK = fn
}

// DispatchToGTK runs fn on the GTK thread through the installed dispatch,
// or directly when none is installed. Settle callbacks must use this.
func (c *ResidencyController) DispatchToGTK(fn func()) {
	if c == nil || fn == nil {
		return
	}
	if c.dispatchToGTK != nil {
		c.dispatchToGTK(fn)
		return
	}
	fn()
}

// SetNativeBusy records one native source busy (true) or settled (false).
// Transitions drive policy work events so expiry never fires while required
// work is outstanding; settling also rechecks a deferred quit.
func (c *ResidencyController) SetNativeBusy(source string, busy bool) usecase.ResidencyDecision {
	if c == nil {
		return usecase.ResidencyDecision{Action: usecase.ResidencyNone}
	}
	if c.nativeBusy == nil {
		c.nativeBusy = make(map[string]bool)
	}
	if c.nativeBusy[source] == busy {
		return usecase.ResidencyDecision{Action: usecase.ResidencyNone}
	}
	c.nativeBusy[source] = busy
	var decision usecase.ResidencyDecision
	if busy {
		decision = c.apply(c.policy.WorkStarted(c.clock()))
	} else {
		decision = c.apply(c.policy.WorkFinished(c.clock()))
		c.MaybeQuit()
	}
	return decision
}

func (c *ResidencyController) doQuit() {
	if c.quitting {
		return
	}
	c.quitting = true
	if c.cancel != nil {
		c.cancel()
	}
	if c.quit != nil {
		c.quit()
	}
}

// scheduleGLibTimeout arms a one-shot GLib timeout for the idle deadline.
// The callback is retained on the controller until it fires or is
// canceled, and stale generations are rejected by the policy on fire.
func (c *ResidencyController) scheduleGLibTimeout(d time.Duration, generation uint64) {
	c.cancelGLibTimeout()
	ms := uint(d.Milliseconds())
	if ms == 0 {
		ms = 1
	}
	callback := glib.SourceFunc(func(_ uintptr) bool {
		c.OnTimerFired(generation)
		return false
	})
	c.timerCallback = &callback
	c.timerTag = glib.TimeoutAdd(ms, &callback, 0)
}

// cancelGLibTimeout drops a pending idle timer and releases its callback.
func (c *ResidencyController) cancelGLibTimeout() {
	if c.timerTag != 0 {
		glib.SourceRemove(c.timerTag)
		c.timerTag = 0
	}
	c.timerCallback = nil
}

// LatchResidencyTimeoutForApp latches the bounded-residency timeout once at
// startup. Only CEF uses it, and only when nonzero; later config changes
// are restart-required instead of partially hot-applied.
func LatchResidencyTimeoutForApp(app *App, timeoutMs int, isCEF bool) {
	if app == nil || !isCEF {
		return
	}
	app.SetResidencyTimeout(ResidencyTimeoutFromMillis(timeoutMs))
}

// residencyShouldQuitAfterLastWindow reports whether the app must quit now
// that no user windows remain. A nil controller (before Run) preserves
// unconditional Quit.
func (a *App) residencyShouldQuitAfterLastWindow() bool {
	if a == nil || a.residency == nil {
		return true
	}
	return a.residency.SetWindowCount(len(a.browserWindows)).Action == usecase.ResidencyQuit
}

// residencyNoteWindowsChanged converges residency accounting to the current
// window count after opens, closes, and activation outcomes.
func (a *App) residencyNoteWindowsChanged() usecase.ResidencyDecision {
	if a == nil || a.residency == nil {
		return usecase.ResidencyDecision{Action: usecase.ResidencyQuit}
	}
	return a.residency.SetWindowCount(len(a.browserWindows))
}

// relayAdmissionProvider exposes a relay admission boundary without
// extending the port interface. It matches desktop's provider structurally.
type relayAdmissionProvider interface {
	AdmissionGate() *process.AdmissionGate
}

// residencyQuiescent composes every native quiescence input: CEF runtime
// activity, snapshot persistence drain, and relay admission leases.
func (a *App) residencyQuiescent() bool {
	if a == nil {
		return true
	}
	if eng, ok := a.engine.(interface{ RuntimeActivity() port.RuntimeActivity }); ok && eng != nil {
		if act := eng.RuntimeActivity(); act != nil && !act.Snapshot().Quiescent() {
			return false
		}
	}
	if drain, ok := a.snapshotService.(port.PersistenceDrain); ok && drain != nil && drain.Active() {
		return false
	}
	if a.deps != nil {
		if provider, ok := a.deps.BrowserLaunchRelay.(relayAdmissionProvider); ok && provider != nil {
			if gate := provider.AdmissionGate(); gate != nil && gate.Active() > 0 {
				return false
			}
		}
	}
	return true
}

// subscribeResidencyQuiescence wires native settle callbacks into the
// residency controller. CEF activity transitions drive policy work events;
// persistence and relay settle paths recheck a deferred quit. Every
// callback crosses into GTK through the controller dispatch: no GTK work
// runs on CEF callbacks and no per-frame polling observes quiescence.
func (a *App) subscribeResidencyQuiescence() {
	r := a.residency
	if a == nil || r == nil || !r.Enabled() {
		return
	}
	dispatch := func(fn func()) { r.DispatchToGTK(fn) }
	if eng, ok := a.engine.(interface{ RuntimeActivity() port.RuntimeActivity }); ok && eng != nil {
		if act := eng.RuntimeActivity(); act != nil {
			unsubscribe := act.Subscribe(func(snapshot port.RuntimeActivitySnapshot) {
				busy := !snapshot.Quiescent()
				dispatch(func() { r.SetNativeBusy("cef-activity", busy) })
			})
			a.residencyUnsubscribe = append(a.residencyUnsubscribe, unsubscribe)
		}
	}
	if drain, ok := a.snapshotService.(port.PersistenceDrain); ok && drain != nil {
		if notifier, ok := drain.(interface{ OnSettled(func()) }); ok {
			notifier.OnSettled(func() { dispatch(func() { r.MaybeQuit() }) })
		}
	}
	if a.deps != nil {
		if provider, ok := a.deps.BrowserLaunchRelay.(relayAdmissionProvider); ok && provider != nil {
			if gate := provider.AdmissionGate(); gate != nil {
				gate.OnDrained(func() { dispatch(func() { r.MaybeQuit() }) })
			}
		}
	}
}

// unsubscribeResidencyQuiescence drops native subscriptions at shutdown.
func (a *App) unsubscribeResidencyQuiescence() {
	if a == nil {
		return
	}
	for _, unsubscribe := range a.residencyUnsubscribe {
		if unsubscribe != nil {
			unsubscribe()
		}
	}
	a.residencyUnsubscribe = nil
}

// closeRelayAdmission stops relay admission before snapshot/update
// teardown so no new work starts while persistence drains.
func (a *App) closeRelayAdmission() {
	if a == nil || a.deps == nil {
		return
	}
	if provider, ok := a.deps.BrowserLaunchRelay.(relayAdmissionProvider); ok && provider != nil {
		if gate := provider.AdmissionGate(); gate != nil {
			gate.Close()
		}
	}
}
