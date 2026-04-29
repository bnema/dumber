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
	engine              *Engine
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
// given HiDPI scale factor.
func newWebViewFactory(engine *Engine, opts webViewFactoryOptions) *WebViewFactory {
	if opts.scale < 1 {
		opts.scale = 1
	}
	if opts.windowlessFrameRate < 1 {
		opts.windowlessFrameRate = 60
	}
	return &WebViewFactory{
		engine:              engine,
		scale:               opts.scale,
		windowlessFrameRate: opts.windowlessFrameRate,
		audioOutputFactory:  opts.audioOutputFactory,
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
	settings.WindowlessFrameRate = f.windowlessFrameRate
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
	viewBridge := NewCef2gtkAdapter()
	if viewBridge == nil {
		return nil, fmt.Errorf("create cef2gtk view")
	}

	wv := &WebView{
		id:                  id,
		ctx:                 ctx,
		engine:              f.engine,
		viewBridge:          viewBridge,
		audioOutputFactory:  f.audioOutputFactory,
		windowlessFrameRate: f.windowlessFrameRate,
		backgroundColor:     f.bgColor.Load(),
	}

	handlers := &handlerSet{wv: wv}
	wv.handlers = handlers
	wv.findCtrl = newFindController()

	var prepareErr error
	wv.runOnGTKSync(func() {
		prepareErr = viewBridge.PrepareOnGTKThread()
	})
	if prepareErr != nil {
		if ctx != nil {
			logging.FromContext(ctx).Error().Err(prepareErr).Uint64("webview_id", uint64(id)).Msg("cef: failed to prepare cef2gtk view")
		}
		return nil, fmt.Errorf("prepare cef2gtk view: %w", prepareErr)
	}

	// Build a CEF client backed by our handlerSet.
	// Store on WebView to prevent GC collection before CEF AddRef's it.
	wv.client = purecef.NewClient(wv.handlers)
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
	firstResize := true
	wv.runOnGTKSync(func() {
		wv.removeSizeObserver = wv.viewBridge.AddSizeObserver(func(w, h int32) {
			if firstResize {
				firstResize = false
				onFirstResize(w, h)
				return
			}

			// Size observers run on the GTK thread; the captured firstResize state is
			// therefore single-threaded.
			wv.mu.RLock()
			host := wv.host
			wv.mu.RUnlock()
			if host == nil {
				logging.FromContext(ctx).Debug().
					Uint64("webview_id", uint64(wv.id)).
					Int32("resize_width", w).
					Int32("resize_height", h).
					Bool("host_nil", true).
					Bool("browser_ready", false).
					Msg("cef: resize observed before browser host existed")
				return
			}
			notifyBrowserResize(host)
			logging.FromContext(ctx).Debug().
				Uint64("webview_id", uint64(wv.id)).
				Int32("resize_width", w).
				Int32("resize_height", h).
				Bool("host_nil", false).
				Bool("browser_ready", true).
				Msg("cef: browser host WasResized invoked")
		})
	})
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
	settings.WindowlessFrameRate = f.windowlessFrameRate
	settings.LocalStorage = 1 // CEF_STATE_ENABLED
	if bg := f.bgColor.Load(); bg != 0 {
		settings.BackgroundColor = bg
	}

	f.configureInitialBrowserCreation(ctx, popupWV, popupWV.client, &windowInfo, &settings, func(w, h int32) {
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
	})
	return popupWV, nil
}
