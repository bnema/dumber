package process

import (
	"context"
	"sync"
	"time"
)

// AdmissionGate is a narrow thread-safe admission/work-lease boundary shared
// by the relay listener and the shutdown/residency policy. A lease must be
// acquired before sending an acknowledgement; shutdown atomically stops
// admission while admitted work drains. It never blocks the holder: waiting
// uses WaitDrained with a caller-provided bounded context.
type AdmissionGate struct {
	mu        sync.Mutex
	closed    bool
	active    int
	onDrained []func()
}

// NewAdmissionGate returns an open gate with no active leases.
func NewAdmissionGate() *AdmissionGate {
	return &AdmissionGate{}
}

// Acquire takes one work lease. It reports false when admission is closed;
// the caller must then send the existing error response before any
// acknowledgement and must not dispatch.
func (g *AdmissionGate) Acquire() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed {
		return false
	}
	g.active++
	return true
}

// Release returns one work lease. The final release notifies drain
// subscribers.
func (g *AdmissionGate) Release() {
	g.mu.Lock()
	var notify []func()
	if g.active > 0 {
		g.active--
	}
	if g.active == 0 {
		notify = append([]func(){}, g.onDrained...)
	}
	g.mu.Unlock()
	for _, fn := range notify {
		fn()
	}
}

// Close atomically stops admission. In-flight leases are unaffected; they
// drain through Release.
func (g *AdmissionGate) Close() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.closed = true
}

// Closed reports whether admission is stopped.
func (g *AdmissionGate) Closed() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.closed
}

// Active reports the number of held leases.
func (g *AdmissionGate) Active() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.active
}

// OnDrained registers a callback invoked (without holding the gate lock)
// every time the active count reaches zero. Registration is not idempotent:
// each call appends. There is no unsubscribe; gates are process-lifetime
// owners. Callbacks must not call back into the gate.
func (g *AdmissionGate) OnDrained(fn func()) {
	if fn == nil {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.onDrained = append(g.onDrained, fn)
}

// WaitDrained blocks until no leases are held or ctx ends. It reports false
// on context expiry so shutdown never waits forever. The short poll runs
// only on shutdown/drain paths, never on any frame or event loop.
func (g *AdmissionGate) WaitDrained(ctx context.Context) bool {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		g.mu.Lock()
		drained := g.active == 0
		g.mu.Unlock()
		if drained {
			return true
		}
		select {
		case <-ctx.Done():
			return false
		case <-ticker.C:
		}
	}
}
