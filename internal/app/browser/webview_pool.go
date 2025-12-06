// Package browser provides the main browser application components.
package browser

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/dumber/pkg/webkit"
)

// PoolState represents the current state of the WebView pool
type PoolState int32

const (
	PoolStateIdle PoolState = iota
	PoolStateWarming
	PoolStateReady
	PoolStateStopped
)

// WebViewPool manages pre-created WebViews for instant tab/pane creation.
// Uses a "warm pool" strategy: pre-creates fresh WebViews rather than recycling,
// since WebKitGTK doesn't support resetting WebView state.
type WebViewPool struct {
	mu sync.RWMutex

	// Pool of ready-to-use WebViews
	available []*webkit.WebView

	// Configuration
	targetSize  int           // Target number of WebViews to maintain
	maxSize     int           // Maximum pool size
	warmupDelay time.Duration // Delay before initial warmup

	// State tracking
	state        int32 // atomic: PoolState
	created      int64 // atomic: total WebViews created
	acquired     int64 // atomic: total WebViews acquired from pool
	misses       int64 // atomic: times pool was empty when requested
	configGetter func() *webkit.Config

	// Shutdown
	stopCh   chan struct{}
	wg       sync.WaitGroup
	stopOnce sync.Once // Ensures Stop() is idempotent
}

// PoolStats contains statistics about pool usage
type PoolStats struct {
	Available  int
	TargetSize int
	MaxSize    int
	Created    int64
	Acquired   int64
	Misses     int64
	HitRate    float64
	State      PoolState
}

// NewWebViewPool creates a new WebView pool with the given configuration.
// configGetter is called each time a new WebView needs to be created.
func NewWebViewPool(targetSize, maxSize int, configGetter func() *webkit.Config) *WebViewPool {
	if targetSize < 1 {
		targetSize = 1
	}
	if maxSize < targetSize {
		maxSize = targetSize
	}

	return &WebViewPool{
		available:    make([]*webkit.WebView, 0, maxSize),
		targetSize:   targetSize,
		maxSize:      maxSize,
		warmupDelay:  100 * time.Millisecond, // Wait for UI to render first
		configGetter: configGetter,
		stopCh:       make(chan struct{}),
	}
}

// SetWarmupDelay sets the delay before initial pool warmup.
// Must be called before Start().
func (p *WebViewPool) SetWarmupDelay(delay time.Duration) {
	p.warmupDelay = delay
}

// Start begins the pool warmup process in the background.
// Should be called after the main window is visible.
func (p *WebViewPool) Start() {
	if !atomic.CompareAndSwapInt32(&p.state, int32(PoolStateIdle), int32(PoolStateWarming)) {
		logging.Warn("[pool] Pool already started or stopped")
		return
	}

	p.wg.Add(1)
	go p.warmupLoop()

	logging.Info(fmt.Sprintf("[pool] WebView pool started (target=%d, max=%d, delay=%v)",
		p.targetSize, p.maxSize, p.warmupDelay))
}

// warmupLoop runs in background to maintain pool size
func (p *WebViewPool) warmupLoop() {
	defer p.wg.Done()

	// Initial delay to let UI render first
	select {
	case <-time.After(p.warmupDelay):
	case <-p.stopCh:
		return
	}

	// Initial warmup
	p.warmup()
	atomic.StoreInt32(&p.state, int32(PoolStateReady))

	// Periodic check to maintain pool size (every 500ms)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			p.warmup()
		case <-p.stopCh:
			return
		}
	}
}

// warmup creates WebViews to reach target size
func (p *WebViewPool) warmup() {
	p.mu.RLock()
	currentSize := len(p.available)
	p.mu.RUnlock()

	needed := p.targetSize - currentSize
	if needed <= 0 {
		return
	}

	logging.Debug(fmt.Sprintf("[pool] Warming up %d WebViews (current=%d, target=%d)",
		needed, currentSize, p.targetSize))

	for i := 0; i < needed; i++ {
		select {
		case <-p.stopCh:
			return
		default:
		}

		// Create WebView on GTK main thread
		var wv *webkit.WebView
		var err error

		done := make(chan struct{})
		webkit.RunOnMainThread(func() {
			defer close(done)
			cfg := p.configGetter()
			if cfg == nil {
				logging.Error("[pool] Config getter returned nil")
				return
			}
			// Pool WebViews don't create their own window
			cfg.CreateWindow = false
			wv, err = webkit.NewWebView(cfg)
		})
		<-done

		if err != nil {
			logging.Error(fmt.Sprintf("[pool] Failed to create WebView: %v", err))
			continue
		}
		if wv == nil {
			logging.Error("[pool] Created WebView is nil")
			continue
		}

		p.mu.Lock()
		if len(p.available) < p.maxSize {
			p.available = append(p.available, wv)
			atomic.AddInt64(&p.created, 1)
			logging.Debug(fmt.Sprintf("[pool] Added WebView ID %d to pool (size=%d)",
				wv.ID(), len(p.available)))
		} else {
			// Pool full, destroy the extra WebView
			p.mu.Unlock()
			webkit.RunOnMainThread(func() {
				wv.Destroy()
			})
			logging.Debug("[pool] Pool at max capacity, discarding extra WebView")
			continue
		}
		p.mu.Unlock()
	}
}

