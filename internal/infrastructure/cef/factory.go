package cef

import (
	"context"
	"sync/atomic"
	"unsafe"

	purecef "github.com/bnema/purego-cef/cef"

	"github.com/bnema/dumber/internal/application/port"
)

// Compile-time interface check.
var _ port.WebViewFactory = (*WebViewFactory)(nil)

// WebViewFactory creates new CEF off-screen browser WebViews. Each WebView
// gets a unique ID, its own renderPipeline and inputBridge, and an
// asynchronously-created CEF browser (via BrowserHostCreateBrowser).
type WebViewFactory struct {
	gl     *glLoader
	nextID atomic.Uint64
	scale  int32
}

// newWebViewFactory returns a factory that will create WebViews using the
// given GL loader and HiDPI scale factor.
func newWebViewFactory(gl *glLoader, scale int32) *WebViewFactory {
	if scale < 1 {
		scale = 1
	}
	return &WebViewFactory{
		gl:    gl,
		scale: scale,
	}
}

// Create builds a new CEF off-screen browser WebView. The browser is created
// asynchronously; the returned WebView is usable immediately but navigation
// will fail with errNoBrowser until OnAfterCreated fires.
func (f *WebViewFactory) Create(_ context.Context) (port.WebView, error) {
	id := port.WebViewID(f.nextID.Add(1))

	pipeline := newRenderPipeline(f.gl, f.scale)

	wv := &WebView{
		id:       id,
		pipeline: pipeline,
	}

	handlers := &handlerSet{wv: wv}
	wv.handlers = handlers

	input := newInputBridge(f.scale)
	input.attachTo(pipeline.glArea)
	wv.input = input

	// Build a CEF client backed by our handlerSet. NewClient returns a Client
	// that wraps the raw CEF struct with refcounting and callback vtable.
	client := purecef.NewClient(wv.handlers)

	// Configure WindowInfo for off-screen rendering (OSR).
	var windowInfo purecef.WindowInfo
	windowInfo.Size = unsafe.Sizeof(windowInfo)
	windowInfo.WindowlessRenderingEnabled = 1

	// Configure BrowserSettings with 60 fps for smooth rendering.
	var settings purecef.BrowserSettings
	settings.Size = unsafe.Sizeof(settings)
	settings.WindowlessFrameRate = 60

	// Create the browser asynchronously. The browser will be nil until
	// OnAfterCreated fires in the LifeSpanHandler.
	purecef.BrowserHostCreateBrowser(
		&windowInfo,
		client,
		"about:blank",
		&settings,
		nil, // extraInfo
		nil, // requestContext
	)

	return wv, nil
}

// CreateRelated creates a WebView that shares session/cookies with the parent.
// TODO(phase2): look up the parent browser by parentID and use the same
// request context so cookies and session state are shared.
func (f *WebViewFactory) CreateRelated(ctx context.Context, _ port.WebViewID) (port.WebView, error) {
	return f.Create(ctx)
}
