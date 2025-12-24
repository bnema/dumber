package webkit

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/puregotk-webkit/webkit"
)

// FilterApplier applies content filters to a UserContentManager.
// This interface decouples the pool from the filtering package.
type FilterApplier interface {
	ApplyTo(ctx context.Context, ucm *webkit.UserContentManager)
}

// PoolConfig configures the WebView pool behavior.
type PoolConfig struct {
	// MinSize is the minimum number of warm WebViews to maintain.
	MinSize int
	// MaxSize is the maximum number of WebViews to keep in the pool.
	MaxSize int
	// PrewarmCount is the number of WebViews to pre-create on startup.
	PrewarmCount int
	// IdleTimeout is how long idle views stay in the pool before being destroyed.
	IdleTimeout time.Duration
}

// DefaultPoolConfig returns sensible defaults for the pool.
func DefaultPoolConfig() PoolConfig {
	return PoolConfig{
		MinSize:      2,
		MaxSize:      8,
		PrewarmCount: 2,
		IdleTimeout:  5 * time.Minute,
	}
}

// WebViewPool manages a pool of pre-created WebViews for fast tab creation.
type WebViewPool struct {
	wkCtx    *WebKitContext
	settings *SettingsManager
	config   PoolConfig

	pool          chan *WebView
	injector      *ContentInjector
	router        *MessageRouter
	filterApplier FilterApplier // Optional content filter applier

	// Background color for WebViews (eliminates white flash)
	bgR, bgG, bgB, bgA float32
	bgMu               sync.RWMutex

	closed atomic.Bool
	wg     sync.WaitGroup
}

// NewWebViewPool creates a new WebView pool.
func NewWebViewPool(
	ctx context.Context,
	wkCtx *WebKitContext,
	settings *SettingsManager,
	cfg PoolConfig,
	injector *ContentInjector,
	router *MessageRouter,
) *WebViewPool {
	if cfg.MaxSize <= 0 {
		cfg.MaxSize = 8
	}
	if cfg.MinSize <= 0 {
		cfg.MinSize = 2
	}
	if cfg.MinSize > cfg.MaxSize {
		cfg.MinSize = cfg.MaxSize
	}
	if ctx == nil {
		ctx = context.Background()
	}

	p := &WebViewPool{
		wkCtx:    wkCtx,
		settings: settings,
		config:   cfg,
		pool:     make(chan *WebView, cfg.MaxSize),
		injector: injector,
		router:   router,
	}

	log := logging.FromContext(ctx)
	log.Debug().Msg("creating webview pool")

	return p
}

// SetFilterApplier sets the content filter applier.
// Filters will be applied to all newly created WebViews.
func (p *WebViewPool) SetFilterApplier(applier FilterApplier) {
	p.filterApplier = applier
}

// SetBackgroundColor sets the background color for newly created WebViews.
// This color is shown before content is painted, eliminating white flash.
func (p *WebViewPool) SetBackgroundColor(r, g, b, a float32) {
	p.bgMu.Lock()
	p.bgR, p.bgG, p.bgB, p.bgA = r, g, b, a
	p.bgMu.Unlock()
}

// Acquire gets a WebView from the pool or creates a new one.
func (p *WebViewPool) Acquire(ctx context.Context) (*WebView, error) {
	log := logging.FromContext(ctx)

	if p.closed.Load() {
		return nil, context.Canceled
	}

	// Try to get from pool first (non-blocking)
	select {
	case wv := <-p.pool:
		if wv != nil && !wv.IsDestroyed() {
			// Ensure frontend is attached even if this WebView was pooled before injection was configured.
			_ = wv.AttachFrontend(ctx, p.injector, p.router)
			// Apply filters if available (may not have been applied during prewarm)
			if p.filterApplier != nil {
				p.filterApplier.ApplyTo(ctx, wv.ucm)
			}
			log.Debug().Uint64("id", uint64(wv.ID())).Msg("acquired webview from pool")
			return wv, nil
		}
		// Fall through to create new if the pooled one was destroyed
	default:
		// Pool empty
	}

	// Pool empty or pooled view was destroyed, create new
	log.Debug().Msg("pool empty, creating new webview")
	return p.createWebView(ctx)
}

