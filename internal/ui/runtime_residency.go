package ui

import (
	"time"

	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/puregotk/v4/glib"
)

// ResidencyTimeoutFromMillis resolves the effective bounded-residency idle
// timeout for CEF from the startup configuration value in milliseconds.
// Values outside 0..300000 are rejected by config validation; this resolver
// additionally clamps defensively so an unvalidated caller can never arm an
// unbounded or negative deadline.
func ResidencyTimeoutFromMillis(timeoutMs int) time.Duration {
	if timeoutMs <= 0 {
		return 0
	}
	if timeoutMs > 300000 {
		timeoutMs = 300000
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

	timerCallback *glib.SourceFunc
	timerTag      uint
}

// NewResidencyController builds the UI-side residency owner. A zero timeout
// keeps the controller disabled: decisions then always resolve to the
// pre-existing last-window exit behavior.
func NewResidencyController(timeout time.Duration, clock func() time.Time, schedule func(time.Duration, uint64), cancel func(), quit func()) *ResidencyController {
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
// cancelled, the policy resolves Quit immediately, and the deadline is never
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
		c.doQuit()
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
// cancelled, and stale generations are rejected by the policy on fire.
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
