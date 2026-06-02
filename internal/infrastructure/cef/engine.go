package cef

import (
	"context"
	"crypto/subtle"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	purecef "github.com/bnema/purego-cef/cef"
	cef2gtk "github.com/bnema/purego-cef2gtk"

	"github.com/bnema/dumber/internal/application/dto"
	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/logging"
)

// Compile-time interface check.
var _ port.Engine = (*Engine)(nil)
var _ port.FilterManagerProvider = (*Engine)(nil)
var _ port.AlreadyRunningAppRelaunchHandlerSetter = (*Engine)(nil)

// Engine implements port.Engine for the CEF browser backend.
// It manages the CEF lifecycle and provides access to all engine subsystems.
type Engine struct {
	ctx                context.Context
	factory            *WebViewFactory
	pool               *WebViewPool
	profileLogDir      string
	runtimeCEFDir      string
	renderStackPlan    cef2gtk.RenderStackPlan
	filterManager      port.FilterManager
	filterBackend      cefFilterBackend
	applicationScaleMu sync.RWMutex
	applicationScale   float64

	messageRouter *MessageRouter
	schemeHandler *dumbSchemeHandler
	contentInj    *contentInjector

	registerHandlers                 HandlerRegistrar
	registerAccentHandlers           AccentHandlerRegistrar
	currentConfigPayload             func() ([]byte, error)
	defaultConfigPayload             func() ([]byte, error)
	ctxMenuBuilder                   port.ContextMenuBuilder
	ctxMenuExecutorFactory           port.ContextMenuActionExecutorFactory
	ctxMenuRenderer                  ContextMenuRenderer
	clipboard                        port.Clipboard
	clipboardTextOrchestrator        port.ClipboardTextOrchestrator
	onClipboardCopied                func(textLen int)
	resolver                         port.ImageDataResolver
	downloadMu                       sync.RWMutex
	downloadHandler                  *downloadHandler
	alreadyRunningAppRelaunchMu      sync.RWMutex
	alreadyRunningAppRelaunchHandler func(string)

	// activeWebViews tracks all live webviews for CSS broadcast.
	activeWebViews  sync.Map // map[port.WebViewID]*WebView
	browserWebViews sync.Map // map[int32]*WebView
	activeCount     atomic.Int32

	// shutdownNotify is signaled when the active webview count reaches 0
	// during shutdown, replacing the busy-wait poll in closeActiveWebViews.
	shutdownNotify chan struct{}

	// Cross-layer render diagnostics.
	cefHeartbeatStop chan struct{}
	cefHeartbeatDone chan struct{}
	cefUIHeartbeat   cefThreadHeartbeatState
	cefIOHeartbeat   cefThreadHeartbeatState

	// Diagnostic counters.
	contextInitializedCount atomic.Uint64
	childLaunchRenderer     atomic.Uint64
	childLaunchGPU          atomic.Uint64
	childLaunchOther        atomic.Uint64

	browserCreateRequests   atomic.Uint64
	browserAfterCreated     atomic.Uint64
	browserCreateLastAtMs   atomic.Int64
	browserCreateLastResult atomic.Int32
	browserCreateLastWidth  atomic.Int32
	browserCreateLastHeight atomic.Int32
	browserCreateComplete   atomic.Bool
}

func (e *Engine) Factory() port.WebViewFactory {
	return &webViewFactoryAdapter{factory: e.factory}
}

func (e *Engine) contextMenuRenderer() ContextMenuRenderer {
	if e == nil {
		return nil
	}
	return e.ctxMenuRenderer
}

func (e *Engine) Pool() port.WebViewPool {
	return &webViewPoolAdapter{pool: e.pool, ctx: e.ctx}
}

func (e *Engine) ContentInjector() port.ContentInjector {
	if e.contentInj != nil {
		return e.contentInj
	}
	return &noopContentInjector{}
}

// SettingsApplier returns a no-op applier (Phase 1 stub).
func (e *Engine) SettingsApplier() port.SettingsApplier {
	return &noopSettingsApplier{}
}

