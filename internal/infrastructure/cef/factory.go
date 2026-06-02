package cef

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	purecef "github.com/bnema/purego-cef/cef"
	cef2gtk "github.com/bnema/purego-cef2gtk"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/logging"
)

// Compile-time interface check.
var _ port.WebViewFactory = (*WebViewFactory)(nil)

// WebViewFactory creates new CEF off-screen browser WebViews. Each WebView
// gets a unique ID, its own cef2gtk bridge view, and an asynchronously-created
// CEF browser (via BrowserHostCreateBrowser).
type WebViewFactory struct {
	engine                      *Engine
	nextID                      atomic.Uint64
	adaptiveWindowlessFrameRate bool
	windowlessFrameRate         int32
	windowlessFrameRateMax      int32
	inputConfig                 RuntimeInputConfig
	bgColor                     atomic.Uint32 // packed ARGB for BrowserSettings.BackgroundColor
	audioOutputFactory          port.AudioOutputFactory
}

type webViewFactoryOptions struct {
	adaptiveWindowlessFrameRate bool
	windowlessFrameRate         int32
	windowlessFrameRateMax      int32
	inputConfig                 RuntimeInputConfig
	audioOutputFactory          port.AudioOutputFactory
}

type resizeNotifiableBrowserHost interface {
	WasResized()
	Invalidate(purecef.PaintElementType)
}

var cefBrowserHostCreateBrowser = purecef.BrowserHostCreateBrowser

const (
	pendingBrowserCreateRetryDelay     = 10 * time.Millisecond
	pendingBrowserCreateMaxPostRetries = 3
)

// newWebViewFactory returns a factory that will create WebViews.
func newWebViewFactory(engine *Engine, opts webViewFactoryOptions) *WebViewFactory {
	if opts.windowlessFrameRate < 1 {
		opts.windowlessFrameRate = 60
	}
	if opts.windowlessFrameRateMax < 1 {
		opts.windowlessFrameRateMax = 240
	}
	return &WebViewFactory{
		engine:                      engine,
		adaptiveWindowlessFrameRate: opts.adaptiveWindowlessFrameRate,
		windowlessFrameRate:         opts.windowlessFrameRate,
		windowlessFrameRateMax:      opts.windowlessFrameRateMax,
		inputConfig:                 opts.inputConfig,
		audioOutputFactory:          opts.audioOutputFactory,
	}
}

// setDefaultBackgroundColor stores a packed ARGB color for new browser creation.
func (f *WebViewFactory) setDefaultBackgroundColor(r, g, b, a float64) {
	const s = colorScale
	argb := uint32(a*s)<<24 | uint32(r*s)<<16 | uint32(g*s)<<8 | uint32(b*s)
	f.bgColor.Store(argb)
}

// Create builds a new CEF off-screen browser WebView. The browser is created
// asynchronously; the returned WebView is usable immediately but navigation
// will fail with errNoBrowser until OnAfterCreated fires.
func (f *WebViewFactory) Create(ctx context.Context) (port.WebView, error) {
	wv, err := f.newWebView(ctx)
	if err != nil {
		return nil, err
	}

	// Delegate windowless/shared-texture setup to the GTK bridge.
	windowInfo := purecef.NewWindowInfo()
	cef2gtk.ConfigureWindowInfo(&windowInfo, cef2gtk.WindowInfoOptions{})
	if externalBeginFrameEnabled() {
		windowInfo.ExternalBeginFrameEnabled = 1
	}

	// Configure BrowserSettings.
	settings := purecef.NewBrowserSettings()
	cef2gtk.ConfigureBrowserSettings(&settings, cef2gtk.BrowserSettingsOptions{WindowlessFrameRate: f.windowlessFrameRate})
	settings.LocalStorage = 1 // CEF_STATE_ENABLED
	if bg := f.bgColor.Load(); bg != 0 {
		settings.BackgroundColor = bg
	}

	// Defer browser creation until the bridge view has a non-zero size.
	// CEF requires GetViewRect to return a non-empty rect, but the GL area
	// is not yet realized at this point.
	f.configureInitialBrowserCreation(ctx, wv, wv.client, &windowInfo, &settings, nil)

	return wv, nil
}

