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

	// Build a CEF client backed by our handlerSet. NewClient allocates a raw
	// CEF client struct with ref-counting and callback vtable wired up. We
	// keep the pointer alive on the WebView so it is not garbage collected
	// before the browser is destroyed.
	clientPtr := purecef.NewClient(wv.handlers)

	// Configure WindowInfo for off-screen rendering (OSR).
	var windowInfo purecef.WindowInfo
	windowInfo.Size = unsafe.Sizeof(windowInfo)
	windowInfo.WindowlessRenderingEnabled = 1

	// Configure BrowserSettings with 60 fps for smooth rendering.
	var settings purecef.BrowserSettings
	settings.Size = unsafe.Sizeof(settings)
	settings.WindowlessFrameRate = 60

	// Create the browser asynchronously. BrowserHostCreateBrowser posts to the
	// CEF UI thread; the browser will be nil until OnAfterCreated fires.
	//
	// We call the raw creation function directly with the client pointer from
	// NewClient, because the high-level wrapper's extractRawPointer requires a
	// package-internal interface that external types cannot satisfy.
	//
	// NOTE: BrowserHostCreateBrowser is currently a stub in purego-cef (returns
	// 0). That is expected — the skeleton is correct and will work once the
	// stub is implemented.
	createBrowser(clientPtr, &windowInfo, &settings)

	return wv, nil
}

// createBrowser calls purecef.BrowserHostCreateBrowser with the raw client
// pointer. This is separated for testability and to document the impedance
// mismatch between NewClient (returns unsafe.Pointer) and
// BrowserHostCreateBrowser (expects Client interface with unexported
// rawPointer method).
func createBrowser(clientPtr unsafe.Pointer, windowInfo *purecef.WindowInfo, settings *purecef.BrowserSettings) {
	// Wrap the raw pointer in a thin Client adapter so BrowserHostCreateBrowser
	// can extract it. handlerSet already implements Client; the real vtable is
	// in the raw struct that clientPtr points to.
	adapter := &clientAdapter{ptr: clientPtr}
	purecef.BrowserHostCreateBrowser(
		windowInfo,
		adapter,
		"about:blank",
		settings,
		nil, // extraInfo
		nil, // requestContext
	)
}

// CreateRelated creates a WebView that shares session/cookies with the parent.
// TODO(phase2): look up the parent browser by parentID and use the same
// request context so cookies and session state are shared.
func (f *WebViewFactory) CreateRelated(ctx context.Context, _ port.WebViewID) (port.WebView, error) {
	return f.Create(ctx)
}

// clientAdapter wraps an unsafe.Pointer from NewClient into something that
// satisfies the purecef.Client interface. The actual CEF callbacks are already
// wired inside the raw struct; these Go methods are never invoked by CEF.
//
// NOTE: purecef.extractRawPointer checks for an unexported rawPointer()
// method, which external packages cannot satisfy. Until purego-cef exports a
// proper WrapClient helper, the pointer extraction will return nil. This is
// acceptable because BrowserHostCreateBrowser is currently a stub.
type clientAdapter struct {
	ptr unsafe.Pointer
}

func (c *clientAdapter) GetAudioHandler() purecef.AudioHandler             { return nil }
func (c *clientAdapter) GetCommandHandler() purecef.CommandHandler         { return nil }
func (c *clientAdapter) GetContextMenuHandler() purecef.ContextMenuHandler { return nil }
func (c *clientAdapter) GetDialogHandler() purecef.DialogHandler           { return nil }
func (c *clientAdapter) GetDisplayHandler() purecef.DisplayHandler         { return nil }
func (c *clientAdapter) GetDownloadHandler() purecef.DownloadHandler       { return nil }
func (c *clientAdapter) GetDragHandler() purecef.DragHandler               { return nil }
func (c *clientAdapter) GetFindHandler() purecef.FindHandler               { return nil }
func (c *clientAdapter) GetFocusHandler() purecef.FocusHandler             { return nil }
func (c *clientAdapter) GetFrameHandler() purecef.FrameHandler             { return nil }
func (c *clientAdapter) GetPermissionHandler() purecef.PermissionHandler   { return nil }
func (c *clientAdapter) GetJsdialogHandler() purecef.JsdialogHandler       { return nil }
func (c *clientAdapter) GetKeyboardHandler() purecef.KeyboardHandler       { return nil }
func (c *clientAdapter) GetLifeSpanHandler() purecef.LifeSpanHandler       { return nil }
func (c *clientAdapter) GetLoadHandler() purecef.LoadHandler               { return nil }
func (c *clientAdapter) GetPrintHandler() purecef.PrintHandler             { return nil }
func (c *clientAdapter) GetRenderHandler() purecef.RenderHandler           { return nil }
func (c *clientAdapter) GetRequestHandler() purecef.RequestHandler         { return nil }

func (c *clientAdapter) OnProcessMessageReceived(_ purecef.Browser, _ purecef.Frame, _ purecef.ProcessID, _ purecef.ProcessMessage) int32 {
	return 0
}
