package webkit

import (
	"context"
	"fmt"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/infrastructure/filtering"
	"github.com/bnema/dumber/internal/infrastructure/handlers"
	"github.com/rs/zerolog"
)

// Engine implements port.Engine for the WebKit browser engine.
type Engine struct {
	ctx                    context.Context
	wkCtx                  *WebKitContext
	settings               *SettingsManager
	injector               *ContentInjector
	messageRouter          *MessageRouter
	pool                   *WebViewPool
	factory                *WebViewFactory
	filterManager          *filtering.Manager
	schemeHandler          *DumbSchemeHandler
	schemePath             string
	logger                 zerolog.Logger
	ctxMenuBuilder         port.ContextMenuBuilder
	ctxMenuExecutorFactory port.ContextMenuActionExecutorFactory
	onClipboardCopied      func(textLen int)
	downloadPath           string
	downloadPreparer       port.DownloadPreparer
	clipboard              port.Clipboard // captured during RegisterHandlers for context menu wiring
}

// Compile-time check that Engine implements port.Engine.
var _ port.Engine = (*Engine)(nil)

// Factory returns the WebViewFactory wrapped as a port.WebViewFactory.
func (e *Engine) Factory() port.WebViewFactory {
	return &webViewFactoryAdapter{factory: e.factory}
}

// Pool returns the WebViewPool wrapped as a port.WebViewPool.
func (e *Engine) Pool() port.WebViewPool {
	return &webViewPoolAdapter{pool: e.pool, ctx: e.ctx}
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
		e.pool.Close(e.ctx)
	}
	return nil
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

// InternalFilterManager returns the FilterManager for content filter lifecycle.
// This is on the concrete *Engine type (not the port.Engine interface) because
// FilterManager is a webkit-specific concern used only during dependency wiring.
func (e *Engine) InternalFilterManager() port.FilterManager { return e.filterManager }

// SetHandlerContext sets the base context for message handler dispatch.
func (e *Engine) SetHandlerContext(ctx context.Context) {
	if e.messageRouter != nil {
		e.messageRouter.SetBaseContext(ctx)
	}
}

// RegisterHandlers registers all WebUI message bridge handlers.
func (e *Engine) RegisterHandlers(ctx context.Context, deps port.HandlerDependencies) error {
	if e.messageRouter == nil {
		return fmt.Errorf("message router not initialized")
	}
	// Capture clipboard for context menu pipeline wiring in ConfigureDownloads.
	e.clipboard = deps.Clipboard
	e.onClipboardCopied = deps.OnClipboardCopied
	return handlers.RegisterAll(ctx, e.messageRouter, deps)
}

// RegisterAccentHandlers registers accent/dead-key input handlers.
func (e *Engine) RegisterAccentHandlers(ctx context.Context, handler port.AccentKeyHandler) error {
	if e.messageRouter == nil {
		return fmt.Errorf("message router not initialized")
	}
	return handlers.RegisterAccentHandlers(ctx, e.messageRouter, handler)
}

// ConfigureDownloads sets up download handling.
func (e *Engine) ConfigureDownloads(
	ctx context.Context, downloadPath string,
	eventHandler port.DownloadEventHandler, preparer port.DownloadPreparer,
) error {
	if e.wkCtx == nil {
		return fmt.Errorf("webkit context not initialized")
	}
	if preparer == nil {
		return fmt.Errorf("download preparer is required")
	}
	handler := NewDownloadHandler(downloadPath, eventHandler, preparer)
	e.wkCtx.SetDownloadHandler(ctx, handler)
	e.downloadPath = downloadPath
	e.downloadPreparer = preparer

	// Wire the shared context menu pipeline now that all dependencies are
	// available. RegisterHandlers (called earlier) captured the clipboard;
	// ConfigureDownloads supplies the download preparer and path.
	e.installContextMenuPipeline()

	return nil
}

// OnToolkitReady refreshes pooled WebViews after toolkit init.
func (e *Engine) OnToolkitReady(ctx context.Context) error {
	if e.pool != nil {
		e.pool.RefreshScripts(ctx)
	}
	return nil
}

// UpdateAppearance updates default background color for pool and factory.
func (e *Engine) UpdateAppearance(_ context.Context, r, g, b, alpha float64) error {
	if e.pool != nil {
		e.pool.SetBackgroundColor(float32(r), float32(g), float32(b), float32(alpha))
	}
	if e.factory != nil {
		e.factory.SetBackgroundColor(float32(r), float32(g), float32(b), float32(alpha))
	}
	return nil
}

// UpdateSettings applies runtime config changes to engine settings.
func (e *Engine) UpdateSettings(ctx context.Context, update entity.EngineSettingsUpdate) error {
	if e.settings != nil {
		e.settings.UpdateFromPayload(ctx, update.Settings)
	}
	return nil
}

// installContextMenuPipeline creates the shared context menu pipeline and
// propagates it to the factory (and pool). This is called once during startup,
// after both RegisterHandlers (clipboard) and ConfigureDownloads (download
// deps) have run. Kept on the concrete *Engine type because it is a
// WebKit-specific concern.
func (e *Engine) installContextMenuPipeline() {
	resolver := NewContextMenuResolver()
	saver := NewResolvedImageSaver(e.downloadPreparer, e.downloadPath)

	// WebKit's context-menu signal fires on the GTK main thread, so no
	// dispatch wrapper is needed.
	renderer := NewRenderer(nil)

	pipeline := &contextMenuPipeline{
		builder:         e.ctxMenuBuilder,
		executorFactory: e.ctxMenuExecutorFactory,
		clipboard:       e.clipboard,
		onCopied:        e.onClipboardCopied,
		resolver:        resolver,
		saver:           saver,
		renderer:        renderer,
	}
	e.SetContextMenuPipeline(pipeline)
}

// SetContextMenuPipeline configures the context menu pipeline for all WebViews
// created by the factory (and pool). Call this after the use cases and renderer
// are available in the bootstrap layer.
func (e *Engine) SetContextMenuPipeline(pipeline *contextMenuPipeline) {
	if e.factory != nil {
		e.factory.SetContextMenuPipeline(pipeline)
	}
}
