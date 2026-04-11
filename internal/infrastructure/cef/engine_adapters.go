package cef

import (
	"context"
	"fmt"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/logging"
)

// --- WebViewFactory adapter ---

// webViewFactoryAdapter bridges *WebViewFactory to port.WebViewFactory.
// WebViewFactory.Create and CreateRelated return port.WebView directly,
// so this adapter is thin but ensures the concrete type stays private.
type webViewFactoryAdapter struct {
	factory *WebViewFactory
}

func (a *webViewFactoryAdapter) Create(ctx context.Context) (port.WebView, error) {
	return a.factory.Create(ctx)
}

func (a *webViewFactoryAdapter) CreateRelated(ctx context.Context, parentID port.WebViewID) (port.WebView, error) {
	return a.factory.CreateRelated(ctx, parentID)
}

// --- WebViewPool adapter ---

// webViewPoolAdapter bridges *WebViewPool to port.WebViewPool.
type webViewPoolAdapter struct {
	pool *WebViewPool
	ctx  context.Context
}

func (a *webViewPoolAdapter) Acquire(ctx context.Context) (port.WebView, error) {
	wv, err := a.pool.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	return wv, nil
}

func (a *webViewPoolAdapter) Release(wv port.WebView) {
	if wv == nil {
		return
	}
	cwv, ok := wv.(*WebView)
	if !ok {
		logging.FromContext(a.ctx).Warn().
			Str("concrete_type", fmt.Sprintf("%T", wv)).
			Msg("webViewPoolAdapter.Release: unexpected type, cannot release to pool")
		return
	}
	a.pool.Release(cwv)
}

func (a *webViewPoolAdapter) Prewarm(count int) {
	a.pool.Prewarm(count)
}

func (a *webViewPoolAdapter) PrewarmAsync(ctx context.Context, count int) {
	a.pool.PrewarmAsync(ctx, count)
}

func (a *webViewPoolAdapter) Size() int {
	return a.pool.Size()
}

func (a *webViewPoolAdapter) Close() {
	a.pool.Close()
}

// --- No-op stubs for Phase 1 ---

// noopContentInjector implements port.ContentInjector as a no-op.
type noopContentInjector struct{}

func (n *noopContentInjector) InjectThemeCSS(_ context.Context, _ string) error {
	return nil
}

func (n *noopContentInjector) InjectFindHighlightCSS(_ context.Context, _ string) error {
	return nil
}

func (n *noopContentInjector) RefreshScripts(_ context.Context, _ port.WebView) error {
	return nil
}

// noopSettingsApplier implements port.SettingsApplier as a no-op.
type noopSettingsApplier struct{}

func (n *noopSettingsApplier) ApplyToAll(_ context.Context, _ []port.WebView) {}

// noopFaviconDatabase implements port.FaviconDatabase as a no-op.
type noopFaviconDatabase struct{}

func (n *noopFaviconDatabase) GetFaviconAsync(_ string, callback func(port.Texture)) {
	callback(nil)
}
