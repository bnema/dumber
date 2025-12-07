// Package browser provides the main browser application components.
package browser

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bnema/dumber/internal/logging"
)

// DeferredState represents the state of a deferred initialization
type DeferredState int32

const (
	DeferredStatePending DeferredState = iota
	DeferredStateInitializing
	DeferredStateReady
	DeferredStateFailed
)

// DeferredInit manages lazy initialization of optional services.
// Services are only initialized when first accessed, reducing boot time.
type DeferredInit[T any] struct {
	name     string
	initFunc func(ctx context.Context) (T, error)
	timeout  time.Duration

	mu       sync.RWMutex
	state    int32 // atomic: DeferredState
	value    T
	err      error
	initOnce sync.Once
}

// NewDeferredInit creates a new deferred initializer.
// name: identifier for logging
// initFunc: function to create the value (called at most once)
// timeout: maximum time to wait for initialization (0 = no timeout)
func NewDeferredInit[T any](name string, initFunc func(ctx context.Context) (T, error), timeout time.Duration) *DeferredInit[T] {
	return &DeferredInit[T]{
		name:     name,
		initFunc: initFunc,
		timeout:  timeout,
	}
}

// Get returns the initialized value, initializing it on first call.
// Subsequent calls return the cached value or error.
// Thread-safe: multiple goroutines can call Get() simultaneously.
func (d *DeferredInit[T]) Get(ctx context.Context) (T, error) {
	// Fast path: already initialized
	state := DeferredState(atomic.LoadInt32(&d.state))
	if state == DeferredStateReady {
		d.mu.RLock()
		v := d.value
		d.mu.RUnlock()
		return v, nil
	}
	if state == DeferredStateFailed {
		d.mu.RLock()
		err := d.err
		d.mu.RUnlock()
		var zero T
		return zero, err
	}

	// Slow path: need to initialize
	d.initOnce.Do(func() {
		d.initialize(ctx)
	})

	// Return result
	d.mu.RLock()
	defer d.mu.RUnlock()

	if d.err != nil {
		var zero T
		return zero, d.err
	}
	return d.value, nil
}

// initialize performs the actual initialization
func (d *DeferredInit[T]) initialize(ctx context.Context) {
	atomic.StoreInt32(&d.state, int32(DeferredStateInitializing))
	startTime := time.Now()

	logging.Debug(fmt.Sprintf("[deferred] Initializing %s...", d.name))

	// Apply timeout if configured
	if d.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, d.timeout)
		defer cancel()
	}

	value, err := d.initFunc(ctx)

	d.mu.Lock()
	defer d.mu.Unlock()

	elapsed := time.Since(startTime)

	if err != nil {
		d.err = fmt.Errorf("deferred init %s failed: %w", d.name, err)
		atomic.StoreInt32(&d.state, int32(DeferredStateFailed))
		logging.Error(fmt.Sprintf("[deferred] %s initialization failed after %v: %v", d.name, elapsed, err))
		return
	}

	d.value = value
	atomic.StoreInt32(&d.state, int32(DeferredStateReady))
	logging.Info(fmt.Sprintf("[deferred] %s initialized in %v", d.name, elapsed))
}

// TryGet returns the value if already initialized, without blocking.
// Returns (value, true) if ready, or (zero, false) if not yet initialized.
func (d *DeferredInit[T]) TryGet() (T, bool) {
	state := DeferredState(atomic.LoadInt32(&d.state))
	if state != DeferredStateReady {
		var zero T
		return zero, false
	}

	d.mu.RLock()
	v := d.value
	d.mu.RUnlock()
	return v, true
}

// IsReady returns true if initialization has completed successfully
func (d *DeferredInit[T]) IsReady() bool {
	return DeferredState(atomic.LoadInt32(&d.state)) == DeferredStateReady
}

// IsFailed returns true if initialization failed
func (d *DeferredInit[T]) IsFailed() bool {
	return DeferredState(atomic.LoadInt32(&d.state)) == DeferredStateFailed
}

// State returns the current initialization state
func (d *DeferredInit[T]) State() DeferredState {
	return DeferredState(atomic.LoadInt32(&d.state))
}

// Name returns the identifier of this deferred initializer
func (d *DeferredInit[T]) Name() string {
	return d.name
}

// Error returns the initialization error, if any
func (d *DeferredInit[T]) Error() error {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.err
}

// DeferredInitGroup manages a collection of deferred initializers
type DeferredInitGroup struct {
	mu    sync.RWMutex
	items map[string]interface{ State() DeferredState }
}

// NewDeferredInitGroup creates a new group for tracking deferred initializers
func NewDeferredInitGroup() *DeferredInitGroup {
	return &DeferredInitGroup{
		items: make(map[string]interface{ State() DeferredState }),
	}
}

// Register adds a deferred initializer to the group
func (g *DeferredInitGroup) Register(name string, item interface{ State() DeferredState }) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.items[name] = item
}

// Stats returns statistics about all registered initializers
func (g *DeferredInitGroup) Stats() map[string]DeferredState {
	g.mu.RLock()
	defer g.mu.RUnlock()

	stats := make(map[string]DeferredState, len(g.items))
	for name, item := range g.items {
		stats[name] = item.State()
	}
	return stats
}

// AllReady returns true if all registered initializers are ready
func (g *DeferredInitGroup) AllReady() bool {
	g.mu.RLock()
	defer g.mu.RUnlock()

	for _, item := range g.items {
		if item.State() != DeferredStateReady {
			return false
		}
	}
	return true
}
