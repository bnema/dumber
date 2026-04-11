package cef

import (
	"context"
	"sync/atomic"

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
	engine                   *Engine
	gl                       *glLoader
	nextID                   atomic.Uint64
	scale                    int32
	windowlessFrameRate      int32
	enableContextMenuHandler bool
	bgColor                  atomic.Uint32 // packed ARGB for BrowserSettings.BackgroundColor
	transcoder               port.MediaTranscoder
	mediaClassifier          MediaClassifier
	audioOutputFactory       port.AudioOutputFactory
}

type webViewFactoryOptions struct {
	scale                    int32
	windowlessFrameRate      int32
	enableContextMenuHandler bool
	transcoder               port.MediaTranscoder
	mediaClassifier          MediaClassifier
	audioOutputFactory       port.AudioOutputFactory
}

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
		engine:                   engine,
		gl:                       gl,
		scale:                    opts.scale,
		windowlessFrameRate:      opts.windowlessFrameRate,
		enableContextMenuHandler: opts.enableContextMenuHandler,
		transcoder:               opts.transcoder,
		mediaClassifier:          opts.mediaClassifier,
		audioOutputFactory:       opts.audioOutputFactory,
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
	id := port.WebViewID(f.nextID.Add(1))

	pipeline := newRenderPipeline(ctx, f.gl, f.scale)

	wv := &WebView{
		id:                 id,
		ctx:                ctx,
		engine:             f.engine,
		pipeline:           pipeline,
		audioOutputFactory: f.audioOutputFactory,
	}

	var transcodingHandler purecef.ResourceRequestHandler
	if f.transcoder != nil {
		transcodingHandler = newTranscodingRequestHandler(f.transcoder, f.mediaClassifier, func() context.Context {
			if f.engine != nil {
				return f.engine.currentContext()
			}
			return ctx
		})
	}

	handlers := &handlerSet{
		wv:                       wv,
		enableContextMenuHandler: f.enableContextMenuHandler,
		transcodingHandler:       transcodingHandler,
	}
	wv.handlers = handlers
	wv.findCtrl = newFindController()

	input := newInputBridge(ctx, f.scale)
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
	client := purecef.NewClient(wv.handlers)
	wv.client = client

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
	f.configureInitialBrowserCreation(ctx, wv, pipeline, client, &windowInfo, &settings)

	return wv, nil
}

func (f *WebViewFactory) configureInitialBrowserCreation(
	ctx context.Context, wv *WebView, pipeline *renderPipeline, client purecef.Client,
	windowInfo *purecef.WindowInfo, settings *purecef.BrowserSettings,
) {
	wv.pendingCreate = &pendingBrowserCreate{
		windowInfo: windowInfo,
		client:     client,
		settings:   settings,
	}

	pipeline.onFirstResize = func(w, h int32) {
		log := logging.FromContext(ctx)
		log.Debug().Int32("w", w).Int32("h", h).Msg("cef: onFirstResize fired, scheduling browser creation")
		pc := wv.takePendingCreate()
		if pc == nil {
			return
		}

		task := purecef.NewTask(cefTaskFunc(func() {
			if wv.destroyed.Load() {
				log.Debug().Msg("cef: skipping browser creation for destroyed webview")
				return
			}
			result := purecef.BrowserHostCreateBrowser(
				pc.windowInfo,
				pc.client,
				"about:blank",
				pc.settings,
				nil, // extraInfo
				nil, // requestContext
			)
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

		postResult := purecef.PostTask(purecef.ThreadIDTidUi, task)
		if postResult != 1 {
			wv.mu.Lock()
			if wv.pendingCreate == nil {
				wv.pendingCreate = pc
			}
			wv.mu.Unlock()
			log.Error().
				Int32("result", postResult).
				Msg("cef: failed to post initial browser creation to CEF UI thread")
		}
	}

	// On subsequent resizes, notify CEF so it re-queries GetViewRect.
	pipeline.onResizeCB = func(_, _ int32) {
		wv.mu.RLock()
		host := wv.host
		wv.mu.RUnlock()
		if host != nil {
			host.WasResized()
		}
	}
}

// CreateRelated creates a WebView that shares session/cookies with the parent.
// TODO(phase2): look up the parent browser by parentID and use the same
// request context so cookies and session state are shared.
func (f *WebViewFactory) CreateRelated(_ context.Context, _ port.WebViewID) (port.WebView, error) {
	return nil, ErrRelatedWebViewUnsupported
}
