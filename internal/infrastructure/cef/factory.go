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
	engine *Engine
	gl     *glLoader
	nextID atomic.Uint64
	scale  int32
}

// newWebViewFactory returns a factory that will create WebViews using the
// given GL loader and HiDPI scale factor.
func newWebViewFactory(engine *Engine, gl *glLoader, scale int32) *WebViewFactory {
	if scale < 1 {
		scale = 1
	}
	return &WebViewFactory{
		engine: engine,
		gl:     gl,
		scale:  scale,
	}
}

// Create builds a new CEF off-screen browser WebView. The browser is created
// asynchronously; the returned WebView is usable immediately but navigation
// will fail with errNoBrowser until OnAfterCreated fires.
func (f *WebViewFactory) Create(ctx context.Context) (port.WebView, error) {
	id := port.WebViewID(f.nextID.Add(1))

	pipeline := newRenderPipeline(ctx, f.gl, f.scale)

	wv := &WebView{
		id:       id,
		ctx:      ctx,
		engine:   f.engine,
		pipeline: pipeline,
	}

	handlers := &handlerSet{wv: wv}
	wv.handlers = handlers

	input := newInputBridge(f.scale)
	input.attachTo(pipeline.glArea)
	wv.input = input

	// Build a CEF client backed by our handlerSet.
	// Store on WebView to prevent GC collection before CEF AddRef's it.
	client := purecef.NewClient(wv.handlers)
	wv.client = client

	// Configure WindowInfo for off-screen rendering (OSR).
	windowInfo := purecef.DefaultWindowInfo()
	windowInfo.WindowlessRenderingEnabled = 1

	// Configure BrowserSettings.
	settings := purecef.DefaultBrowserSettings()
	settings.WindowlessFrameRate = 60

	// Defer browser creation until the GL area has a non-zero size.
	// CEF requires GetViewRect to return a non-empty rect, but the GL area
	// is not yet realized at this point.
	wv.pendingCreate = &pendingBrowserCreate{
		windowInfo: &windowInfo,
		client:     client,
		settings:   &settings,
	}

	// When the GL area gets its first non-zero size, create the browser.
	pipeline.onFirstResize = func(w, h int32) {
		log := logging.FromContext(ctx)
		log.Debug().Int32("w", w).Int32("h", h).Msg("cef: onFirstResize fired, creating browser")
		if pc := wv.pendingCreate; pc != nil {
			wv.pendingCreate = nil
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
				Int32("shared_texture", pc.windowInfo.SharedTextureEnabled).
				Int32("external_begin_frame", pc.windowInfo.ExternalBeginFrameEnabled).
				Bool("client_nil", pc.client == nil).
				Msg("cef: BrowserHostCreateBrowser call completed")
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

	return wv, nil
}

// CreateRelated creates a WebView that shares session/cookies with the parent.
// TODO(phase2): look up the parent browser by parentID and use the same
// request context so cookies and session state are shared.
func (f *WebViewFactory) CreateRelated(ctx context.Context, _ port.WebViewID) (port.WebView, error) {
	logging.FromContext(ctx).Warn().Msg("cef: CreateRelated does not yet support session sharing, creating independent WebView")
	return f.Create(ctx)
}
