package webkit

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bnema/dumber/internal/logging"
	"github.com/rs/zerolog"
)

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

	pool     chan *WebView
	logger   zerolog.Logger
	ctx      context.Context
	injector *ContentInjector
	router   *MessageRouter

	closed atomic.Bool
	wg     sync.WaitGroup
}

// NewWebViewPool creates a new WebView pool.
func NewWebViewPool(ctx context.Context, wkCtx *WebKitContext, settings *SettingsManager, cfg PoolConfig, injector *ContentInjector, router *MessageRouter, logger zerolog.Logger) *WebViewPool {
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
		logger:   logger.With().Str("component", "webview-pool").Logger(),
		ctx:      ctx,
		injector: injector,
		router:   router,
	}

	return p
}

// Acquire gets a WebView from the pool or creates a new one.
func (p *WebViewPool) Acquire(ctx context.Context) (*WebView, error) {
	if p.closed.Load() {
		return nil, context.Canceled
	}

	// Try to get from pool first (non-blocking)
	select {
	case wv := <-p.pool:
		if wv != nil && !wv.IsDestroyed() {
			// Ensure frontend is attached even if this WebView was pooled before injection was configured.
			_ = wv.AttachFrontend(p.ctx, p.injector, p.router)
			p.logger.Debug().Uint64("id", uint64(wv.ID())).Msg("acquired webview from pool")
			return wv, nil
		}
		// Fall through to create new if the pooled one was destroyed
	default:
		// Pool empty
	}

	// Pool empty or pooled view was destroyed, create new
	p.logger.Debug().Msg("pool empty, creating new webview")
	return p.createWebView()
}

func (p *WebViewPool) createWebView() (*WebView, error) {
	wv, err := NewWebView(p.wkCtx, p.settings, p.logger)
	if err != nil {
		return nil, err
	}

	// Ensure WebView is visible when used
	wv.inner.SetVisible(true)

	if err := wv.AttachFrontend(p.ctx, p.injector, p.router); err != nil {
		logging.FromContext(p.ctx).Warn().Err(err).Msg("failed to attach frontend to new webview")
	}

	return wv, nil
}

// Release returns a WebView to the pool.
// The WebView is reset to about:blank before being pooled.
func (p *WebViewPool) Release(wv *WebView) {
	if wv == nil || wv.IsDestroyed() || p.closed.Load() {
		if wv != nil && !wv.IsDestroyed() {
			wv.Destroy()
		}
		return
	}

	// Reset to blank state
	_ = wv.LoadURI(p.ctx, "about:blank")

	// Clear callbacks
	wv.OnLoadChanged = nil
	wv.OnTitleChanged = nil
	wv.OnURIChanged = nil
	wv.OnProgressChanged = nil
	wv.OnClose = nil

	// Try to return to pool (non-blocking)
	select {
	case p.pool <- wv:
		p.logger.Debug().Uint64("id", uint64(wv.ID())).Msg("returned webview to pool")
	default:
		// Pool full, destroy the view
		p.logger.Debug().Uint64("id", uint64(wv.ID())).Msg("pool full, destroying webview")
		wv.Destroy()
	}
}

// Prewarm creates WebViews synchronously to populate the pool.
// Must be called from the GTK main thread (after GTK application is initialized).
func (p *WebViewPool) Prewarm(count int) {
	if count <= 0 {
		count = p.config.PrewarmCount
	}
	if count <= 0 {
		return
	}
	p.prewarmSync(count)
}

// prewarmSync creates WebViews synchronously.
func (p *WebViewPool) prewarmSync(count int) {
	for i := 0; i < count && !p.closed.Load(); i++ {
		// Check if pool is already at capacity
		if len(p.pool) >= p.config.MaxSize {
			break
		}

		wv, err := p.createWebView()
		if err != nil {
			p.logger.Warn().Err(err).Msg("failed to prewarm webview")
			continue
		}

		// Try to add to pool
		select {
		case p.pool <- wv:
			p.logger.Debug().Uint64("id", uint64(wv.ID())).Msg("prewarmed webview added to pool")
		default:
			// Pool full, destroy
			wv.Destroy()
			break
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
func (p *WebViewPool) Close() {
	if p.closed.Swap(true) {
		return // Already closed
	}

	// Wait for any background operations
	p.wg.Wait()

	// Drain and destroy all pooled views
	close(p.pool)
	for wv := range p.pool {
		if wv != nil && !wv.IsDestroyed() {
			wv.Destroy()
		}
	}

	p.logger.Debug().Msg("webview pool closed")
}
