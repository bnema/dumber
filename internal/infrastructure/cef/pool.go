package cef

import (
	"context"
	"errors"
	"sync"

	"github.com/bnema/dumber/internal/application/port"
)

// Compile-time interface check.
var _ port.WebViewPool = (*WebViewPool)(nil)

// errPoolClosed is returned when an operation is attempted on a closed pool.
var errPoolClosed = errors.New("cef: webview pool is closed")

// WebViewPool manages a pool of pre-created WebViews for fast tab creation.
// In Phase 1 the pool is simple: Acquire pops or creates, Release destroys.
type WebViewPool struct {
	factory *WebViewFactory
	mu      sync.Mutex
	pool    []*WebView
	closed  bool
}

// newWebViewPool returns a pool backed by the given factory.
func newWebViewPool(factory *WebViewFactory) *WebViewPool {
	return &WebViewPool{
		factory: factory,
	}
}

// Acquire obtains a WebView from the pool. If a pre-warmed WebView is
// available it is returned immediately; otherwise a new one is created.
func (p *WebViewPool) Acquire(ctx context.Context) (port.WebView, error) {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil, errPoolClosed
	}

	if len(p.pool) > 0 {
		wv := p.pool[len(p.pool)-1]
		p.pool = p.pool[:len(p.pool)-1]
		p.mu.Unlock()
		wv.generation.Add(1)
		return wv, nil
	}
	p.mu.Unlock()

	return p.factory.Create(ctx)
}

// Release destroys the WebView. In Phase 1 there is no reuse — the browser
// is closed and all resources are freed immediately.
func (p *WebViewPool) Release(wv port.WebView) {
	if wv == nil {
		return
	}
	wv.Destroy()
}

// Prewarm creates count WebViews synchronously and adds them to the pool.
func (p *WebViewPool) Prewarm(count int) {
	if count <= 0 {
		return
	}

	ctx := context.Background()
	views := make([]*WebView, 0, count)
	for range count {
		wv, err := p.factory.Create(ctx)
		if err != nil {
			continue
		}
		if cefWV, ok := wv.(*WebView); ok {
			views = append(views, cefWV)
		}
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		for _, wv := range views {
			wv.Destroy()
		}
		return
	}
	p.pool = append(p.pool, views...)
}

// PrewarmAsync schedules WebView creation. In Phase 1 this calls Prewarm
// synchronously; a future phase will use the GTK idle loop to avoid blocking.
// TODO(phase2): schedule creation via glib.IdleAdd for non-blocking startup.
func (p *WebViewPool) PrewarmAsync(_ context.Context, count int) {
	p.Prewarm(count)
}

// Size returns the number of available WebViews currently in the pool.
func (p *WebViewPool) Size() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.pool)
}

// Close shuts down the pool and destroys all pooled WebViews.
func (p *WebViewPool) Close() {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	p.closed = true
	views := p.pool
	p.pool = nil
	p.mu.Unlock()

	for _, wv := range views {
		wv.Destroy()
	}
}
