package webkit

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/puregotk-webkit/webkit"
	"github.com/jwijenbergh/puregotk/v4/glib"
)

// ErrPoolClosed is returned when operations are attempted on a closed pool.
var ErrPoolClosed = errors.New("webview pool is closed")

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
		PrewarmCount: 4, // Pre-create 4 WebViews for faster initial tab creation
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
	bg bgColor

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
	p.bg.set(r, g, b, a)
}

// Acquire gets a WebView from the pool or creates a new one.
func (p *WebViewPool) Acquire(ctx context.Context) (*WebView, error) {
	log := logging.FromContext(ctx)

	if p.closed.Load() {
		return nil, ErrPoolClosed
	}

	// Try to get from pool first (non-blocking)
	select {
	case wv := <-p.pool:
		if wv != nil && !wv.IsDestroyed() {
			if r, g, b, a := p.bg.get(); a > 0 {
				wv.SetBackgroundColor(r, g, b, a)
			}
			wv.inner.AddCssClass("webview-themed")
			// Keep pooled WebViews hidden until we explicitly reveal them.
			wv.inner.SetVisible(false)

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

	wv, err := NewWebView(ctx, p.wkCtx, p.settings, p.bg.toGdkRGBA())
	if err != nil {
		return nil, err
	}

	// Add CSS class for theme background styling (prevents white flash)
	wv.inner.AddCssClass("webview-themed")

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

// PrewarmFirst creates exactly one WebView synchronously.
// Call this during startup (before GTK activate) to ensure first Acquire() is instant.
// This is the most impactful optimization for cold start since WebView creation
// is the heaviest operation (spawns WebKit web process).
// Returns error if WebView creation fails; caller should log and continue.
func (p *WebViewPool) PrewarmFirst(ctx context.Context) error {
	if p.closed.Load() {
		return ErrPoolClosed
	}
	if len(p.pool) > 0 {
		return nil // Already have at least one
	}

	log := logging.FromContext(ctx)
	wv, err := p.createWebView(ctx)
	if err != nil {
		return err
	}

	select {
	case p.pool <- wv:
		log.Debug().Uint64("id", uint64(wv.ID())).Msg("prewarmed first webview for cold start")
	default:
		wv.Destroy() // Pool somehow full (shouldn't happen)
	}
	return nil
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

// PrewarmAsync schedules WebView creation on the GTK idle loop.
// This avoids blocking startup (especially cold-start navigation) while still
// warming up WebViews for subsequent tab creation.
func (p *WebViewPool) PrewarmAsync(ctx context.Context, count int) {
	log := logging.FromContext(ctx)

	if count <= 0 {
		count = p.config.PrewarmCount
	}
	if count <= 0 {
		return
	}
	if p.closed.Load() {
		return
	}

	log.Debug().Int("count", count).Int("pool_size", len(p.pool)).Msg("scheduling async webview pool prewarm")

	p.wg.Add(1)
	var doneOnce sync.Once
	remaining := count

	var schedule func()
	schedule = func() {
		// Stop early if the caller canceled the operation.
		if err := ctx.Err(); err != nil {
			log.Debug().Err(err).Msg("async webview pool prewarm canceled")
			doneOnce.Do(p.wg.Done)
			return
		}

		cb := glib.SourceFunc(func(_ uintptr) bool {
			// Stop if the caller canceled the operation.
			if err := ctx.Err(); err != nil {
				log.Debug().Err(err).Msg("async webview pool prewarm canceled")
				doneOnce.Do(p.wg.Done)
				return false
			}

			// Stop if pool is closing/closed.
			if p.closed.Load() {
				doneOnce.Do(p.wg.Done)
				return false
			}

			// Stop early if we reached capacity.
			if len(p.pool) >= p.config.MaxSize {
				doneOnce.Do(p.wg.Done)
				return false
			}

			wv, err := p.createWebView(ctx)
			if err != nil {
				log.Warn().Err(err).Msg("failed to prewarm webview")
			} else {
				select {
				case p.pool <- wv:
					log.Debug().Uint64("id", uint64(wv.ID())).Msg("prewarmed webview added to pool")
				default:
					wv.Destroy()
				}
			}

			remaining--
			if remaining <= 0 {
				log.Debug().Int("pool_size", len(p.pool)).Msg("async webview pool prewarm complete")
				doneOnce.Do(p.wg.Done)
				return false
			}

			schedule()
			return false
		})
		// Use LOW priority so initial navigation and other high-priority work runs first
		glib.IdleAddFull(glib.PRIORITY_LOW, &cb, 0, nil)
	}

	schedule()
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

// RefreshScripts clears and re-injects scripts to all pooled WebViews.
// This is needed after adw.Init() to update dark mode preference that was
// injected with the wrong value during early prewarming.
func (p *WebViewPool) RefreshScripts(ctx context.Context) {
	if p.closed.Load() || p.injector == nil {
		return
	}

	log := logging.FromContext(ctx)

	// Drain and refresh each WebView, then put it back
	count := len(p.pool)
	if count == 0 {
		return
	}

	refreshed := 0
refreshLoop:
	for i := 0; i < count; i++ {
		select {
		case wv := <-p.pool:
			if wv == nil || wv.IsDestroyed() {
				continue
			}

			// Clear existing scripts and re-inject with updated values
			if wv.ucm != nil {
				wv.ucm.RemoveAllScripts()
				wv.ucm.RemoveAllStyleSheets()
				p.injector.InjectScripts(ctx, wv.ucm, wv.ID())
			}

			// Put back in pool
			select {
			case p.pool <- wv:
				refreshed++
			default:
				wv.Destroy()
			}
		default:
			// Pool was modified during iteration, stop
			break refreshLoop
		}
	}

	log.Debug().
		Int("refreshed", refreshed).
		Bool("prefers_dark", p.injector.PrefersDark()).
		Msg("refreshed scripts in pooled webviews")
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