func (f *WebViewFactory) newWebView(ctx context.Context) (*WebView, error) {
	id := port.WebViewID(f.nextID.Add(1))
	applicationScale := f.engine.currentApplicationScale()
	viewBridge := NewCef2gtkAdapter(f.engine.renderStackPlan, applicationScale)
	if viewBridge == nil {
		return nil, fmt.Errorf("create cef2gtk view")
	}

	wv := &WebView{
		id:                          id,
		ctx:                         ctx,
		engine:                      f.engine,
		factory:                     f,
		viewBridge:                  viewBridge,
		audioOutputFactory:          f.audioOutputFactory,
		adaptiveWindowlessFrameRate: f.adaptiveWindowlessFrameRate,
		windowlessFrameRate:         f.windowlessFrameRate,
		windowlessFrameRateMax:      f.windowlessFrameRateMax,
		inputConfig:                 f.inputConfig,
		backgroundColor:             f.bgColor.Load(),
	}
	wv.runOnGTKSync(func() {
		nativeWidget := viewBridge.Widget()
		popupSurface := newPopupBridgeSurface(ctx, nativeWidget, f.engine.renderStackPlan, applicationScale)
		wv.nativeWidget = nativeWidget
		if popupSurface != nil {
			wv.popupSurface = popupSurface
			if popupRoot := popupSurface.RootWidget(); popupRoot != nil {
				wv.nativeWidget = popupRoot
			}
		}
	})

	handlers := &handlerSet{wv: wv}
	wv.handlers = handlers
	wv.findCtrl = newFindController()

	// Build a CEF client backed by our handlerSet.
	// Store on WebView to prevent GC collection before CEF AddRef's it.
	wv.client = purecef.NewClient(wv.handlers)
	if opts := f.engine.cef2gtkProfileOptions(wv); opts.Enabled {
		if err := viewBridge.ConfigureProfiling(opts); err != nil {
			logging.FromContext(ctx).Warn().Err(err).Uint64("webview_id", uint64(id)).Msg("cef2gtk: failed to enable profiling")
		}
	}
	wv.installViewportSyncHooks()
	wv.startRenderStallWatchdog()
	return wv, nil
}

func (f *WebViewFactory) configureInitialBrowserCreation(
	ctx context.Context, wv *WebView, client purecef.RawClient,
	windowInfo *purecef.WindowInfo, settings *purecef.BrowserSettings,
	onFirstResize func(w, h int32),
) {
	wv.pendingCreate = &pendingBrowserCreate{
		windowInfo: windowInfo,
		client:     client,
		settings:   settings,
	}
	if onFirstResize == nil {
		onFirstResize = func(w, h int32) {
			logging.FromContext(ctx).Debug().Int32("w", w).Int32("h", h).Msg("cef: onFirstResize fired, scheduling browser creation")
			f.postPendingBrowserCreate(ctx, wv, w, h)
		}
	}
	if wv.viewBridge == nil {
		return
	}
	wv.runOnGTKSync(func() {
		wv.removeSizeObserver = wv.viewBridge.AddSizeObserver(func(w, h int32) {
			// Size observers run on the GTK thread. Prepare the GtkGLArea only for
			// the one-shot initial resize path; once that path has been consumed,
			// later size events must go through normal viewport sync even if popup
			// attachment keeps pendingCreate alive a little longer.
			if f.handleInitialBrowserCreateSizeObserver(ctx, wv, onFirstResize, wv.viewBridge.PrepareOnGTKThread, w, h) {
				return
			}

			synced := wv.syncResizeViewportOnGTK(ctx, "gtk-size-observer")
			logging.FromContext(ctx).Debug().
				Uint64("webview_id", uint64(wv.id)).
				Int32("resize_width", w).
				Int32("resize_height", h).
				Bool("browser_ready", synced).
				Msg("cef: viewport sync handled after GTK size change")
		})
	})
}

