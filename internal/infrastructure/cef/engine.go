package cef

import (
	"context"

	purecef "github.com/bnema/purego-cef/cef"
	"github.com/rs/zerolog"

	"github.com/bnema/dumber/internal/application/port"
)

// Compile-time interface check.
var _ port.Engine = (*Engine)(nil)

// Engine implements port.Engine for the CEF browser backend.
// It manages the CEF lifecycle and provides access to all engine subsystems.
type Engine struct {
	ctx     context.Context
	gl      *glLoader
	factory *WebViewFactory
	pool    *WebViewPool
	logger  zerolog.Logger
}

// Factory returns the WebViewFactory for creating new WebView instances.
func (e *Engine) Factory() port.WebViewFactory {
	return &webViewFactoryAdapter{factory: e.factory}
}

// Pool returns the WebViewPool for acquiring pre-warmed WebView instances.
func (e *Engine) Pool() port.WebViewPool {
	return &webViewPoolAdapter{pool: e.pool, ctx: e.ctx}
}

// ContentInjector returns a no-op injector (Phase 1 stub).
func (e *Engine) ContentInjector() port.ContentInjector {
	return &noopContentInjector{}
}

// SettingsApplier returns a no-op applier (Phase 1 stub).
func (e *Engine) SettingsApplier() port.SettingsApplier {
	return &noopSettingsApplier{}
}

// FilterApplier returns nil (content filtering not yet supported for CEF).
func (e *Engine) FilterApplier() port.FilterApplier {
	return nil
}

// FaviconDatabase returns a no-op database (Phase 1 stub).
func (e *Engine) FaviconDatabase() port.FaviconDatabase {
	return &noopFaviconDatabase{}
}

// InternalSchemePath returns the URI scheme used for internal app resources.
func (e *Engine) InternalSchemePath() string {
	return "cef://dumber/"
}

// Close releases all resources held by the engine.
func (e *Engine) Close() error {
	e.pool.Close()
	purecef.Shutdown()
	return nil
}

// RegisterHandlers registers all WebUI message bridge handlers (Phase 1 stub).
func (e *Engine) RegisterHandlers(_ context.Context, _ port.HandlerDependencies) error {
	return nil
}

// RegisterAccentHandlers registers accent/dead-key input handlers (Phase 1 stub).
func (e *Engine) RegisterAccentHandlers(_ context.Context, _ port.AccentKeyHandler) error {
	return nil
}

// ConfigureDownloads sets up download handling (Phase 1 stub).
func (e *Engine) ConfigureDownloads(_ context.Context, _ string, _ port.DownloadEventHandler, _ port.DownloadPreparer) error {
	return nil
}

// OnToolkitReady is called after the UI toolkit has initialized (Phase 1 stub).
func (e *Engine) OnToolkitReady(_ context.Context) error {
	return nil
}

// UpdateAppearance updates default background color for new WebViews (Phase 1 stub).
func (e *Engine) UpdateAppearance(_ context.Context, _, _, _, _ float64) error {
	return nil
}

// UpdateSettings applies runtime config changes to engine internals (Phase 1 stub).
func (e *Engine) UpdateSettings(_ context.Context, _ port.EngineSettingsUpdate) error {
	return nil
}

// SetHandlerContext sets the base context for message handler dispatch.
func (e *Engine) SetHandlerContext(ctx context.Context) {
	e.ctx = ctx
}
