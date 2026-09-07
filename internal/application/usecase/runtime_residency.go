package usecase

import "time"

// ResidencyAction is the single decision a RuntimeResidency event produces.
// The UI boundary maps it to scheduler operations: clock and timer handles
// stay outside this controller, which never sleeps.
type ResidencyAction int

const (
	// ResidencyNone requires no scheduler change.
	ResidencyNone ResidencyAction = iota
	// ResidencyArm starts (or restarts) the idle deadline.
	ResidencyArm
	// ResidencyCancel drops a previously armed idle deadline.
	ResidencyCancel
	// ResidencyQuit ends the process through the orderly shutdown path.
	ResidencyQuit
)

// ResidencyDecision is the outcome of one residency event.
type ResidencyDecision struct {
	Action     ResidencyAction
	Deadline   time.Time
	Generation uint64
}

// RuntimeResidency is a testable bounded-residency policy: after the last
// user window closes it arms a deadline instead of quitting immediately, so
// a fast reopen can reuse the retained runtime. It tracks only engine-neutral
// counts; native GTK holds, timers, and quiescence signals live in UI
// adapters. The zero timeout preserves the existing last-window exit.
type RuntimeResidency struct {
	timeout    time.Duration
	windows    int
	activeWork int
	shutdown   bool
	armed      bool
	deadline   time.Time
	generation uint64
}

// NewRuntimeResidency returns a policy with the given idle timeout. Negative
// values clamp to zero (disabled). The accepted configuration range is
// 0..300000ms; enforcement of that range belongs to config validation.
func NewRuntimeResidency(timeout time.Duration) *RuntimeResidency {
	if timeout < 0 {
		timeout = 0
	}
	return &RuntimeResidency{timeout: timeout}
}

// WindowOpened records a user window (including native browser/auth popups
// that still belong to the application) and cancels any armed deadline.
func (r *RuntimeResidency) WindowOpened(_ time.Time) ResidencyDecision {
	r.windows++
	if r.armed {
		return r.cancel()
	}
	return ResidencyDecision{Action: ResidencyNone, Generation: r.generation}
}

// WindowClosed records a user-window close. Duplicate closes are idempotent
// and never drive the count negative.
func (r *RuntimeResidency) WindowClosed(now time.Time) ResidencyDecision {
	if r.windows > 0 {
		r.windows--
	}
	return r.evaluateIdle(now)
}

// OpenFailed records a failed reopen attempt. Idle eligibility is restored
// rather than stranding the policy: a failed open can never leak the
// application hold indefinitely.
func (r *RuntimeResidency) OpenFailed(now time.Time) ResidencyDecision {
	if r.windows > 0 {
		r.windows--
	}
	return r.evaluateIdle(now)
}

// WorkStarted records newly accepted native work. An armed deadline is
// cancelled: expiry must never fire while required work is outstanding.
func (r *RuntimeResidency) WorkStarted(_ time.Time) ResidencyDecision {
	r.activeWork++
	if r.armed {
		return r.cancel()
	}
	return ResidencyDecision{Action: ResidencyNone, Generation: r.generation}
}

// WorkFinished records settled work and re-evaluates idle eligibility.
func (r *RuntimeResidency) WorkFinished(now time.Time) ResidencyDecision {
	if r.activeWork > 0 {
		r.activeWork--
	}
	return r.evaluateIdle(now)
}

// ShutdownRequested records an explicit quit or signal. It always decides
// Quit immediately and never waits for the idle deadline.
func (r *RuntimeResidency) ShutdownRequested() ResidencyDecision {
	r.shutdown = true
	r.armed = false
	return ResidencyDecision{Action: ResidencyQuit, Generation: r.generation}
}

// HandleExpiry processes a fired idle timer. Stale generations (superseded by
// a later arm/cancel) are ignored. Expiry quits only when still quiescent;
// otherwise it stands down and lets the next event re-arm.
func (r *RuntimeResidency) HandleExpiry(_ time.Time, generation uint64) ResidencyDecision {
	if !r.armed || generation != r.generation {
		return ResidencyDecision{Action: ResidencyNone, Generation: r.generation}
	}
	if r.windows > 0 || r.activeWork > 0 || r.shutdown {
		r.armed = false
		if r.shutdown {
			return ResidencyDecision{Action: ResidencyQuit, Generation: r.generation}
		}
		return ResidencyDecision{Action: ResidencyNone, Generation: r.generation}
	}
	r.armed = false
	return ResidencyDecision{Action: ResidencyQuit, Generation: r.generation}
}

// Armed reports whether an idle deadline is currently armed. The UI boundary
// uses it only for diagnostics; scheduling follows decisions.
func (r *RuntimeResidency) Armed() bool { return r.armed }

// Generation returns the current timer generation for expiry correlation.
func (r *RuntimeResidency) Generation() uint64 { return r.generation }

func (r *RuntimeResidency) cancel() ResidencyDecision {
	r.armed = false
	r.generation++
	return ResidencyDecision{Action: ResidencyCancel, Generation: r.generation}
}

func (r *RuntimeResidency) evaluateIdle(now time.Time) ResidencyDecision {
	if r.shutdown {
		r.armed = false
		return ResidencyDecision{Action: ResidencyQuit, Generation: r.generation}
	}
	if r.windows > 0 {
		if r.armed {
			return r.cancel()
		}
		return ResidencyDecision{Action: ResidencyNone, Generation: r.generation}
	}
	if r.timeout <= 0 {
		r.armed = false
		return ResidencyDecision{Action: ResidencyQuit, Generation: r.generation}
	}
	if r.activeWork > 0 {
		return ResidencyDecision{Action: ResidencyNone, Generation: r.generation}
	}
	r.armed = true
	r.deadline = now.Add(r.timeout)
	r.generation++
	return ResidencyDecision{Action: ResidencyArm, Deadline: r.deadline, Generation: r.generation}
}