// FilterApplier returns nil because CEF filtering is applied through request interception.
func (e *Engine) FilterApplier() port.FilterApplier {
	return nil
}

// InternalFilterManager returns the shared lifecycle manager for CEF request filtering.
func (e *Engine) InternalFilterManager() port.FilterManager { return e.filterManager }

// FaviconDatabase returns a no-op database (Phase 1 stub).
func (e *Engine) FaviconDatabase() port.FaviconDatabase {
	return &noopFaviconDatabase{}
}

// SetColorResolver sets the color scheme resolver on the content injector.
// This allows dark mode detection for internal pages. Safe to call after
// engine creation (e.g., from bootstrap wiring).
func (e *Engine) SetColorResolver(resolver port.ColorSchemeResolver) {
	if e.contentInj != nil {
		e.contentInj.setColorResolver(resolver)
	}
}

// SetAlreadyRunningAppRelaunchHandler configures the callback invoked when CEF
// relaunches an already-running app instance.
func (e *Engine) SetAlreadyRunningAppRelaunchHandler(handler func(string)) {
	e.alreadyRunningAppRelaunchMu.Lock()
	defer e.alreadyRunningAppRelaunchMu.Unlock()
	e.alreadyRunningAppRelaunchHandler = handler
}

func (e *Engine) alreadyRunningAppRelaunchCallback() func(string) {
	e.alreadyRunningAppRelaunchMu.RLock()
	defer e.alreadyRunningAppRelaunchMu.RUnlock()
	return e.alreadyRunningAppRelaunchHandler
}

// registerWebView adds a webview to the active tracking map.
func (e *Engine) registerWebView(wv *WebView) {
	e.activeWebViews.Store(wv.id, wv)
	e.activeCount.Add(1)
}

func (e *Engine) lookupWebView(id port.WebViewID) *WebView {
	if e == nil {
		return nil
	}
	current, ok := e.activeWebViews.Load(id)
	if !ok {
		return nil
	}
	wv, _ := current.(*WebView)
	return wv
}

func (e *Engine) bindBrowserWebView(browser purecef.Browser, wv *WebView) {
	if e == nil || browser == nil || wv == nil {
		return
	}
	browserID := browser.GetIdentifier()
	if browserID <= 0 {
		logging.FromContext(e.ctx).Debug().Int32("browser_id", browserID).Msg("cef: skipped browser/webview binding for invalid browser id")
		return
	}
	e.browserWebViews.Store(browserID, wv)
}

func (e *Engine) unbindBrowserWebView(browserID int32, wv *WebView) {
	if e == nil || browserID <= 0 {
		return
	}
	if current, ok := e.browserWebViews.Load(browserID); ok {
		if existing, ok := current.(*WebView); ok && existing == wv {
			e.browserWebViews.Delete(browserID)
		}
	}
}

// unregisterWebView removes a webview from the active tracking map.
// If the count reaches 0 and a shutdown is in progress, it signals shutdownNotify.
func (e *Engine) unregisterWebView(wv *WebView, browserID int32) {
	e.activeWebViews.Delete(wv.id)
	e.unbindBrowserWebView(browserID, wv)
	if e.activeCount.Add(-1) == 0 && e.shutdownNotify != nil {
		select {
		case e.shutdownNotify <- struct{}{}:
		default:
		}
	}
}

// internalSchemePath is the URI scheme used for internal app resources.
const internalSchemePath = "dumb://"

// InternalSchemePath returns the URI scheme used for internal app resources.
func (e *Engine) InternalSchemePath() string {
	return internalSchemePath
}

