package cef

import (
	"context"
	"sync"
	"testing"
	"time"

	purecef "github.com/bnema/purego-cef/cef"
	cefmocks "github.com/bnema/purego-cef/cef/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/bnema/dumber/internal/application/port"
)

type recordingClipboardOrchestrator struct {
	mu             sync.Mutex
	selection      port.SelectionClipboardInput
	selectionCalls int
	explicit       port.ExplicitClipboardInput
}

func (r *recordingClipboardOrchestrator) HandleSelectionUpdate(_ context.Context, input port.SelectionClipboardInput) error {
	r.mu.Lock()
	r.selection = input
	r.selectionCalls++
	r.mu.Unlock()
	return nil
}

func (r *recordingClipboardOrchestrator) HandleExplicitCopy(_ context.Context, input port.ExplicitClipboardInput) error {
	r.explicit = input
	return nil
}

func newTestPipeline(w, h, s int32) *renderPipeline {
	rp := &renderPipeline{scale: s}
	rp.widthAtomic.Store(w)
	rp.heightAtomic.Store(h)
	return rp
}

type stubFrame struct {
	main bool
	url  string
}

func (f stubFrame) IsValid() bool                                                { return true }
func (f stubFrame) Undo()                                                        {}
func (f stubFrame) Redo()                                                        {}
func (f stubFrame) Cut()                                                         {}
func (f stubFrame) Copy()                                                        {}
func (f stubFrame) Paste()                                                       {}
func (f stubFrame) PasteAndMatchStyle()                                          {}
func (f stubFrame) Del()                                                         {}
func (f stubFrame) SelectAll()                                                   {}
func (f stubFrame) ViewSource()                                                  {}
func (f stubFrame) GetSource(purecef.StringVisitor)                              {}
func (f stubFrame) GetText(purecef.StringVisitor)                                {}
func (f stubFrame) LoadRequest(purecef.Request)                                  {}
func (f stubFrame) LoadURL(string)                                               {}
func (f stubFrame) ExecuteJavaScript(string, string, int32)                      {}
func (f stubFrame) IsMain() bool                                                 { return f.main }
func (f stubFrame) IsFocused() bool                                              { return false }
func (f stubFrame) GetName() string                                              { return "" }
func (f stubFrame) GetIdentifier() string                                        { return "" }
func (f stubFrame) GetParent() purecef.Frame                                     { return nil }
func (f stubFrame) GetURL() string                                               { return f.url }
func (f stubFrame) GetBrowser() purecef.Browser                                  { return nil }
func (f stubFrame) GetV8Context() purecef.V8Context                              { return nil }
func (f stubFrame) VisitDom(purecef.Domvisitor)                                  {}
func (f stubFrame) SendProcessMessage(purecef.ProcessID, purecef.ProcessMessage) {}
func (f stubFrame) CreateUrlrequest(purecef.Request, purecef.UrlrequestClient) purecef.Urlrequest {
	return nil
}

func TestOnLoadStartFiresCommittedAndUpdatesURI(t *testing.T) {
	wv := &WebView{ctx: context.Background()}
	var gotEvents []port.LoadEvent
	wv.SetCallbacks(&port.WebViewCallbacks{
		OnLoadChanged: func(event port.LoadEvent) {
			gotEvents = append(gotEvents, event)
		},
	})

	h := &handlerSet{wv: wv}
	h.OnLoadStart(nil, stubFrame{main: true, url: "https://google.com"}, 0)

	require.Len(t, gotEvents, 1)
	require.Equal(t, port.LoadCommitted, gotEvents[0])
	require.Equal(t, "https://google.com", wv.URI())
}

func TestOnLoadEndDoesNotDispatchBrowserLevelCompletion(t *testing.T) {
	wv := &WebView{ctx: context.Background()}
	var gotEvents []port.LoadEvent
	var gotProgress []float64
	wv.SetCallbacks(&port.WebViewCallbacks{
		OnLoadChanged: func(event port.LoadEvent) {
			gotEvents = append(gotEvents, event)
		},
		OnProgressChanged: func(progress float64) {
			gotProgress = append(gotProgress, progress)
		},
	})

	h := &handlerSet{wv: wv}
	h.OnLoadEnd(nil, stubFrame{main: true, url: "https://reddit.com"}, 200)

	assert.Empty(t, gotEvents)
	assert.Empty(t, gotProgress)
}

func TestOnTextSelectionChanged_ForwardsSelectionToClipboardOrchestrator(t *testing.T) {
	orchestrator := &recordingClipboardOrchestrator{}
	wv := &WebView{
		ctx: context.Background(),
		id:  42,
		engine: &Engine{
			clipboardTextOrchestrator: orchestrator,
		},
	}
	h := &handlerSet{wv: wv}

	h.OnTextSelectionChanged(nil, "selected text", nil)

	require.Eventually(t, func() bool {
		orchestrator.mu.Lock()
		defer orchestrator.mu.Unlock()
		return orchestrator.selectionCalls == 1 &&
			orchestrator.selection.Text == "selected text" &&
			orchestrator.selection.SourceEngine == port.ClipboardSourceCEF &&
			orchestrator.selection.ViewID == port.WebViewID(42)
	}, 2*time.Second, 10*time.Millisecond)
}