func (f *WebViewFactory) handleInitialBrowserCreateSizeObserver(
	ctx context.Context,
	wv *WebView,
	onFirstResize func(w, h int32),
	prepareOnGTK func() error,
	w, h int32,
) bool {
	if wv == nil || !wv.shouldStartBrowserCreateFromSizeObserver() {
		return false
	}
	if !initialBrowserCreateSizeReadyFromObserver(w, h) {
		if wv.awaitsNativePopupAttachment() && onFirstResize != nil {
			onFirstResize(w, h)
		}
		logging.FromContext(ctx).Debug().
			Uint64("webview_id", uint64(wv.id)).
			Int32("resize_width", w).
			Int32("resize_height", h).
			Msg("cef: ignoring bootstrap size observer event before real view size is ready")
		return true
	}
	if prepareOnGTK != nil {
		if err := prepareOnGTK(); err != nil {
			logging.FromContext(ctx).Warn().
				Err(err).
				Uint64("webview_id", uint64(wv.id)).
				Int32("resize_width", w).
				Int32("resize_height", h).
				Msg("cef: cef2gtk view not ready after resize, will retry")
			return true
		}
	}
	wv.markInitialBrowserCreateResizeHandled()
	if onFirstResize != nil {
		onFirstResize(w, h)
	}
	return true
}

func initialBrowserCreateSizeReadyFromObserver(width, height int32) bool {
	return observedViewportSizeReady(width, height, width, height)
}

func (f *WebViewFactory) postPendingBrowserCreate(ctx context.Context, wv *WebView, w, h int32) {
	log := logging.FromContext(ctx)
	pc := wv.takePendingCreate()
	if pc == nil {
		return
	}

	task := cefNewTask(cefTaskFunc(func() {
		if wv.destroyed.Load() {
			log.Debug().Msg("cef: skipping browser creation for destroyed webview")
			return
		}
		pendingURL := wv.pendingNavigationURI()
		// Always bootstrap OSR browsers on about:blank first. Starting directly on
		// the target URL can race host visibility/focus setup and strand the
		// renderer without an initial paint. We replay pending navigation from
		// OnAfterCreated once the host is fully wired.
		initialURL := "about:blank"
		result := cefBrowserHostCreateBrowser(
			pc.windowInfo,
			pc.client,
			initialURL,
			pc.settings,
			pc.extraInfo,
			nil, // requestContext
		)
		log.Debug().
			Str("initial_url", initialURL).
			Str("pending_url", pendingURL).
			Msg("cef: BrowserHostCreateBrowser initial URL")
		if f.engine != nil {
			f.engine.recordBrowserCreateRequest(w, h, result)
		}
		log.Debug().
			Int32("result", result).
			Int32("windowless", pc.windowInfo.WindowlessRenderingEnabled).
			Int32("windowless_frame_rate", pc.settings.WindowlessFrameRate).
			Int32("shared_texture", pc.windowInfo.SharedTextureEnabled).
			Int32("external_begin_frame", pc.windowInfo.ExternalBeginFrameEnabled).
			Bool("client_nil", pc.client == nil).
			Msg("cef: BrowserHostCreateBrowser call completed on CEF UI thread")
	}))

	postResult := cefPostTask(purecef.ThreadIDTidUi, task)
	if postResult != 1 {
		pc.postTaskRetries++
		wv.mu.Lock()
		wv.pendingCreate = pc
		wv.mu.Unlock()
		log.Error().
			Int32("result", postResult).
			Int("retry", pc.postTaskRetries).
			Msg("cef: failed to post initial browser creation to CEF UI thread")
		f.schedulePendingBrowserCreateRetry(ctx, wv, w, h, pc)
	}
}

func (f *WebViewFactory) schedulePendingBrowserCreateRetry(
	ctx context.Context,
	wv *WebView,
	w, h int32,
	pc *pendingBrowserCreate,
) {
	if f == nil || wv == nil || pc == nil || wv.destroyed.Load() {
		return
	}
	if pc.postTaskRetries > pendingBrowserCreateMaxPostRetries {
		logging.FromContext(ctx).Warn().
			Uint64("webview_id", uint64(wv.id)).
			Int("retries", pc.postTaskRetries).
			Int("max_retries", pendingBrowserCreateMaxPostRetries).
			Msg("cef: browser creation retries exhausted")
		return
	}
	cefScheduleAfter(pendingBrowserCreateRetryDelay, func() {
		if wv.destroyed.Load() {
			return
		}
		f.postPendingBrowserCreate(ctx, wv, w, h)
	})
}

