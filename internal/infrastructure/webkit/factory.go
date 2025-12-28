package webkit

import (
	"context"
	"fmt"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/logging"
)

// WebViewFactory creates WebView instances for the application.
// It supports creating regular WebViews (optionally via pool) and
// related WebViews that share session/cookies with a parent (for popups).
type WebViewFactory struct {
	wkCtx         *WebKitContext
	settings      *SettingsManager
	pool          *WebViewPool
	injector      *ContentInjector
	router        *MessageRouter
	filterApplier FilterApplier // Optional content filter applier

	// Background color for WebViews (eliminates white flash)
	bg bgColor
}

// NewWebViewFactory creates a new WebViewFactory.
// The pool parameter is optional; if nil, WebViews are created directly.
func NewWebViewFactory(
	wkCtx *WebKitContext,
	settings *SettingsManager,
	pool *WebViewPool,
	injector *ContentInjector,
	router *MessageRouter,
) *WebViewFactory {
	return &WebViewFactory{
		wkCtx:    wkCtx,
		settings: settings,
		pool:     pool,
		injector: injector,
		router:   router,
	}
}

// SetFilterApplier sets the content filter applier.
// Filters will be applied to all newly created WebViews.
func (f *WebViewFactory) SetFilterApplier(applier FilterApplier) {
	f.filterApplier = applier
	// Also propagate to the pool if present
	if f.pool != nil {
		f.pool.SetFilterApplier(applier)
	}
}

// SetBackgroundColor sets the background color for newly created WebViews.
// This color is shown before content is painted, eliminating white flash.
func (f *WebViewFactory) SetBackgroundColor(r, g, b, a float32) {
	f.bg.set(r, g, b, a)
	// Also propagate to the pool if present
	if f.pool != nil {
		f.pool.SetBackgroundColor(r, g, b, a)
	}
}

// Create creates a new WebView instance.
// If a pool is configured, it will try to acquire from the pool first.
func (f *WebViewFactory) Create(ctx context.Context) (*WebView, error) {
	log := logging.FromContext(ctx)

	// Try pool first if available
	if f.pool != nil {
		wv, err := f.pool.Acquire(ctx)
		if err == nil && wv != nil {
			log.Debug().Uint64("id", uint64(wv.ID())).Msg("acquired webview from pool")
			return wv, nil
		}
		log.Debug().Err(err).Msg("pool acquire failed, creating directly")
	}

	// Create directly
	return f.createDirect(ctx)
}

// CreateRelated creates a WebView that shares session/cookies with the parent.
// This is required for popup windows to maintain authentication state (OAuth).
// Related WebViews bypass the pool since they must be linked to a specific parent.
func (f *WebViewFactory) CreateRelated(ctx context.Context, parentID port.WebViewID) (*WebView, error) {
	log := logging.FromContext(ctx)

	// Look up parent WebView
	parent := LookupWebView(parentID)
	if parent == nil {
		return nil, fmt.Errorf("parent webview %d not found", parentID)
	}
	if parent.IsDestroyed() {
		return nil, fmt.Errorf("parent webview %d is destroyed", parentID)
	}

	// Create related WebView
	wv, err := NewWebViewWithRelated(ctx, parent, f.settings)
	if err != nil {
		return nil, fmt.Errorf("create related webview: %w", err)
	}

	// Set background color to match theme (eliminates white flash)
	if r, g, b, a := f.bg.get(); a > 0 {
		wv.SetBackgroundColor(r, g, b, a)
	}

	// Keep hidden until content is painted
	wv.inner.SetVisible(false)

	// Attach frontend (scripts, message handler)
	if err := wv.AttachFrontend(ctx, f.injector, f.router); err != nil {
		log.Warn().Err(err).Uint64("id", uint64(wv.ID())).Msg("failed to attach frontend to related webview")
	}

	// Apply content filters if configured
	if f.filterApplier != nil {
		f.filterApplier.ApplyTo(ctx, wv.ucm)
	}

	log.Debug().
		Uint64("id", uint64(wv.ID())).
		Uint64("parent_id", uint64(parentID)).
		Msg("created related webview for popup")

	return wv, nil
}

// createDirect creates a WebView without using the pool.
func (f *WebViewFactory) createDirect(ctx context.Context) (*WebView, error) {
	log := logging.FromContext(ctx)

	wv, err := NewWebView(ctx, f.wkCtx, f.settings, f.bg.toGdkRGBA())
	if err != nil {
		return nil, err
	}

	// Add CSS class for theme background styling (prevents white flash)
	wv.inner.AddCssClass("webview-themed")

	// Keep hidden until content is painted
	wv.inner.SetVisible(false)

	// Attach frontend
	if err := wv.AttachFrontend(ctx, f.injector, f.router); err != nil {
		log.Warn().Err(err).Uint64("id", uint64(wv.ID())).Msg("failed to attach frontend to webview")
	}

	// Apply content filters if configured
	if f.filterApplier != nil {
		f.filterApplier.ApplyTo(ctx, wv.ucm)
	}

	log.Debug().Uint64("id", uint64(wv.ID())).Msg("created webview directly")
	return wv, nil
}

// Release returns a WebView to the pool if available, otherwise destroys it.
// Related WebViews (popups) should be destroyed directly, not released to pool.
func (f *WebViewFactory) Release(ctx context.Context, wv *WebView) {
	if wv == nil || wv.IsDestroyed() {
		return
	}

	if f.pool != nil {
		f.pool.Release(ctx, wv)
	} else {
		wv.Destroy()
	}
}
