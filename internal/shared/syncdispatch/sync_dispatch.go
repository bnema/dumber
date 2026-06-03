package syncdispatch

import (
	"sync"
	"time"
)

// SyncDispatchStatus describes how a synchronous event-loop dispatch finished.
type SyncDispatchStatus string

const (
	// SyncDispatchInline means the callback ran immediately because the caller
	// already owned the target main context.
	SyncDispatchInline SyncDispatchStatus = "inline"
	// SyncDispatchCompleted means the callback was dispatched and completed before
	// the configured timeout elapsed.
	SyncDispatchCompleted SyncDispatchStatus = "completed"
	// SyncDispatchTimedOut means the callback did not start before the timeout and
	// was canceled before running side effects.
	SyncDispatchTimedOut SyncDispatchStatus = "timed_out"
	// SyncDispatchCompletedAfterTimeout means the callback started before the
	// timeout and completed after the timeout elapsed. Callers must treat it as
	// completed because side effects have run.
	SyncDispatchCompletedAfterTimeout SyncDispatchStatus = "completed_after_timeout"
	// SyncDispatchQueuedAfterTimeout means the callback did not start before the
	// timeout, but the dispatch was intentionally left queued so it may run later.
	SyncDispatchQueuedAfterTimeout SyncDispatchStatus = "queued_after_timeout"
	// SyncDispatchSkipped means required inputs were missing, so no callback ran.
	SyncDispatchSkipped SyncDispatchStatus = "skipped"
)

// SyncDispatchOptions configures RunSynchronousDispatch.
type SyncDispatchOptions struct {
	// Label identifies the caller in diagnostics. The helper does not log by
	// itself, but callers can include this label when reporting the returned
	// result.
	Label string
	// Timeout bounds how long the caller waits for dispatched work. Non-positive
	// durations fall back to DefaultSyncDispatchTimeout.
	Timeout time.Duration
	// IsOwner returns whether the caller already owns the target main context.
	IsOwner func() bool
	// Dispatch schedules a callback on the target main context.
	Dispatch func(func())
	// AllowLateStartAfterTimeout keeps queued work runnable when the timeout fires
	// before Dispatch starts the callback. Use this only for cleanup work where
	// canceling would leak resources; decision callbacks should leave this false.
	AllowLateStartAfterTimeout bool
}

// SyncDispatchResult is returned by RunSynchronousDispatch.
type SyncDispatchResult struct {
	Label   string
	Status  SyncDispatchStatus
	Elapsed time.Duration
}

// Completed reports whether the callback completed before returning.
func (r SyncDispatchResult) Completed() bool {
	return r.Status == SyncDispatchInline || r.Status == SyncDispatchCompleted || r.Status == SyncDispatchCompletedAfterTimeout
}

// DefaultSyncDispatchTimeout is a conservative upper bound for synchronous
// waits on a target event loop. It is long enough for normal idle dispatch but
// short enough to avoid converting a stalled loop into an unbounded caller
// deadlock.
const DefaultSyncDispatchTimeout = 2 * time.Second

const (
	dispatchStateInit int32 = iota
	dispatchStateStarted
	dispatchStateCompleted
	dispatchStateTimedOut
)

// RunSynchronousDispatch executes fn inline when the target main context is
// already owned by the caller; otherwise it schedules fn with opts.Dispatch and
// waits for completion up to opts.Timeout.
//
// If the timeout fires before the dispatched callback starts, the default
// behavior is to skip the callback when the main loop eventually services it. If
// AllowLateStartAfterTimeout is true, queued work is left runnable and the
// result reports queued_after_timeout instead. If the callback has already
// started, Go cannot preempt it safely; this helper waits for it to finish and
// reports completed_after_timeout so callers do not fail closed while side
// effects are still possible.
func RunSynchronousDispatch(opts SyncDispatchOptions, fn func()) SyncDispatchResult {
	start := time.Now()
	result := SyncDispatchResult{Label: opts.Label, Status: SyncDispatchSkipped}
	if fn == nil {
		result.Elapsed = time.Since(start)
		return result
	}
	if opts.IsOwner != nil && opts.IsOwner() {
		fn()
		result.Status = SyncDispatchInline
		result.Elapsed = time.Since(start)
		return result
	}
	if opts.Dispatch == nil {
		result.Elapsed = time.Since(start)
		return result
	}

	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = DefaultSyncDispatchTimeout
	}

	deadline := start.Add(timeout)
	done := make(chan struct{})
	state := dispatchStateInit
	var stateMu sync.Mutex
	loadState := func() int32 {
		stateMu.Lock()
		defer stateMu.Unlock()
		return state
	}
	markCallbackStarted := func() bool {
		stateMu.Lock()
		defer stateMu.Unlock()
		if state != dispatchStateInit {
			return false
		}
		// Check deadline and transition to dispatchStateStarted under one lock so
		// the timer cannot mark dispatchStateTimedOut after this callback is allowed
		// to start. Late-start cleanup opts out through AllowLateStartAfterTimeout.
		if !opts.AllowLateStartAfterTimeout && time.Now().After(deadline) {
			state = dispatchStateTimedOut
			return false
		}
		state = dispatchStateStarted
		return true
	}
	markCallbackCompleted := func() {
		stateMu.Lock()
		defer stateMu.Unlock()
		state = dispatchStateCompleted
	}
	go func() {
		opts.Dispatch(func() {
			if !markCallbackStarted() {
				close(done)
				return
			}
			defer close(done)
			fn()
			markCallbackCompleted()
		})
	}()

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-done:
		if loadState() == dispatchStateTimedOut {
			result.Status = SyncDispatchTimedOut
		} else {
			result.Status = SyncDispatchCompleted
		}
	case <-timer.C:
		stateMu.Lock()
		currentState := state
		if opts.AllowLateStartAfterTimeout && currentState == dispatchStateInit {
			// Cleanup work may remain queued after timer expiry; decision callbacks
			// instead fall through and mark dispatchStateTimedOut while still unstarted.
			stateMu.Unlock()
			result.Status = SyncDispatchQueuedAfterTimeout
			result.Elapsed = time.Since(start)
			return result
		}
		if currentState == dispatchStateInit {
			state = dispatchStateTimedOut
			stateMu.Unlock()
			result.Status = SyncDispatchTimedOut
			result.Elapsed = time.Since(start)
			return result
		}
		stateMu.Unlock()
		// dispatchStateStarted means the worker already owns fn side effects; wait on
		// done so the caller reports completed-after-timeout only after completion.
		<-done
		if loadState() == dispatchStateTimedOut {
			result.Status = SyncDispatchTimedOut
		} else {
			result.Status = SyncDispatchCompletedAfterTimeout
		}
	}
	result.Elapsed = time.Since(start)
	return result
}
