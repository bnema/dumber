package syncdispatch

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestRunSynchronousDispatchRunsInlineWhenOwner(t *testing.T) {
	var dispatched atomic.Bool
	var ran atomic.Bool

	result := RunSynchronousDispatch(SyncDispatchOptions{
		Label:   "test.inline",
		Timeout: time.Second,
		IsOwner: func() bool { return true },
		Dispatch: func(func()) {
			dispatched.Store(true)
		},
	}, func() {
		ran.Store(true)
	})

	if result.Status != SyncDispatchInline {
		t.Fatalf("status = %s, want %s", result.Status, SyncDispatchInline)
	}
	if !ran.Load() {
		t.Fatal("callback did not run inline")
	}
	if dispatched.Load() {
		t.Fatal("callback was dispatched despite owner context")
	}
}

func TestRunSynchronousDispatchCompletesWhenDispatchedCallbackRuns(t *testing.T) {
	var ran atomic.Bool

	result := RunSynchronousDispatch(SyncDispatchOptions{
		Label:   "test.dispatch",
		Timeout: time.Second,
		IsOwner: func() bool { return false },
		Dispatch: func(fn func()) {
			go fn()
		},
	}, func() {
		ran.Store(true)
	})

	if result.Status != SyncDispatchCompleted {
		t.Fatalf("status = %s, want %s", result.Status, SyncDispatchCompleted)
	}
	if !result.Completed() {
		t.Fatal("result.Completed() = false, want true")
	}
	if !ran.Load() {
		t.Fatal("callback did not run")
	}
}

func TestRunSynchronousDispatchTimesOutWhenDispatchBlocks(t *testing.T) {
	releaseDispatch := make(chan struct{})
	var ran atomic.Bool

	result := RunSynchronousDispatch(SyncDispatchOptions{
		Label:   "test.blocking_dispatch",
		Timeout: 5 * time.Millisecond,
		IsOwner: func() bool { return false },
		Dispatch: func(fn func()) {
			<-releaseDispatch
			fn()
		},
	}, func() {
		ran.Store(true)
	})

	if result.Status != SyncDispatchTimedOut {
		t.Fatalf("status = %s, want %s", result.Status, SyncDispatchTimedOut)
	}
	if ran.Load() {
		t.Fatal("callback ran while dispatch was blocked")
	}
	close(releaseDispatch)
	time.Sleep(20 * time.Millisecond)
	if ran.Load() {
		t.Fatal("callback ran after a blocking dispatch timed out")
	}
}

func TestRunSynchronousDispatchTimesOutWhenCallbackNeverStarts(t *testing.T) {
	var ran atomic.Bool

	result := RunSynchronousDispatch(SyncDispatchOptions{
		Label:   "test.timeout",
		Timeout: 5 * time.Millisecond,
		IsOwner: func() bool { return false },
		Dispatch: func(func()) {
			// Simulate an event loop that never services the queued callback.
		},
	}, func() {
		ran.Store(true)
	})

	if result.Status != SyncDispatchTimedOut {
		t.Fatalf("status = %s, want %s", result.Status, SyncDispatchTimedOut)
	}
	if result.Completed() {
		t.Fatal("result.Completed() = true, want false")
	}
	if ran.Load() {
		t.Fatal("callback ran despite timeout before dispatch start")
	}
}

func TestRunSynchronousDispatchWaitsForStartedCallbackPastTimeout(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	resultCh := make(chan SyncDispatchResult, 1)
	var ran atomic.Bool

	go func() {
		resultCh <- RunSynchronousDispatch(SyncDispatchOptions{
			Label:   "test.started_late_finish",
			Timeout: 50 * time.Millisecond,
			IsOwner: func() bool { return false },
			Dispatch: func(fn func()) {
				go fn()
			},
		}, func() {
			close(started)
			<-release
			ran.Store(true)
		})
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("callback did not start")
	}
	time.Sleep(100 * time.Millisecond)
	if ran.Load() {
		t.Fatal("callback ran before release")
	}
	close(release)

	select {
	case result := <-resultCh:
		if result.Status != SyncDispatchCompletedAfterTimeout {
			t.Fatalf("status = %s, want %s", result.Status, SyncDispatchCompletedAfterTimeout)
		}
		if !result.Completed() {
			t.Fatal("result.Completed() = false, want true")
		}
	case <-time.After(time.Second):
		t.Fatal("RunSynchronousDispatch did not return after started callback completed")
	}
	if !ran.Load() {
		t.Fatal("callback did not run")
	}
}