// Close releases all resources held by the engine.
func (e *Engine) Close() error {
	log := logging.FromContext(e.ctx)
	activeBefore := e.activeWebViewCount()
	log.Debug().
		Int("active_webviews", activeBefore).
		Msg("cef: engine close started")

	e.closeActiveWebViews()
	e.pool.Close()
	e.stopCEFHeartbeat()

	if activeAfter := e.activeWebViewCount(); activeAfter > 0 {
		log.Warn().
			Int("remaining_active_webviews", activeAfter).
			Msg("cef: shutting down with active webviews still registered")
	}

	purecef.Shutdown()
	log.Debug().Msg("cef: engine close completed")
	return nil
}

// cefShutdownWaitTimeout gives CEF browser destruction enough time to flush
// GTK/DMABUF texture teardown and child-process shutdown on busy media pages.
// It is intentionally longer than a UI-frame timeout; Close still logs if the
// wait expires so slow shutdown remains visible during diagnostics.
const cefShutdownWaitTimeout = 10 * time.Second

func (e *Engine) activeWebViewCount() int {
	return int(e.activeCount.Load())
}

func (e *Engine) closeActiveWebViews() {
	if e == nil {
		return
	}

	log := logging.FromContext(e.ctx)
	e.shutdownNotify = make(chan struct{}, 1)
	webViews := make([]*WebView, 0, e.activeWebViewCount())
	e.activeWebViews.Range(func(_, value any) bool {
		wv, ok := value.(*WebView)
		if ok && wv != nil {
			webViews = append(webViews, wv)
		}
		return true
	})

	if len(webViews) == 0 {
		return
	}

	log.Debug().
		Int("count", len(webViews)).
		Msg("cef: closing active webviews before shutdown")

	for _, wv := range webViews {
		wv.Destroy()
	}

	// Check immediately in case all webviews closed synchronously.
	if e.activeWebViewCount() == 0 {
		log.Debug().Msg("cef: all active webviews closed before shutdown")
		e.destroyClosedWebViewBridges(webViews)
		return
	}

	// Wait for unregisterWebView to signal that all webviews are gone, or timeout.
	select {
	case <-e.shutdownNotify:
		log.Debug().Msg("cef: all active webviews closed before shutdown")
	case <-time.After(cefShutdownWaitTimeout):
		log.Warn().
			Int("remaining", e.activeWebViewCount()).
			Str("timeout", cefShutdownWaitTimeout.String()).
			Msg("cef: timed out waiting for OnBeforeClose before shutdown")
	}
	e.destroyClosedWebViewBridges(webViews)
}

func (e *Engine) destroyClosedWebViewBridges(webViews []*WebView) {
	for _, wv := range webViews {
		if wv != nil && wv.destroyed.Load() {
			wv.destroyViewBridgeOnGTKSync()
		}
	}
}

// RegisterHandlers registers all WebUI message bridge handlers with the message router.
// Handler registration follows the same pattern as WebKit's handlers/register.go:
// each handler is registered by message type with the router, which dispatches
// incoming /api/message POSTs to the correct handler based on Message.Type.
func (e *Engine) RegisterHandlers(ctx context.Context, deps port.HandlerDependencies) error {
	e.clipboardTextOrchestrator = deps.ClipboardTextOrchestrator
	e.onClipboardCopied = deps.OnClipboardCopied
	if e.messageRouter == nil || e.registerHandlers == nil {
		return nil
	}
	return e.registerHandlers(ctx, e.messageRouter, deps)
}

func (e *Engine) RegisterAccentHandlers(ctx context.Context, handler port.AccentKeyHandler) error {
	if e.messageRouter == nil || handler == nil || e.registerAccentHandlers == nil {
		return nil
	}
	return e.registerAccentHandlers(ctx, e.messageRouter, handler)
}

// ConfigureDownloads sets up download handling for all CEF webviews.
func (e *Engine) ConfigureDownloads(
	_ context.Context,
	downloadPath string,
	eventHandler port.DownloadEventHandler,
	preparer port.DownloadPreparer,
) error {
	if preparer == nil {
		return fmt.Errorf("cef: download preparer is required")
	}
	e.downloadMu.Lock()
	e.downloadHandler = newDownloadHandler(downloadPath, eventHandler, preparer)
	e.downloadMu.Unlock()
	return nil
}

