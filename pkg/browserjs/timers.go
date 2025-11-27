package browserjs

import (
	"log"
	"sync"
	"time"

	"github.com/grafana/sobek"
)

// TimerManager manages setTimeout, setInterval, and related timer functions.
type TimerManager struct {
	vm    *sobek.Runtime
	tasks chan func()
	clock Clock
	mu    sync.Mutex

	timers    map[int]Timer
	intervals map[int]Ticker
	nextID    int
}

// NewTimerManager creates a new timer manager.
func NewTimerManager(vm *sobek.Runtime, tasks chan func(), clock Clock) *TimerManager {
	if clock == nil {
		clock = &realClock{}
	}
	return &TimerManager{
		vm:        vm,
		tasks:     tasks,
		clock:     clock,
		timers:    make(map[int]Timer),
		intervals: make(map[int]Ticker),
		nextID:    1,
	}
}

// Install registers timer globals on the VM.
func (tm *TimerManager) Install() error {
	tm.vm.Set("setTimeout", tm.setTimeout)
	tm.vm.Set("clearTimeout", tm.clearTimeout)
	tm.vm.Set("setInterval", tm.setInterval)
	tm.vm.Set("clearInterval", tm.clearInterval)
	tm.vm.Set("queueMicrotask", tm.queueMicrotask)
	return nil
}

// Cleanup stops all active timers and intervals.
func (tm *TimerManager) Cleanup() {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	for _, t := range tm.timers {
		t.Stop()
	}
	for _, t := range tm.intervals {
		t.Stop()
	}
	tm.timers = make(map[int]Timer)
	tm.intervals = make(map[int]Ticker)
}

func (tm *TimerManager) setTimeout(call sobek.FunctionCall) sobek.Value {
	if len(call.Arguments) < 1 {
		return tm.vm.ToValue(0)
	}

	callback, ok := sobek.AssertFunction(call.Arguments[0])
	if !ok {
		return tm.vm.ToValue(0)
	}

	delay := int64(0)
	if len(call.Arguments) > 1 {
		delay = call.Arguments[1].ToInteger()
	}

	log.Printf("[browserjs] DEBUG: setTimeout called with delay=%dms, tasks channel len=%d", delay, len(tm.tasks))

	// For short delays during initialization, run synchronously to avoid deadlock
	// with TLA module evaluation. This is a workaround for Sobek's blocking Evaluate().
	if delay <= 100 && tm.tasks != nil && len(tm.tasks) == 0 {
		// Collect extra arguments
		var args []sobek.Value
		if len(call.Arguments) > 2 {
			args = call.Arguments[2:]
		}

		// Schedule callback via queueMicrotask pattern - run after current call returns
		// but before the next await is processed
		log.Printf("[browserjs] DEBUG: setTimeout short delay - using sync execution")
		tm.vm.Set("__pendingTimeoutCallback", func() {
			_, _ = callback(sobek.Undefined(), args...)
		})
		_, _ = tm.vm.RunString("Promise.resolve().then(() => { if (typeof __pendingTimeoutCallback === 'function') { __pendingTimeoutCallback(); delete __pendingTimeoutCallback; } })")
		return tm.vm.ToValue(0) // Return 0 as timer ID since it runs immediately
	}

	// Collect extra arguments
	var args []sobek.Value
	if len(call.Arguments) > 2 {
		args = call.Arguments[2:]
	}

	tm.mu.Lock()
	id := tm.nextID
	tm.nextID++

	timer := tm.clock.AfterFunc(time.Duration(delay)*time.Millisecond, func() {
		if tm.tasks != nil {
			tm.tasks <- func() {
				tm.mu.Lock()
				delete(tm.timers, id)
				tm.mu.Unlock()
				_, _ = callback(sobek.Undefined(), args...)
			}
		} else {
			// Direct execution if no task queue
			tm.mu.Lock()
			delete(tm.timers, id)
			tm.mu.Unlock()
			_, _ = callback(sobek.Undefined(), args...)
		}
	})
	tm.timers[id] = timer
	tm.mu.Unlock()

	return tm.vm.ToValue(id)
}

func (tm *TimerManager) clearTimeout(call sobek.FunctionCall) sobek.Value {
	if len(call.Arguments) < 1 {
		return sobek.Undefined()
	}

	id := int(call.Arguments[0].ToInteger())
	tm.mu.Lock()
	if timer, ok := tm.timers[id]; ok {
		timer.Stop()
		delete(tm.timers, id)
	}
	tm.mu.Unlock()

	return sobek.Undefined()
}

func (tm *TimerManager) setInterval(call sobek.FunctionCall) sobek.Value {
	if len(call.Arguments) < 1 {
		return tm.vm.ToValue(0)
	}

	callback, ok := sobek.AssertFunction(call.Arguments[0])
	if !ok {
		return tm.vm.ToValue(0)
	}

	delay := int64(0)
	if len(call.Arguments) > 1 {
		delay = call.Arguments[1].ToInteger()
	}
	if delay < 1 {
		delay = 1
	}

	// Collect extra arguments
	var args []sobek.Value
	if len(call.Arguments) > 2 {
		args = call.Arguments[2:]
	}

	tm.mu.Lock()
	id := tm.nextID
	tm.nextID++

	ticker := tm.clock.NewTicker(time.Duration(delay) * time.Millisecond)
	tm.intervals[id] = ticker
	tm.mu.Unlock()

	go func() {
		for range ticker.C() {
			if tm.tasks != nil {
				tm.tasks <- func() {
					_, _ = callback(sobek.Undefined(), args...)
				}
			} else {
				_, _ = callback(sobek.Undefined(), args...)
			}
		}
	}()

	return tm.vm.ToValue(id)
}

func (tm *TimerManager) clearInterval(call sobek.FunctionCall) sobek.Value {
	if len(call.Arguments) < 1 {
		return sobek.Undefined()
	}

	id := int(call.Arguments[0].ToInteger())
	tm.mu.Lock()
	if ticker, ok := tm.intervals[id]; ok {
		ticker.Stop()
		delete(tm.intervals, id)
	}
	tm.mu.Unlock()

	return sobek.Undefined()
}

func (tm *TimerManager) queueMicrotask(call sobek.FunctionCall) sobek.Value {
	if len(call.Arguments) < 1 {
		return sobek.Undefined()
	}

	callback, ok := sobek.AssertFunction(call.Arguments[0])
	if !ok {
		return sobek.Undefined()
	}

	log.Printf("[browserjs] DEBUG: queueMicrotask called, tasks channel len=%d", len(tm.tasks))

	// Queue the microtask to run on next event loop iteration
	if tm.tasks != nil {
		tm.tasks <- func() {
			_, _ = callback(sobek.Undefined())
		}
	} else {
		// Direct execution if no task queue
		_, _ = callback(sobek.Undefined())
	}

	return sobek.Undefined()
}