// Acquire gets a WebView from the pool, or creates one if pool is empty.
// The returned WebView is ready to use but needs to be added to a container.
// Returns nil and error if creation fails.
func (p *WebViewPool) Acquire() (*webkit.WebView, error) {
	state := PoolState(atomic.LoadInt32(&p.state))
	if state == PoolStateStopped {
		return nil, fmt.Errorf("pool is stopped")
	}

	p.mu.Lock()
	if len(p.available) > 0 {
		// Take from pool (LIFO for better cache locality)
		wv := p.available[len(p.available)-1]
		p.available = p.available[:len(p.available)-1]
		remaining := len(p.available) // Capture before unlock to avoid race
		p.mu.Unlock()

		atomic.AddInt64(&p.acquired, 1)
		logging.Debug(fmt.Sprintf("[pool] Acquired WebView ID %d from pool (remaining=%d)",
			wv.ID(), remaining))

		return wv, nil
	}
	p.mu.Unlock()

	// Pool empty, create on-demand (fallback)
	atomic.AddInt64(&p.misses, 1)
	logging.Debug("[pool] Pool empty, creating WebView on-demand")

	var wv *webkit.WebView
	var err error

	done := make(chan struct{})
	webkit.RunOnMainThread(func() {
		defer close(done)
		cfg := p.configGetter()
		if cfg == nil {
			err = fmt.Errorf("config getter returned nil")
			return
		}
		cfg.CreateWindow = false
		wv, err = webkit.NewWebView(cfg)
	})
	<-done

	if err != nil {
		return nil, fmt.Errorf("failed to create WebView: %w", err)
	}

	atomic.AddInt64(&p.created, 1)
	return wv, nil
}

// TryAcquire attempts to get a WebView from the pool without blocking.
// Returns nil if pool is empty (does not create on-demand).
func (p *WebViewPool) TryAcquire() *webkit.WebView {
	state := PoolState(atomic.LoadInt32(&p.state))
	if state == PoolStateStopped {
		return nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.available) > 0 {
		wv := p.available[len(p.available)-1]
		p.available = p.available[:len(p.available)-1]
		atomic.AddInt64(&p.acquired, 1)
		logging.Debug(fmt.Sprintf("[pool] TryAcquire: got WebView ID %d (remaining=%d)",
			wv.ID(), len(p.available)))
		return wv
	}

	atomic.AddInt64(&p.misses, 1)
	return nil
}

// Stats returns current pool statistics
func (p *WebViewPool) Stats() PoolStats {
	p.mu.RLock()
	available := len(p.available)
	p.mu.RUnlock()

	acquired := atomic.LoadInt64(&p.acquired)
	misses := atomic.LoadInt64(&p.misses)

	var hitRate float64
	total := acquired + misses
	if total > 0 {
		hitRate = float64(acquired) / float64(total)
	}

	return PoolStats{
		Available:  available,
		TargetSize: p.targetSize,
		MaxSize:    p.maxSize,
		Created:    atomic.LoadInt64(&p.created),
		Acquired:   acquired,
		Misses:     misses,
		HitRate:    hitRate,
		State:      PoolState(atomic.LoadInt32(&p.state)),
	}
}

// Stop shuts down the pool and destroys all pooled WebViews.
// Safe to call multiple times.
func (p *WebViewPool) Stop() {
	p.stopOnce.Do(func() {
		// Transition to stopped state from any running state
		for {
			oldState := atomic.LoadInt32(&p.state)
			if oldState == int32(PoolStateStopped) || oldState == int32(PoolStateIdle) {
				return
			}
			if atomic.CompareAndSwapInt32(&p.state, oldState, int32(PoolStateStopped)) {
				break
			}
		}

		close(p.stopCh)
		p.wg.Wait()

		// Destroy all pooled WebViews
		p.mu.Lock()
		views := p.available
		p.available = nil
		p.mu.Unlock()

		if len(views) > 0 {
			logging.Info(fmt.Sprintf("[pool] Destroying %d pooled WebViews", len(views)))
			for _, wv := range views {
				if wv != nil && !wv.IsDestroyed() {
					wv.Destroy()
				}
			}
		}

		stats := p.Stats()
		logging.Info(fmt.Sprintf("[pool] Pool stopped (created=%d, acquired=%d, misses=%d, hitRate=%.2f%%)",
			stats.Created, stats.Acquired, stats.Misses, stats.HitRate*100))
	})
}

// IsReady returns true if the pool has completed initial warmup
func (p *WebViewPool) IsReady() bool {
	return PoolState(atomic.LoadInt32(&p.state)) == PoolStateReady
}