// OnToolkitReady is called after the UI toolkit has initialized.
// With multi-threaded message loop enabled, CEF drives its own pump
// and no manual pump is needed.
func (e *Engine) OnToolkitReady(_ context.Context) error {
	log := logging.FromContext(e.ctx)
	log.Debug().Msg("cef: OnToolkitReady called")
	return nil
}

// UpdateAppearance updates the default background color for new WebViews.
func (e *Engine) UpdateAppearance(_ context.Context, r, g, b, alpha float64) error {
	if e.factory != nil {
		e.factory.setDefaultBackgroundColor(r, g, b, alpha)
	}
	return nil
}

// UpdateSettings applies runtime config changes to engine internals.
//
// CEF view scaling is fixed when each cef2gtk view is constructed. Changes to
// default_ui_scale therefore apply to newly-created CEF views in this engine;
// existing views continue using their construction-time scale until reload under
// a new engine lifetime. Browser zoom remains independently adjustable at
// runtime through SetZoomLevel.
func (e *Engine) UpdateSettings(ctx context.Context, update port.EngineSettingsUpdate) error {
	newScale := normalizedApplicationScale(update.Settings.DefaultUIScale)
	e.applicationScaleMu.Lock()
	oldScale := e.applicationScale
	if oldScale != newScale {
		e.applicationScale = newScale
	}
	e.applicationScaleMu.Unlock()

	if oldScale != newScale {
		logging.FromContext(ctx).Info().
			Float64("old_application_scale", oldScale).
			Float64("new_application_scale", newScale).
			Msg("cef: application scale update will apply to newly-created CEF views")
	}
	return nil
}

func (e *Engine) currentApplicationScale() float64 {
	if e == nil {
		return 1
	}
	e.applicationScaleMu.RLock()
	defer e.applicationScaleMu.RUnlock()
	return normalizedApplicationScale(e.applicationScale)
}

func (e *Engine) handleExplicitClipboardBridgeText(viewID port.WebViewID, action, text string) {
	if e == nil || text == "" {
		return
	}
	if e.clipboardTextOrchestrator == nil {
		logging.FromContext(e.currentContext()).Debug().
			Str("action", action).
			Msg("cef: clipboard orchestration skipped — orchestrator nil")
		return
	}
	if err := e.clipboardTextOrchestrator.HandleExplicitCopy(e.currentContext(), dto.ExplicitClipboardInput{
		Text:         text,
		Action:       action,
		SourceEngine: dto.ClipboardSourceCEF,
		ViewID:       uint64(viewID),
	}); err != nil {
		logging.FromContext(e.currentContext()).Debug().
			Err(err).
			Str("action", action).
			Int("text_len", utf8.RuneCountInString(text)).
			Msg("cef: clipboard explicit copy handling failed")
	}
}

func (e *Engine) handleClipboardSelectionUpdate(viewID port.WebViewID, text string) {
	if e == nil || e.clipboardTextOrchestrator == nil {
		return
	}
	if err := e.clipboardTextOrchestrator.HandleSelectionUpdate(e.currentContext(), dto.SelectionClipboardInput{
		Text:         text,
		SourceEngine: dto.ClipboardSourceCEF,
		ViewID:       uint64(viewID),
	}); err != nil {
		logging.FromContext(e.currentContext()).Debug().
			Err(err).
			Int("text_len", utf8.RuneCountInString(text)).
			Msg("cef: clipboard selection handling failed")
	}
}

func (e *Engine) notifyClipboardCopied(text string) {
	if e == nil || e.onClipboardCopied == nil || text == "" {
		return
	}
	e.onClipboardCopied(utf8.RuneCountInString(text))
}

