package browserjs

import (
	"sync"
	"testing"
	"time"

	"github.com/grafana/sobek"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockClock implements Clock for testing with controllable time.
type mockClock struct {
	mu      sync.Mutex
	now     time.Time
	timers  []*mockTimer
	tickers []*mockTicker
}

func newMockClock() *mockClock {
	return &mockClock{now: time.Now()}
}

func (c *mockClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *mockClock) AfterFunc(d time.Duration, f func()) Timer {
	c.mu.Lock()
	defer c.mu.Unlock()
	t := &mockTimer{
		deadline: c.now.Add(d),
		f:        f,
	}
	c.timers = append(c.timers, t)
	return t
}

func (c *mockClock) NewTicker(d time.Duration) Ticker {
	c.mu.Lock()
	defer c.mu.Unlock()
	t := &mockTicker{
		interval: d,
		ch:       make(chan time.Time, 1),
	}
	c.tickers = append(c.tickers, t)
	return t
}

// Advance moves time forward and fires any due timers.
func (c *mockClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	var toFire []func()
	var remaining []*mockTimer
	for _, t := range c.timers {
		if !t.stopped && !c.now.Before(t.deadline) {
			toFire = append(toFire, t.f)
		} else if !t.stopped {
			remaining = append(remaining, t)
		}
	}
	c.timers = remaining
	c.mu.Unlock()

	// Fire callbacks outside lock
	for _, f := range toFire {
		f()
	}
}

type mockTimer struct {
	deadline time.Time
	f        func()
	stopped  bool
}

func (t *mockTimer) Stop() bool {
	wasRunning := !t.stopped
	t.stopped = true
	return wasRunning
}

type mockTicker struct {
	interval time.Duration
	ch       chan time.Time
	stopped  bool
}

func (t *mockTicker) Stop() {
	t.stopped = true
	close(t.ch)
}

func (t *mockTicker) C() <-chan time.Time {
	return t.ch
}

func TestTimerManager_SetTimeout(t *testing.T) {
	vm := sobek.New()
	clock := newMockClock()
	tasks := make(chan func(), 10)

	tm := NewTimerManager(vm, tasks, clock)
	require.NoError(t, tm.Install())

	// Track if callback was called
	var called bool
	vm.Set("markCalled", func() {
		called = true
	})

	// Run setTimeout from JS
	_, err := vm.RunString(`
		var id = setTimeout(function() {
			markCalled();
		}, 100);
	`)
	require.NoError(t, err)

	// Should not be called yet
	assert.False(t, called, "callback should not be called immediately")

	// Advance time past the timeout
	clock.Advance(150 * time.Millisecond)

	// Process task queue
	select {
	case task := <-tasks:
		task()
	default:
		t.Fatal("expected task in queue")
	}

	assert.True(t, called, "callback should be called after timeout")
}

func TestTimerManager_ClearTimeout(t *testing.T) {
	vm := sobek.New()
	clock := newMockClock()
	tasks := make(chan func(), 10)

	tm := NewTimerManager(vm, tasks, clock)
	require.NoError(t, tm.Install())

	var called bool
	vm.Set("markCalled", func() {
		called = true
	})

	// Set and immediately clear timeout
	_, err := vm.RunString(`
		var id = setTimeout(function() {
			markCalled();
		}, 100);
		clearTimeout(id);
	`)
	require.NoError(t, err)

	// Advance time past the timeout
	clock.Advance(150 * time.Millisecond)

	// No task should be queued
	select {
	case <-tasks:
		t.Fatal("no task should be queued after clearTimeout")
	default:
		// Expected
	}

	assert.False(t, called, "callback should not be called after clearTimeout")
}

func TestTimerManager_QueueMicrotask(t *testing.T) {
	vm := sobek.New()
	tasks := make(chan func(), 10)

	tm := NewTimerManager(vm, tasks, nil)
	require.NoError(t, tm.Install())

	var called bool
	vm.Set("markCalled", func() {
		called = true
	})

	_, err := vm.RunString(`
		queueMicrotask(function() {
			markCalled();
		});
	`)
	require.NoError(t, err)

	// Task should be queued immediately
	select {
	case task := <-tasks:
		task()
	default:
		t.Fatal("expected task in queue")
	}

	assert.True(t, called, "microtask callback should be called")
}

func TestTimerManager_Cleanup(t *testing.T) {
	vm := sobek.New()
	clock := newMockClock()
	tasks := make(chan func(), 10)

	tm := NewTimerManager(vm, tasks, clock)
	require.NoError(t, tm.Install())

	// Create a timer
	_, err := vm.RunString(`setTimeout(function() {}, 1000);`)
	require.NoError(t, err)

	assert.Len(t, tm.timers, 1, "should have one timer")

	tm.Cleanup()

	assert.Len(t, tm.timers, 0, "timers should be cleared after cleanup")
}