func TestOnTextSelectionChanged_DebouncesAndCollapsesRapidUpdates(t *testing.T) {
	orchestrator := &recordingClipboardOrchestrator{}
	wv := &WebView{
		ctx: context.Background(),
		id:  42,
		engine: &Engine{
			clipboardTextOrchestrator: orchestrator,
		},
	}
	h := &handlerSet{wv: wv}

	h.OnTextSelectionChanged(nil, "first selection", nil)
	h.OnTextSelectionChanged(nil, "second selection", nil)

	orchestrator.mu.Lock()
	gotCalls := orchestrator.selectionCalls
	orchestrator.mu.Unlock()
	require.Zero(t, gotCalls)

	require.Eventually(t, func() bool {
		orchestrator.mu.Lock()
		defer orchestrator.mu.Unlock()
		return orchestrator.selectionCalls == 1 && orchestrator.selection.Text == "second selection"
	}, 2*time.Second, 10*time.Millisecond)
	require.Equal(t, "second selection", wv.selectedTextSnapshot())
}

func TestOnTextSelectionChanged_SuppressesAutoCopyWhenFocusedNodeEditableAndResumesWhenCleared(t *testing.T) {
	orchestrator := &recordingClipboardOrchestrator{}
	wv := &WebView{
		ctx: context.Background(),
		id:  42,
		engine: &Engine{
			clipboardTextOrchestrator: orchestrator,
		},
	}
	h := &handlerSet{wv: wv}
	frame := cefmocks.NewMockFrame(t)
	oldFactory := newRendererBridgeProcessMessage
	t.Cleanup(func() { newRendererBridgeProcessMessage = oldFactory })

	newRendererBridgeProcessMessage = func(name string) purecef.ProcessMessage {
		return newTestBridgeProcessMessage(name, true)
	}
	frame.EXPECT().SendProcessMessage(purecef.ProcessIDPidBrowser, mock.Anything).Run(func(_ purecef.ProcessID, message purecef.ProcessMessage) {
		require.Equal(t, rendererBridgeMessageName, message.GetName())
		args := message.GetArgumentList()
		require.Equal(t, "editable_focus_changed", args.GetString(0))
		require.Equal(t, "1", args.GetString(1))
		h.OnProcessMessageReceived(nil, nil, 0, message)
	}).Once()
	(&rendererBridgeProcessHandler{}).OnFocusedNodeChanged(nil, frame, stubEditableDomnode{editable: true})

	h.OnTextSelectionChanged(nil, "editable selection", nil)
	require.Equal(t, "editable selection", wv.selectedTextSnapshot())

	orchestrator.mu.Lock()
	gotCalls := orchestrator.selectionCalls
	orchestrator.mu.Unlock()
	require.Zero(t, gotCalls)
	time.Sleep(350 * time.Millisecond)
	orchestrator.mu.Lock()
	gotCalls = orchestrator.selectionCalls
	orchestrator.mu.Unlock()
	require.Zero(t, gotCalls)

	newRendererBridgeProcessMessage = func(name string) purecef.ProcessMessage {
		return newTestBridgeProcessMessage(name, false)
	}
	frame.EXPECT().SendProcessMessage(purecef.ProcessIDPidBrowser, mock.Anything).Run(func(_ purecef.ProcessID, message purecef.ProcessMessage) {
		require.Equal(t, rendererBridgeMessageName, message.GetName())
		args := message.GetArgumentList()
		require.Equal(t, "editable_focus_changed", args.GetString(0))
		require.Equal(t, "0", args.GetString(1))
		h.OnProcessMessageReceived(nil, nil, 0, message)
	}).Once()
	(&rendererBridgeProcessHandler{}).OnFocusedNodeChanged(nil, frame, stubEditableDomnode{editable: false})

	h.OnTextSelectionChanged(nil, "free selection", nil)
	require.Equal(t, "free selection", wv.selectedTextSnapshot())
	require.Zero(t, orchestrator.selectionCalls)
	require.Eventually(t, func() bool {
		orchestrator.mu.Lock()
		defer orchestrator.mu.Unlock()
		return orchestrator.selectionCalls == 1 && orchestrator.selection.Text == "free selection"
	}, 2*time.Second, 10*time.Millisecond)
}

func TestOnTextSelectionChanged_DoesNotEmitLateDebouncedUpdateAfterDestroy(t *testing.T) {
	orchestrator := &recordingClipboardOrchestrator{}
	wv := &WebView{
		ctx: context.Background(),
		id:  42,
		engine: &Engine{
			clipboardTextOrchestrator: orchestrator,
		},
	}
	h := &handlerSet{wv: wv}

	h.OnTextSelectionChanged(nil, "selected text", nil)
	wv.Destroy()

	orchestrator.mu.Lock()
	gotCalls := orchestrator.selectionCalls
	orchestrator.mu.Unlock()
	require.Zero(t, gotCalls)
	time.Sleep(350 * time.Millisecond)
	orchestrator.mu.Lock()
	gotCalls = orchestrator.selectionCalls
	orchestrator.mu.Unlock()
	require.Zero(t, gotCalls)
	require.Equal(t, "selected text", wv.selectedTextSnapshot())
}

