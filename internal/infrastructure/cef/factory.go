package cef

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	purecef "github.com/bnema/purego-cef/cef"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/logging"
)

// Compile-time interface check.
var _ port.WebViewFactory = (*WebViewFactory)(nil)

// WebViewFactory creates new CEF off-screen browser WebViews. Each WebView
// gets a unique ID, its own renderPipeline and inputBridge, and an
// asynchronously-created CEF browser (via BrowserHostCreateBrowser).
type WebViewFactory struct {
	engine              *Engine
	gl                  *glLoader
	nextID              atomic.Uint64
	scale               int32
	windowlessFrameRate int32
	bgColor             atomic.Uint32 // packed ARGB for BrowserSettings.BackgroundColor
	audioOutputFactory  port.AudioOutputFactory
}

type webViewFactoryOptions struct {
	scale               int32
	windowlessFrameRate int32
	audioOutputFactory  port.AudioOutputFactory
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

// newWebViewFactory returns a factory that will create WebViews using the
// given GL loader and HiDPI scale factor.
func newWebViewFactory(engine *Engine, gl *glLoader, opts webViewFactoryOptions) *WebViewFactory {
	if opts.scale < 1 {
		opts.scale = 1
	}
	if opts.windowlessFrameRate < 1 {
		opts.windowlessFrameRate = 60
	}
	return &WebViewFactory{
		engine:              engine,
		gl:                  gl,
		scale:               opts.scale,
		windowlessFrameRate: opts.windowlessFrameRate,
		audioOutputFactory: opts.audioOutputFactory,
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
	wv := f.newWebView(ctx)

	// Configure WindowInfo for off-screen rendering (OSR).
	windowInfo := purecef.NewWindowInfo()
	windowInfo.WindowlessRenderingEnabled = 1
	if externalBeginFrameEnabled() {
		windowInfo.ExternalBeginFrameEnabled = 1
	}

	// Configure BrowserSettings.
	settings := purecef.NewBrowserSettings()
	settings.WindowlessFrameRate = f.windowlessFrameRate
	settings.LocalStorage = 1 // CEF_STATE_ENABLED
	if bg := f.bgColor.Load(); bg != 0 {
		settings.BackgroundColor = bg
	}

	// Defer browser creation until the GL area has a non-zero size.
	// CEF requires GetViewRect to return a non-empty rect, but the GL area
	// is not yet realized at this point.
	f.configureInitialBrowserCreation(ctx, wv, wv.pipeline, wv.client, &windowInfo, &settings)

	return wv, nil
}

func (f *WebViewFactory) newWebView(ctx context.Context) *WebView {
	id := port.WebViewID(f.nextID.Add(1))
	pipeline := newRenderPipeline(ctx, f.gl, f.scale, id)

	wv := &WebView{
		id:                  id,
		ctx:                 ctx,
		engine:              f.engine,
		pipeline:            pipeline,
		audioOutputFactory:  f.audioOutputFactory,
		resizeReconciler:    newResizeReconciler(ctx, id),
		windowlessFrameRate: f.windowlessFrameRate,
		backgroundColor:     f.bgColor.Load(),
	}

	handlers := &handlerSet{
		wv: wv,
	}
	wv.handlers = handlers
	wv.findCtrl = newFindController()

	input := newInputBridge(ctx, f.scale)
	input.selectionText = wv.selectedTextSnapshot
	input.explicitCopyText = func(action, text string) {
		if f.engine != nil {
			f.engine.handleExplicitClipboardBridgeText(wv.id, action, text)
		}
	}
	input.attachTo(pipeline.glArea)
	wv.input = input

	// Wire middle-click → new tab using the cached hover URI.
	input.onMiddleClick = func(_ string) {
		wv.mu.RLock()
		hoverURI := wv.lastHoverURI
		cb := wv.callbacks
		wv.mu.RUnlock()
		if hoverURI == "" || cb == nil || cb.OnLinkMiddleClick == nil {
			return
		}
		uri := hoverURI
		wv.runOnGTK(func() {
			cb.OnLinkMiddleClick(uri)
		})
	}

	// Build a CEF client backed by our handlerSet.
	// Store on WebView to prevent GC collection before CEF AddRef's it.
	wv.client = purecef.NewClient(wv.handlers)
	return wv
}

func (f *WebViewFactory) configureInitialBrowserCreation(
	ctx context.Context, wv *WebView, pipeline *renderPipeline, client purecef.RawClient,
	windowInfo *purecef.WindowInfo, settings *purecef.BrowserSettings,
) {
	wv.pendingCreate = &pendingBrowserCreate{
		windowInfo: windowInfo,
		client:     client,
		settings:   settings,
	}

	pipeline.onFirstResize = func(w, h int32) {
		logging.FromContext(ctx).Debug().Int32("w", w).Int32("h", h).Msg("cef: onFirstResize fired, scheduling browser creation")
		f.postPendingBrowserCreate(ctx, wv, w, h)
	}

	// On subsequent resizes, notify CEF so it re-queries GetViewRect.
	pipeline.onResizeCB = func(w, h int32) {
		log := logging.FromContext(ctx)
		wv.mu.RLock()
		host := wv.host
		wv.mu.RUnlock()
		if host == nil {
			log.Debug().
				Uint64("webview_id", uint64(wv.id)).
				Int32("resize_width", w).
				Int32("resize_height", h).
				Bool("host_nil", true).
				Bool("browser_ready", false).
				Msg("cef: resize observed before browser host existed")
			return
		}
		log.Debug().
			Uint64("webview_id", uint64(wv.id)).
			Int32("resize_width", w).
			Int32("resize_height", h).
			Bool("host_nil", false).
			Bool("browser_ready", true).
			Msg("cef: browser resize observed before WasResized")
		notifyBrowserResize(host)
		log.Debug().
			Uint64("webview_id", uint64(wv.id)).
			Int32("resize_width", w).
			Int32("resize_height", h).
			Bool("host_nil", false).
			Bool("browser_ready", true).
			Msg("cef: browser host WasResized invoked")
		if wv.resizeReconciler != nil {
			resizeSeq, _ := wv.pipeline.latestResizeDiagnostics()
			wv.resizeReconciler.start(
				resizeSeq,
				func() resizeNotifiableBrowserHost {
					wv.mu.RLock()
					defer wv.mu.RUnlock()
					return wv.host
				},
				func() bool { return wv.destroyed.Load() },
			)
		}
	}
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

	popupWV := f.newWebView(ctx)
	popupWV.markNativePopupCandidate(parent)

	windowInfo := purecef.NewWindowInfo()
	windowInfo.WindowlessRenderingEnabled = 1
	if externalBeginFrameEnabled() {
		windowInfo.ExternalBeginFrameEnabled = 1
	}

	settings := purecef.NewBrowserSettings()
	settings.WindowlessFrameRate = f.windowlessFrameRate
	settings.LocalStorage = 1 // CEF_STATE_ENABLED
	if bg := f.bgColor.Load(); bg != 0 {
		settings.BackgroundColor = bg
	}

	f.configureInitialBrowserCreation(ctx, popupWV, popupWV.pipeline, popupWV.client, &windowInfo, &settings)
	popupWV.pipeline.onFirstResize = func(w, h int32) {
		log := logging.FromContext(ctx)
		if popupWV.awaitsNativePopupAttachment() {
			log.Debug().Int32("w", w).Int32("h", h).Msg("cef: popup shell first resize, awaiting native popup attach")
			popupWV.scheduleNativePopupFallback(nativePopupAttachFallbackDelay, func() {
				if !popupWV.startNativePopupFallback() {
					return
				}
				logging.FromContext(ctx).Warn().
					Uint64("webview_id", uint64(popupWV.id)).
					Msg("cef: native popup attach timed out, creating popup browser directly")
				f.postPendingBrowserCreate(ctx, popupWV, w, h)
			})
			return
		}

		if !popupWV.preparePopupShellDirectBrowserCreation() {
			return
		}
		log.Debug().Int32("w", w).Int32("h", h).Msg("cef: popup shell first resize, creating browser directly")
		f.postPendingBrowserCreate(ctx, popupWV, w, h)
	}
	return popupWV, nil
}
