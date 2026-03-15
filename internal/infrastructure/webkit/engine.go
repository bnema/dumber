package webkit

import (
	"context"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/infrastructure/filtering"
)

// Engine implements port.Engine for the WebKit browser engine.
type Engine struct {
	wkCtx         *WebKitContext
	settings      *SettingsManager
	injector      *ContentInjector
	messageRouter *MessageRouter
	pool          *WebViewPool
	factory       *WebViewFactory
	filterManager *filtering.Manager
	schemeHandler *DumbSchemeHandler
	schemePath    string
}

// Compile-time check that Engine implements port.Engine.
var _ port.Engine = (*Engine)(nil)

// Init is a no-op for WebKit — initialization happens in NewEngine.
func (e *Engine) Init(_ context.Context, _ port.EngineOptions) error {
	return nil
}

// Factory returns the WebViewFactory wrapped as a port.WebViewFactory.
func (e *Engine) Factory() port.WebViewFactory {
	return &webViewFactoryAdapter{factory: e.factory}
}

// Pool returns the WebViewPool wrapped as a port.WebViewPool.
func (e *Engine) Pool() port.WebViewPool {
	return &webViewPoolAdapter{pool: e.pool}
}

// ContentInjector returns the ContentInjector implementing port.ContentInjector.
func (e *Engine) ContentInjector() port.ContentInjector {
	return e.injector
}

// InternalSchemePath returns the URI scheme used for internal app resources.
func (e *Engine) InternalSchemePath() string {
	return e.schemePath
}

// Close releases all resources held by the engine.
func (e *Engine) Close() error {
	if e.pool != nil {
		e.pool.Close(context.Background())
	}
	return nil
}

// SchemeHandler returns a port.SchemeHandler adapter for the DumbSchemeHandler.
func (e *Engine) SchemeHandler() port.SchemeHandler {
	return &schemeHandlerAdapter{handler: e.schemeHandler}
}

// MessageRouter returns a port.MessageRouter adapter for the internal MessageRouter.
func (e *Engine) MessageRouter() port.MessageRouter {
	return &messageRouterAdapter{router: e.messageRouter}
}

// SettingsApplier returns a port.SettingsApplier adapter for the SettingsManager.
func (e *Engine) SettingsApplier() port.SettingsApplier {
	return &settingsApplierAdapter{settings: e.settings}
}

// FilterApplier returns a port.FilterApplier adapter, or nil if filtering is disabled.
func (e *Engine) FilterApplier() port.FilterApplier {
	if e.filterManager == nil {
		return nil
	}
	return &filterApplierAdapter{manager: e.filterManager}
}

// FaviconDatabase returns a port.FaviconDatabase adapter for async favicon lookups.
func (e *Engine) FaviconDatabase() port.FaviconDatabase {
	return &faviconDatabaseAdapter{wkCtx: e.wkCtx}
}

// --- Temporary accessors for migration (M3->M4 bridge) ---
// These will be removed when ui.Dependencies is updated to use port.Engine.

func (e *Engine) InternalContext() *WebKitContext           { return e.wkCtx }
func (e *Engine) InternalSettings() *SettingsManager        { return e.settings }
func (e *Engine) InternalInjector() *ContentInjector        { return e.injector }
func (e *Engine) InternalMessageRouter() *MessageRouter     { return e.messageRouter }
func (e *Engine) InternalPool() *WebViewPool                { return e.pool }
func (e *Engine) InternalFilterManager() *filtering.Manager { return e.filterManager }