type stubEditableDomnode struct {
	editable bool
}

func (n stubEditableDomnode) GetType() purecef.DomNodeType                          { return 0 }
func (n stubEditableDomnode) IsText() bool                                          { return false }
func (n stubEditableDomnode) IsElement() bool                                       { return true }
func (n stubEditableDomnode) IsEditable() bool                                      { return n.editable }
func (n stubEditableDomnode) IsFormControlElement() bool                            { return false }
func (n stubEditableDomnode) GetFormControlElementType() purecef.DomFormControlType { return 0 }
func (n stubEditableDomnode) IsSame(that purecef.Domnode) bool                      { return n == that }
func (n stubEditableDomnode) GetName() string                                       { return "" }
func (n stubEditableDomnode) GetValue() string                                      { return "" }
func (n stubEditableDomnode) SetValue(string) int32                                 { return 0 }
func (n stubEditableDomnode) GetAsMarkup() string                                   { return "" }
func (n stubEditableDomnode) GetDocument() purecef.Domdocument                      { return nil }
func (n stubEditableDomnode) GetParent() purecef.Domnode                            { return nil }
func (n stubEditableDomnode) GetPreviousSibling() purecef.Domnode                   { return nil }
func (n stubEditableDomnode) GetNextSibling() purecef.Domnode                       { return nil }
func (n stubEditableDomnode) HasChildren() bool                                     { return false }
func (n stubEditableDomnode) GetFirstChild() purecef.Domnode                        { return nil }
func (n stubEditableDomnode) GetLastChild() purecef.Domnode                         { return nil }
func (n stubEditableDomnode) GetElementTagName() string                             { return "" }
func (n stubEditableDomnode) HasElementAttributes() bool                            { return false }
func (n stubEditableDomnode) HasElementAttribute(string) bool                       { return false }
func (n stubEditableDomnode) GetElementAttribute(string) string                     { return "" }
func (n stubEditableDomnode) GetElementAttributes(uintptr)                          {}
func (n stubEditableDomnode) SetElementAttribute(string, string) int32              { return 0 }
func (n stubEditableDomnode) GetElementInnerText() string                           { return "" }
func (n stubEditableDomnode) GetElementBounds() uintptr                             { return 0 }

func TestGetViewRectUsesDIPCoordinates(t *testing.T) {
	rect := &purecef.Rect{}
	h := &handlerSet{
		wv: &WebView{
			ctx:      context.Background(),
			pipeline: newTestPipeline(800, 600, 2),
		},
	}

	h.GetViewRect(nil, rect)

	require.Equal(t, int32(0), rect.X)
	require.Equal(t, int32(0), rect.Y)
	require.Equal(t, int32(400), rect.Width)
	require.Equal(t, int32(300), rect.Height)
}

func TestGetScreenInfoUsesDIPRectAndScale(t *testing.T) {
	si := purecef.NewScreenInfo()
	info := &si
	h := &handlerSet{
		wv: &WebView{
			ctx:      context.Background(),
			pipeline: newTestPipeline(1500, 900, 3),
		},
	}

	ok := h.GetScreenInfo(nil, info)

	require.Equal(t, int32(1), ok)
	require.InEpsilon(t, float32(3), info.DeviceScaleFactor, 0.001)
	require.Equal(t, int32(500), info.Rect.Width)
	require.Equal(t, int32(300), info.Rect.Height)
	require.Equal(t, int32(500), info.AvailableRect.Width)
	require.Equal(t, int32(300), info.AvailableRect.Height)
}

func TestOptionalHandlersAreAlwaysEnabled(t *testing.T) {
	h := &handlerSet{}

	// AudioHandler is always enabled (required for media decoding).
	require.Same(t, h, h.GetAudioHandler())
	require.Same(t, h, h.GetContextMenuHandler())
}

func TestOnPaint_DropsStaleMainViewPaint(t *testing.T) {
	wv := &WebView{
		id: 42,
		pipeline: &renderPipeline{
			ctx:   context.Background(),
			scale: 1,
		},
	}
	wv.pipeline.widthAtomic.Store(1269)
	wv.pipeline.heightAtomic.Store(1035)

	h := &handlerSet{wv: wv}
	buffer := make([]byte, 1269*2106*4)

	h.OnPaint(nil, purecef.PaintElementTypePetView, []purecef.Rect{{X: 0, Y: 0, Width: 1269, Height: 2106}}, buffer, 1269, 2106)

	require.Equal(t, int32(1269), wv.pipeline.widthAtomic.Load())
	require.Equal(t, int32(1035), wv.pipeline.heightAtomic.Load())
	require.Nil(t, wv.pipeline.staging)
	require.Zero(t, wv.pipeline.lastQueuedPaintSeq.Load())
	require.False(t, wv.pipeline.needsUpload)
	require.Empty(t, wv.pipeline.dirtyRects)
}