func TestRunSynchronousDispatchCancelsCallbackThatAttemptsToStartAfterDeadline(t *testing.T) {
	callbackAttempted := make(chan struct{}, 1)
	var ran atomic.Bool

	result := RunSynchronousDispatch(SyncDispatchOptions{
		Label:   "test.after_deadline",
		Timeout: 5 * time.Millisecond,
		IsOwner: func() bool { return false },
		Dispatch: func(fn func()) {
			go func() {
				time.Sleep(20 * time.Millisecond)
				fn()
				callbackAttempted <- struct{}{}
			}()
		},
	}, func() {
		ran.Store(true)
	})

	if result.Status != SyncDispatchTimedOut {
		t.Fatalf("status = %s, want %s", result.Status, SyncDispatchTimedOut)
	}
	select {
	case <-callbackAttempted:
	case <-time.After(time.Second):
		t.Fatal("callback did not attempt late start")
	}
	if ran.Load() {
		t.Fatal("callback ran after deadline")
	}
}

func TestRunSynchronousDispatchLeavesCallbackQueuedWhenBlockingDispatchTimesOutAndLateStartAllowed(t *testing.T) {
	releaseDispatch := make(chan struct{})
	var ran atomic.Bool

	result := RunSynchronousDispatch(SyncDispatchOptions{
		Label:                      "test.blocking_cleanup_dispatch",
		Timeout:                    5 * time.Millisecond,
		IsOwner:                    func() bool { return false },
		AllowLateStartAfterTimeout: true,
		Dispatch: func(fn func()) {
			<-releaseDispatch
			fn()
		},
	}, func() {
		ran.Store(true)
	})

	if result.Status != SyncDispatchQueuedAfterTimeout {
		t.Fatalf("status = %s, want %s", result.Status, SyncDispatchQueuedAfterTimeout)
	}
	if ran.Load() {
		t.Fatal("callback ran before dispatch was released")
	}
	close(releaseDispatch)
	deadline := time.Now().Add(time.Second)
	for !ran.Load() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !ran.Load() {
		t.Fatal("queued cleanup callback did not run")
	}
}

func TestRunSynchronousDispatchLeavesCallbackQueuedAfterTimeoutWhenConfigured(t *testing.T) {
	delayed := make(chan func(), 1)
	var ran atomic.Bool

	result := RunSynchronousDispatch(SyncDispatchOptions{
		Label:                      "test.late_cleanup",
		Timeout:                    5 * time.Millisecond,
		IsOwner:                    func() bool { return false },
		AllowLateStartAfterTimeout: true,
		Dispatch: func(fn func()) {
			delayed <- fn
		},
	}, func() {
		ran.Store(true)
	})

	if result.Status != SyncDispatchQueuedAfterTimeout {
		t.Fatalf("status = %s, want %s", result.Status, SyncDispatchQueuedAfterTimeout)
	}
	if result.Completed() {
		t.Fatal("result.Completed() = true, want false")
	}
	if ran.Load() {
		t.Fatal("callback ran before late dispatch")
	}

	select {
	case fn := <-delayed:
		fn()
	case <-time.After(time.Second):
		t.Fatal("dispatch callback was not captured")
	}

	if !ran.Load() {
		t.Fatal("queued callback did not run after timeout")
	}
}

func TestRunSynchronousDispatchCancelsCallbackThatStartsAfterTimeout(t *testing.T) {
	delayed := make(chan func(), 1)
	var ran atomic.Bool

	result := RunSynchronousDispatch(SyncDispatchOptions{
		Label:   "test.late",
		Timeout: 5 * time.Millisecond,
		IsOwner: func() bool { return false },
		Dispatch: func(fn func()) {
			delayed <- fn
		},
	}, func() {
		ran.Store(true)
	})

	if result.Status != SyncDispatchTimedOut {
		t.Fatalf("status = %s, want %s", result.Status, SyncDispatchTimedOut)
	}

	select {
	case fn := <-delayed:
		fn()
	case <-time.After(time.Second):
		t.Fatal("dispatch callback was not captured")
	}

	if ran.Load() {
		t.Fatal("late callback ran after timeout")
	}
}
