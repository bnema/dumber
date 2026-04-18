package cef

import (
	"context"
	"crypto/subtle"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	purecef "github.com/bnema/purego-cef/cef"

	"github.com/bnema/dumber/internal/application/dto"
	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/logging"
)

// Compile-time interface check.
var _ port.Engine = (*Engine)(nil)
var _ port.AlreadyRunningAppRelaunchHandlerSetter = (*Engine)(nil)

// Engine implements port.Engine for the CEF browser backend.
// It manages the CEF lifecycle and provides access to all engine subsystems.
type Engine struct {
	ctx     context.Context
	gl      *glLoader
	factory *WebViewFactory
	pool    *WebViewPool

	messageRouter *MessageRouter
	schemeHandler *dumbSchemeHandler
	contentInj    *contentInjector

	registerHandlers                 HandlerRegistrar
	registerAccentHandlers           AccentHandlerRegistrar
	currentConfigPayload             func() ([]byte, error)
	defaultConfigPayload             func() ([]byte, error)
	ctxMenuBuilder                   port.ContextMenuBuilder
	ctxMenuExecutorFactory           port.ContextMenuActionExecutorFactory
	clipboard                        port.Clipboard
	clipboardTextOrchestrator        port.ClipboardTextOrchestrator
	onClipboardCopied                func(textLen int)
	resolver                         port.ImageDataResolver
	mediaClassifier                  MediaClassifier
	alreadyRunningAppRelaunchMu      sync.RWMutex
	alreadyRunningAppRelaunchHandler func(string)

	// activeWebViews tracks all live webviews for CSS broadcast.
	activeWebViews sync.Map // map[port.WebViewID]*WebView
	activeCount    atomic.Int32

	// shutdownNotify is signaled when the active webview count reaches 0
	// during shutdown, replacing the busy-wait poll in closeActiveWebViews.
	shutdownNotify chan struct{}

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

	transcoderState       transcoderStartupState
	transcoderStateLogged atomic.Bool
}

type transcoderStartupState struct {
	ConfigEnabled  bool
	ProbeAttempted bool
	HWAccel        string
	MaxConcurrent  int
	Quality        string
	Status         string
	API            string
	Encoders       []string
	Decoders       []string
}

func (e *Engine) Factory() port.WebViewFactory {
	return &webViewFactoryAdapter{factory: e.factory}
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

// FilterApplier returns nil (content filtering not yet supported for CEF).
func (e *Engine) FilterApplier() port.FilterApplier {
	return nil
}

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

// unregisterWebView removes a webview from the active tracking map.
// If the count reaches 0 and a shutdown is in progress, it signals shutdownNotify.
func (e *Engine) unregisterWebView(wv *WebView) {
	e.activeWebViews.Delete(wv.id)
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

	if activeAfter := e.activeWebViewCount(); activeAfter > 0 {
		log.Warn().
			Int("remaining_active_webviews", activeAfter).
			Msg("cef: shutting down with active webviews still registered")
	}

	purecef.Shutdown()
	log.Debug().Msg("cef: engine close completed")
	return nil
}

const cefShutdownWaitTimeout = 2 * time.Second

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

// ConfigureDownloads sets up download handling (Phase 1 stub).
func (e *Engine) ConfigureDownloads(
	_ context.Context,
	_ string,
	_ port.DownloadEventHandler,
	_ port.DownloadPreparer,
) error {
	return ErrDownloadsUnsupported
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

// UpdateSettings applies runtime config changes to engine internals (Phase 1 stub).
func (e *Engine) UpdateSettings(_ context.Context, _ port.EngineSettingsUpdate) error {
	return nil
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
	if e.messageRouter != nil {
		e.messageRouter.SetBaseContext(ctx)
	}
	e.logTranscoderStartupState()
}

func (e *Engine) recordContextInitialized() {
	count := e.contextInitializedCount.Add(1)
	logging.FromContext(e.ctx).Debug().
		Uint64("count", count).
		Msg("cef: OnContextInitialized")
}

func (e *Engine) recordChildProcessLaunch(processType, useAngle, ozonePlatform, commandLine string) {
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
		Str("command_line", commandLine).
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

	browserID := browser.GetIdentifier()
	valid := false
	e.activeWebViews.Range(func(_, value any) bool {
		wv, ok := value.(*WebView)
		if !ok || wv == nil {
			return true
		}

		wv.mu.RLock()
		wvBrowser := wv.browser
		wvBridgeNonce := wv.bridgeNonce
		wv.mu.RUnlock()
		if wvBrowser == nil || wvBridgeNonce == "" || wvBrowser.GetIdentifier() != browserID {
			return true
		}
		if subtle.ConstantTimeCompare([]byte(wvBridgeNonce), []byte(bridgeNonce)) == 1 {
			valid = true
			return false
		}
		return true
	})
	return valid
}

func (e *Engine) logTranscoderStartupState() {
	if e == nil || !e.transcoderStateLogged.CompareAndSwap(false, true) {
		return
	}

	state := e.transcoderState
	log := logging.FromContext(e.currentContext())
	event := log.Info().
		Str("component", "cef-transcoder").
		Bool("config_enabled", state.ConfigEnabled).
		Bool("probe_attempted", state.ProbeAttempted).
		Str("hwaccel", state.HWAccel).
		Int("max_concurrent", state.MaxConcurrent).
		Str("quality", state.Quality).
		Str("status", state.Status).
		Bool("request_handler_enabled", e.factory != nil && e.factory.transcoder != nil)

	if state.API != "" {
		event = event.Str("api", state.API)
	}
	if len(state.Encoders) > 0 {
		event = event.Strs("encoders", state.Encoders)
	}
	if len(state.Decoders) > 0 {
		event = event.Strs("decoders", state.Decoders)
	}

	event.Msg("cef: transcoder startup state")
}