func (p *WebViewPool) createWebView(ctx context.Context) (*WebView, error) {
	log := logging.FromContext(ctx)

	wv, err := NewWebView(ctx, p.wkCtx, p.settings)
	if err != nil {
		return nil, err
	}

	// Set background color to match theme (eliminates white flash)
	p.bgMu.RLock()
	r, g, b, a := p.bgR, p.bgG, p.bgB, p.bgA
	p.bgMu.RUnlock()
	if a > 0 { // Only set if a valid color was configured
		wv.SetBackgroundColor(r, g, b, a)
	}

	// Keep WebView hidden until content is painted (see content.go:onLoadCommitted)
	wv.inner.SetVisible(false)

	if err := wv.AttachFrontend(ctx, p.injector, p.router); err != nil {
		log.Warn().Err(err).Msg("failed to attach frontend to new webview")
	}

	// Apply content filters if configured
	if p.filterApplier != nil {
		p.filterApplier.ApplyTo(ctx, wv.ucm)
	}

	return wv, nil
}

// Release returns a WebView to the pool.
// For safety, we always destroy and never pool WebViews that have been used.
// This prevents crashes from GLib signals firing after the widget tree is modified.
// The pool is only used for prewarmed WebViews that haven't been attached to UI yet.
func (p *WebViewPool) Release(ctx context.Context, wv *WebView) {
	log := logging.FromContext(ctx)

	if wv == nil || wv.IsDestroyed() || p.closed.Load() {
		if wv != nil && !wv.IsDestroyed() {
			wv.Destroy()
		}
		return
	}

	// CRITICAL: Always destroy used WebViews instead of pooling them.
	// GLib signals are connected with closures that can fire asynchronously
	// even after we clear Go callbacks. Loading about:blank triggers signals
	// that can crash if the widget tree has been modified.
	//
	// The pool is designed for prewarmed WebViews only, not for recycling
	// WebViews that have been attached to the UI.
	log.Debug().Uint64("id", uint64(wv.ID())).Msg("destroying webview (not pooling used views)")
	wv.Destroy()
}

// Prewarm creates WebViews synchronously to populate the pool.
// Must be called from the GTK main thread (after GTK application is initialized).
func (p *WebViewPool) Prewarm(ctx context.Context, count int) {
	if count <= 0 {
		count = p.config.PrewarmCount
	}
	if count <= 0 {
		return
	}
	p.prewarmSync(ctx, count)
}

// prewarmSync creates WebViews synchronously.
func (p *WebViewPool) prewarmSync(ctx context.Context, count int) {
	log := logging.FromContext(ctx)

	for i := 0; i < count && !p.closed.Load(); i++ {
		// Check if pool is already at capacity
		if len(p.pool) >= p.config.MaxSize {
			break
		}

		wv, err := p.createWebView(ctx)
		if err != nil {
			log.Warn().Err(err).Msg("failed to prewarm webview")
			continue
		}

		// Try to add to pool
		select {
		case p.pool <- wv:
			log.Debug().Uint64("id", uint64(wv.ID())).Msg("prewarmed webview added to pool")
		default:
			// Pool full, destroy
			wv.Destroy()
		}

		// Small delay between creations to avoid overwhelming the system
		time.Sleep(50 * time.Millisecond)
	}
}

// Size returns the current number of WebViews in the pool.
func (p *WebViewPool) Size() int {
	return len(p.pool)
}

// Close shuts down the pool and destroys all pooled WebViews.
func (p *WebViewPool) Close(ctx context.Context) {
	if p.closed.Swap(true) {
		return // Already closed
	}

	log := logging.FromContext(ctx)

	// Wait for any background operations
	p.wg.Wait()

	// Drain and destroy all pooled views
	close(p.pool)
	for wv := range p.pool {
		if wv != nil && !wv.IsDestroyed() {
			wv.Destroy()
		}
	}

	log.Debug().Msg("webview pool closed")
}