// SetHandlerContext sets the base context for message handler dispatch.
func (e *Engine) SetHandlerContext(ctx context.Context) {
	e.ctx = ctx
	logger := logging.FromContext(ctx)
	logger.Info().
		Str("settings_cef_dir", e.runtimeCEFDir).
		Str("env_cef_dir", os.Getenv("CEF_DIR")).
		Msg("cef: runtime selection")
	if libcefPath := loadedLibCEFPath(); libcefPath != "" {
		logger.Info().Str("libcef_path", libcefPath).Msg("cef: runtime library loaded")
	} else {
		logger.Warn().Msg("cef: runtime library loaded but libcef path was not found in /proc/self/maps")
	}
	if e.messageRouter != nil {
		e.messageRouter.SetBaseContext(ctx)
	}
}

func (e *Engine) recordContextInitialized() {
	count := e.contextInitializedCount.Add(1)
	logging.FromContext(e.ctx).Debug().
		Uint64("count", count).
		Msg("cef: OnContextInitialized")
}

func (e *Engine) recordChildProcessLaunch(processType, useAngle, ozonePlatform, renderNodeOverride, commandLine string) {
	switch processType {
	case "renderer":
		e.childLaunchRenderer.Add(1)
	case "gpu-process":
		e.childLaunchGPU.Add(1)
	default:
		e.childLaunchOther.Add(1)
	}

	logging.FromContext(e.ctx).Debug().
		Str("process_type", processType).
		Str("use_angle", useAngle).
		Str("ozone_platform", ozonePlatform).
		Str("render_node_override", renderNodeOverride).
		Strs("command_line_flags", safeChromiumCmdlineFlags(commandLine)).
		Uint64("renderer_launches", e.childLaunchRenderer.Load()).
		Uint64("gpu_launches", e.childLaunchGPU.Load()).
		Uint64("other_launches", e.childLaunchOther.Load()).
		Msg("cef: OnBeforeChildProcessLaunch")
}

func (e *Engine) recordBrowserCreateRequest(width, height, result int32) {
	count := e.browserCreateRequests.Add(1)
	e.browserCreateLastAtMs.Store(time.Now().UnixMilli())
	e.browserCreateLastResult.Store(result)
	e.browserCreateLastWidth.Store(width)
	e.browserCreateLastHeight.Store(height)

	logging.FromContext(e.ctx).Debug().
		Uint64("request_count", count).
		Int32("width", width).
		Int32("height", height).
		Int32("result", result).
		Msg("cef: BrowserHostCreateBrowser returned")
	if result != 1 {
		logging.FromContext(e.ctx).Warn().
			Uint64("request_count", count).
			Int32("width", width).
			Int32("height", height).
			Int32("result", result).
			Msg("cef: BrowserHostCreateBrowser returned non-success")
	}
}

func (e *Engine) recordBrowserAfterCreated(browser purecef.Browser) {
	count := e.browserAfterCreated.Add(1)
	if count >= e.browserCreateRequests.Load() {
		e.browserCreateComplete.Store(true)
	}
	browserID := int32(0)
	if browser != nil {
		browserID = browser.GetIdentifier()
	}
	logging.FromContext(e.ctx).Debug().
		Uint64("after_created_count", count).
		Uint64("create_requests", e.browserCreateRequests.Load()).
		Int32("browser_id", browserID).
		Msg("cef: browser created")
}

func (e *Engine) currentContext() context.Context {
	if e == nil || e.ctx == nil {
		return context.Background()
	}
	return e.ctx
}

func (e *Engine) validateBridgeRequest(browser purecef.Browser, bridgeNonce string) bool {
	if e == nil || browser == nil || bridgeNonce == "" {
		return false
	}

	wv := e.webViewForBrowser(browser)
	if wv == nil {
		return false
	}

	wv.mu.RLock()
	wvBridgeNonce := wv.bridgeNonce
	wv.mu.RUnlock()
	if wvBridgeNonce == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(wvBridgeNonce), []byte(bridgeNonce)) == 1
}

func (e *Engine) currentDownloadHandler() *downloadHandler {
	if e == nil {
		return nil
	}
	e.downloadMu.RLock()
	defer e.downloadMu.RUnlock()
	return e.downloadHandler
}