func notifyBrowserResize(host resizeNotifiableBrowserHost) {
	if host == nil {
		return
	}
	host.WasResized()
	host.Invalidate(purecef.PaintElementTypePetView)
}

// CreateRelated creates a popup shell for a native CEF popup. The popup
// browser itself is created by CEF from OnBeforePopup and later bound to this
// shell in OnAfterCreated.
func (f *WebViewFactory) CreateRelated(ctx context.Context, parentID port.WebViewID) (port.WebView, error) {
	if f == nil || f.engine == nil {
		return nil, ErrRelatedWebViewUnsupported
	}

	parent := f.engine.lookupWebView(parentID)
	if parent == nil {
		return nil, fmt.Errorf("parent webview %d not found", parentID)
	}
	if parent.destroyed.Load() {
		return nil, fmt.Errorf("parent webview %d is destroyed", parentID)
	}

	popupWV, err := f.newWebView(ctx)
	if err != nil {
		return nil, err
	}
	popupWV.markNativePopupCandidate(parent)

	windowInfo := purecef.NewWindowInfo()
	cef2gtk.ConfigureWindowInfo(&windowInfo, cef2gtk.WindowInfoOptions{})
	if externalBeginFrameEnabled() {
		windowInfo.ExternalBeginFrameEnabled = 1
	}

	settings := purecef.NewBrowserSettings()
	cef2gtk.ConfigureBrowserSettings(&settings, cef2gtk.BrowserSettingsOptions{WindowlessFrameRate: f.windowlessFrameRate})
	settings.LocalStorage = 1 // CEF_STATE_ENABLED
	if bg := f.bgColor.Load(); bg != 0 {
		settings.BackgroundColor = bg
	}

	f.configureInitialBrowserCreation(ctx, popupWV, popupWV.client, &windowInfo, &settings, func(w, h int32) {
		f.handlePopupShellInitialResize(ctx, popupWV, func(w, h int32) {
			f.postPendingBrowserCreate(ctx, popupWV, w, h)
		}, w, h)
	})
	return popupWV, nil
}

func (f *WebViewFactory) handlePopupShellInitialResize(
	ctx context.Context,
	popupWV *WebView,
	postPendingCreate func(w, h int32),
	w, h int32,
) {
	if popupWV == nil {
		return
	}
	log := logging.FromContext(ctx)
	if popupWV.awaitsNativePopupAttachment() {
		log.Debug().Int32("w", w).Int32("h", h).Msg("cef: popup shell initial size observed, awaiting native popup attach")
		f.schedulePopupShellNativeFallback(ctx, popupWV)
		return
	}
	if popupWV.awaitsBrowserCreateFromNativePopupFallback() {
		log.Debug().Int32("w", w).Int32("h", h).Msg("cef: popup shell size ready after fallback, creating browser directly")
		if postPendingCreate != nil {
			postPendingCreate(w, h)
		}
		return
	}
	if !popupWV.preparePopupShellDirectBrowserCreation() {
		return
	}
	log.Debug().Int32("w", w).Int32("h", h).Msg("cef: popup shell first resize, creating browser directly")
	if postPendingCreate != nil {
		postPendingCreate(w, h)
	}
}

func (f *WebViewFactory) schedulePopupShellNativeFallback(ctx context.Context, popupWV *WebView) {
	if popupWV == nil {
		return
	}
	popupWV.scheduleNativePopupFallback(nativePopupAttachFallbackDelay, func() {
		popupWV.runOnGTK(func() {
			if !popupWV.startNativePopupFallback() {
				return
			}
			logging.FromContext(ctx).Warn().
				Uint64("webview_id", uint64(popupWV.id)).
				Msg("cef: native popup attach timed out, starting direct popup fallback")
			synced := popupWV.syncViewportNowOnGTK(ctx, "native-popup-fallback-timeout")
			logging.FromContext(ctx).Debug().
				Uint64("webview_id", uint64(popupWV.id)).
				Bool("browser_ready", synced).
				Msg("cef: popup fallback viewport sync requested")
		})
	})
}
