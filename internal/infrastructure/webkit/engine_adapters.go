package webkit

import (
	"context"
	"fmt"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/infrastructure/filtering"
	"github.com/bnema/dumber/internal/logging"
	"github.com/jwijenbergh/puregotk/v4/gio"
)

// --- WebViewFactory adapter ---

// webViewFactoryAdapter bridges *WebViewFactory to port.WebViewFactory.
// WebViewFactory.Create and CreateRelated return *WebView, not port.WebView,
// so this adapter wraps the return values.
type webViewFactoryAdapter struct {
	factory *WebViewFactory
}

func (a *webViewFactoryAdapter) Create(ctx context.Context) (port.WebView, error) {
	wv, err := a.factory.Create(ctx)
	if err != nil {
		return nil, err
	}
	return wv, nil
}

func (a *webViewFactoryAdapter) CreateRelated(ctx context.Context, parentID port.WebViewID) (port.WebView, error) {
	wv, err := a.factory.CreateRelated(ctx, parentID)
	if err != nil {
		return nil, err
	}
	return wv, nil
}

// --- WebViewPool adapter ---

// webViewPoolAdapter bridges *WebViewPool to port.WebViewPool.
// WebViewPool methods use *WebView and require a context; this adapter adapts
// the signatures to match the port interface.
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
	wwv, ok := wv.(*WebView)
	if !ok {
		logging.FromContext(a.ctx).Warn().
			Str("concrete_type", fmt.Sprintf("%T", wv)).
			Msg("webViewPoolAdapter.Release: unexpected type, cannot release to pool")
		return
	}
	a.pool.Release(a.ctx, wwv)
}

func (a *webViewPoolAdapter) Prewarm(count int) {
	a.pool.Prewarm(a.ctx, count)
}

func (a *webViewPoolAdapter) PrewarmAsync(ctx context.Context, count int) {
	a.pool.PrewarmAsync(ctx, count)
}

func (a *webViewPoolAdapter) Size() int {
	return a.pool.Size()
}

func (a *webViewPoolAdapter) Close() {
	a.pool.Close(a.ctx)
}

// --- SettingsApplier adapter ---

// settingsApplierAdapter bridges *SettingsManager to port.SettingsApplier.
type settingsApplierAdapter struct {
	settings *SettingsManager
}

func (a *settingsApplierAdapter) ApplyToAll(ctx context.Context, webviews []port.WebView) {
	for _, wv := range webviews {
		if wwv, ok := wv.(*WebView); ok && !wwv.IsDestroyed() {
			a.settings.ApplyToWebView(ctx, wwv.Widget())
		}
	}
}

// --- FilterApplier adapter ---

// filterApplierAdapter bridges *filtering.Manager to port.FilterApplier.
type filterApplierAdapter struct {
	manager *filtering.Manager
}

func (a *filterApplierAdapter) ApplyToAll(ctx context.Context, webviews []port.WebView) {
	for _, wv := range webviews {
		if wwv, ok := wv.(*WebView); ok && !wwv.IsDestroyed() {
			a.manager.ApplyTo(ctx, wwv.UserContentManager())
		}
	}
}

// --- FaviconDatabase adapter ---

// faviconDatabaseAdapter bridges *WebKitContext to port.FaviconDatabase.
// The WebKitContext exposes the favicon database; async lookup is a stub until
// the coordinator layer is migrated to use this port.
type faviconDatabaseAdapter struct {
	wkCtx *WebKitContext
}

func (a *faviconDatabaseAdapter) GetFaviconAsync(pageURL string, callback func(port.Texture)) {
	db := a.wkCtx.FaviconDatabase()
	if db == nil {
		callback(nil)
		return
	}

	asyncCb := gio.AsyncReadyCallback(func(_ uintptr, resultPtr uintptr, _ uintptr) {
		if resultPtr == 0 {
			callback(nil)
			return
		}

		result := &gio.AsyncResultBase{Ptr: resultPtr}
		texture, err := db.GetFaviconFinish(result)
		if err != nil || texture == nil {
			callback(nil)
			return
		}

		callback(texture)
	})

	db.GetFavicon(pageURL, nil, &asyncCb, 0)
}
