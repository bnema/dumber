package webkit

import (
	"context"
	"fmt"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/infrastructure/filtering"
	"github.com/rs/zerolog"
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
	pool   *WebViewPool
	logger zerolog.Logger
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
		a.logger.Warn().
			Str("concrete_type", fmt.Sprintf("%T", wv)).
			Msg("webViewPoolAdapter.Release: unexpected type, cannot release to pool")
		return
	}
	a.pool.Release(context.Background(), wwv)
}

func (a *webViewPoolAdapter) Prewarm(count int) {
	a.pool.Prewarm(context.Background(), count)
}

func (a *webViewPoolAdapter) Size() int {
	return a.pool.Size()
}

func (a *webViewPoolAdapter) Close() {
	a.pool.Close(context.Background())
}

// --- SchemeHandler adapter ---

// schemeHandlerAdapter bridges *DumbSchemeHandler to port.SchemeHandler.
// DumbSchemeHandler uses a page-based registration pattern (RegisterPage) rather
// than a generic scheme+handler registration; this adapter is a stub until
// consumers are migrated.
type schemeHandlerAdapter struct {
	handler *DumbSchemeHandler
	logger  zerolog.Logger
}

func (a *schemeHandlerAdapter) RegisterScheme(scheme string, _ func(uri string) ([]byte, string, error)) {
	// DumbSchemeHandler handles the "dumb" scheme exclusively via RegisterPage/RegisterWithContext.
	// Generic scheme registration is not yet wired through this adapter.
	a.logger.Warn().
		Str("scheme", scheme).
		Msg("schemeHandlerAdapter.RegisterScheme: not implemented — DumbSchemeHandler uses RegisterPage/RegisterWithContext pattern")
}

// --- MessageRouter adapter ---

// messageRouterAdapter bridges *MessageRouter to port.MessageRouter.
// The internal MessageRouter uses a typed MessageHandler interface rather than
// the simple func(string)(string,error) signature in the port interface.
// PostMessage is also not directly available on the internal type.
type messageRouterAdapter struct {
	router *MessageRouter
}

func (*messageRouterAdapter) RegisterHandler(_ string, _ func(message string) (string, error)) {
	// Internal MessageRouter.RegisterHandler takes a MessageHandler interface, not a plain func.
	// Wire up via RegisterHandler(msgType, MessageHandlerFunc{...}) when consumers are migrated.
	panic("not implemented — use MessageRouter.RegisterHandler(msgType, MessageHandler) directly")
}

func (*messageRouterAdapter) PostMessage(webviewID port.WebViewID, message string) error {
	wv := LookupWebView(webviewID)
	if wv == nil {
		return fmt.Errorf("webview %d not found", webviewID)
	}
	wv.RunJavaScript(context.Background(), message, "")
	return nil
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

func (*faviconDatabaseAdapter) GetFaviconAsync(_ string, callback func(port.Texture)) {
	// FaviconDatabase async lookup is not yet wired through this adapter.
	// The underlying API is: wkCtx.FaviconDatabase().GetFavicon(uri, callback).
	// This stub satisfies the interface contract; callers will receive nil.
	callback(nil)
}
