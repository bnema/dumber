package cef

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	purecef "github.com/bnema/purego-cef/cef"
	cefmocks "github.com/bnema/purego-cef/cef/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/bnema/dumber/internal/application/dto"
	"github.com/bnema/dumber/internal/application/port"
	portmocks "github.com/bnema/dumber/internal/application/port/mocks"
)

type clipboardOrchestratorRecorder struct {
	mu             sync.Mutex
	selection      dto.SelectionClipboardInput
	selectionCalls int
}

type stubDownloadPreparer struct{}

func (stubDownloadPreparer) Execute(context.Context, port.DownloadPrepareInput) *port.DownloadPrepareOutput {
	return &port.DownloadPrepareOutput{}
}

type controlledSelectionTimer struct {
	stopped bool
}

func (t *controlledSelectionTimer) Stop() bool {
	alreadyStopped := t.stopped
	t.stopped = true
	return !alreadyStopped
}

type controlledSelectionScheduler struct {
	timers    []*controlledSelectionTimer
	callbacks []func()
}

func (s *controlledSelectionScheduler) schedule(_ time.Duration, fn func()) stoppableTimer {
	timer := &controlledSelectionTimer{}
	s.timers = append(s.timers, timer)
	s.callbacks = append(s.callbacks, fn)
	return timer
}

func (s *controlledSelectionScheduler) fire(index int) {
	if index < 0 || index >= len(s.callbacks) {
		return
	}
	if !s.timers[index].stopped {
		s.callbacks[index]()
	}
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

type stubStoppableTimer struct{}

func (stubStoppableTimer) Stop() bool { return true }

type spyStoppableTimer struct{ stopped bool }

func (t *spyStoppableTimer) Stop() bool {
	t.stopped = true
	return true
}

func TestPreparePaneHostedBrowsingContext_ClearsNativePopupFallbackState(t *testing.T) {
	parent := &WebView{ctx: context.Background()}
	timer := &spyStoppableTimer{}
	wv := &WebView{
		nativePopupCandidate:       true,
		nativePopupParent:          parent,
		nativePopupID:              77,
		nativePopupFallbackStarted: true,
		nativePopupFallbackTimer:   timer,
		pendingCreate:              &pendingBrowserCreate{},
		popupOpenerBridgeParent:    parent,
		popupOpenerBridgeParentURI: "https://example.com/login",
	}

	wv.PreparePaneHostedBrowsingContext()

	require.False(t, wv.nativePopupCandidate)
	require.Nil(t, wv.nativePopupParent)
	require.Zero(t, wv.nativePopupID)
	require.False(t, wv.nativePopupFallbackStarted)
	require.Nil(t, wv.nativePopupFallbackTimer)
	require.True(t, timer.stopped)
	require.Nil(t, wv.popupOpenerBridgeParent)
	require.Empty(t, wv.popupOpenerBridgeParentURI)
}

func TestStartNativePopupFallback_AllowsPopupShellAwaitingArm(t *testing.T) {
	wv := &WebView{
		nativePopupCandidate: true,
		pendingCreate:        &pendingBrowserCreate{},
	}

	require.True(t, wv.startNativePopupFallback())
	require.False(t, wv.isNativePopupCandidate())
	require.True(t, wv.nativePopupFallbackStarted)
}

func TestPreparePopupShellDirectBrowserCreation_EnablesSyntheticOpenerBridge(t *testing.T) {
	parent := &WebView{ctx: context.Background()}
	parent.updateURI("https://example.com/login")
	wv := &WebView{
		pendingCreate:     &pendingBrowserCreate{},
		nativePopupParent: parent,
	}

	require.True(t, wv.preparePopupShellDirectBrowserCreation())
	require.Same(t, parent, wv.popupOpenerBridgeParent)
	require.Equal(t, "https://example.com/login", wv.popupOpenerBridgeParentURI)
}

func TestPreparePopupShellDirectBrowserCreation_SkipsDestroyedParentOpenerBridge(t *testing.T) {
	parent := &WebView{ctx: context.Background()}
	parent.updateURI("https://example.com/login")
	parent.destroyed.Store(true)
	wv := &WebView{
		pendingCreate:     &pendingBrowserCreate{},
		nativePopupParent: parent,
	}

	require.True(t, wv.preparePopupShellDirectBrowserCreation())
	require.Nil(t, wv.popupOpenerBridgeParent)
	require.Empty(t, wv.popupOpenerBridgeParentURI)
}

func TestScheduleNativePopupFallback_SkipsDestroyedWebView(t *testing.T) {
	called := false
	wv := &WebView{}
	wv.destroyed.Store(true)
	wv.nativePopupFallbackSchedule = func(time.Duration, func()) stoppableTimer {
		called = true
		return stubStoppableTimer{}
	}

	wv.scheduleNativePopupFallback(time.Millisecond, func() {
		called = true
	})

	require.False(t, called)
}

func TestScheduleNativePopupFallback_DoesNotFireAfterDestroy(t *testing.T) {
	called := false
	var scheduled func()
	wv := &WebView{}
	wv.nativePopupFallbackSchedule = func(_ time.Duration, fn func()) stoppableTimer {
		scheduled = fn
		return stubStoppableTimer{}
	}

	wv.scheduleNativePopupFallback(time.Millisecond, func() {
		called = true
	})
	require.NotNil(t, scheduled)

	wv.destroyed.Store(true)
	scheduled()

	require.False(t, called)
}

func TestStartNativePopupFallback_RejectsDestroyedWebView(t *testing.T) {
	wv := &WebView{
		nativePopupCandidate: true,
		pendingCreate:        &pendingBrowserCreate{},
	}
	wv.destroyed.Store(true)

	require.False(t, wv.startNativePopupFallback())
}

func TestNativePopupActivationDoesNotLetInitialBlankLoadClearPendingNavigationBeforeReplay(t *testing.T) {
	parent := &WebView{}
	wv := &WebView{
		ctx:                  context.Background(),
		nativePopupCandidate: true,
		nativePopupParent:    parent,
		client:               cefmocks.NewMockRawClient(t),
		pendingCreate:        &pendingBrowserCreate{},
	}

	rawClient, ok := wv.activateNativePopup(55, "https://example.com/oauth")
	require.True(t, ok)
	require.NotNil(t, rawClient)
	require.Equal(t, "https://example.com/oauth", wv.pendingNavigationURI())

	wv.updateURI("about:blank")
	wv.updateLoadState(false, false, false)

	require.Equal(t, "https://example.com/oauth", wv.pendingNavigationURI())
}

func TestHandleNativePopupAborted_PreservesPrimedNavigationForFallback(t *testing.T) {
	closed := false
	parent := &WebView{}
	wv := &WebView{
		nativePopupCandidate: true,
		nativePopupParent:    parent,
		nativePopupID:        55,
		pendingCreate:        &pendingBrowserCreate{},
		pendingURI:           "https://example.com/oauth",
		isLoading:            true,
		closeCallbacks: []func(){
			func() { closed = true },
		},
	}

	wv.handleNativePopupAborted()

	require.False(t, closed)
	require.Equal(t, "https://example.com/oauth", wv.pendingNavigationURI())
	require.False(t, wv.isNativePopupCandidate())
	require.Same(t, parent, wv.nativePopupParent)
	require.Equal(t, int32(0), wv.nativePopupID)
	require.True(t, wv.isLoading)
}

func TestOnBeforePopup_TimesOutGTKDispatchAndBlocksPopup(t *testing.T) {
	delayed := make(chan func(), 1)
	parentWV := &WebView{
		ctx:            context.Background(),
		id:             16,
		gtkSyncTimeout: 5 * time.Millisecond,
		gtkSyncIsOwner: func() bool { return false },
		gtkSyncDispatch: func(fn func()) {
			delayed <- fn
		},
	}
	var createCalls atomic.Int32
	parentWV.SetCallbacks(&port.WebViewCallbacks{
		OnCreate: func(_ port.PopupRequest) port.WebView {
			createCalls.Add(1)
			return &WebView{ctx: context.Background(), id: 24}
		},
	})

	h := &handlerSet{wv: parentWV}
	blocked := h.OnBeforePopup(nil, nil, 90, "https://example.com/slow-popup", "slow-popup", 0, 1, nil, nil, nil, nil, nil, nil)

	require.True(t, blocked)
	require.Zero(t, createCalls.Load())

	select {
	case fn := <-delayed:
		fn()
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for delayed popup dispatch")
	}
	require.Zero(t, createCalls.Load())
}

func TestOnBeforePopup_PrimesPopupNavigationWhenCEFPopupBlocksNativeCreation(t *testing.T) {
	parentWV := &WebView{ctx: context.Background(), id: 17}
	popupWV := &WebView{ctx: context.Background(), id: 23}
	parentWV.SetCallbacks(&port.WebViewCallbacks{
		OnCreate: func(req port.PopupRequest) port.WebView {
			require.Equal(t, "https://example.com/popup", req.TargetURI)
			require.Equal(t, "auth-popup", req.FrameName)
			require.True(t, req.IsUserGesture)
			popupWV.PrimePopupNavigation(req.TargetURI)
			return popupWV
		},
	})

	h := &handlerSet{wv: parentWV}
	blocked := h.OnBeforePopup(nil, nil, 91, "https://example.com/popup", "auth-popup", 0, 1, nil, nil, nil, nil, nil, nil)

	require.True(t, blocked)
	require.Equal(t, "https://example.com/popup", popupWV.pendingNavigationURI())
}

func TestOnBeforePopup_PrimesPopupShellWhenNativePopupCannotBeArmed(t *testing.T) {
	parentWV := &WebView{ctx: context.Background(), id: 31}
	popupWV := &WebView{ctx: context.Background(), id: 32}
	popupWV.markNativePopupCandidate(parentWV)
	parentWV.SetCallbacks(&port.WebViewCallbacks{
		OnCreate: func(req port.PopupRequest) port.WebView {
			require.Equal(t, "https://example.com/login", req.TargetURI)
			popupWV.PrimePopupNavigation(req.TargetURI)
			return popupWV
		},
	})

	h := &handlerSet{wv: parentWV}
	blocked := h.OnBeforePopup(nil, nil, 77, "https://example.com/login", "Google login", 0, 1, nil, nil, nil, nil, nil, nil)

	require.True(t, blocked)
	require.False(t, popupWV.isNativePopupCandidate())
	require.Equal(t, "https://example.com/login", popupWV.pendingNavigationURI())
}

func TestOnBeforePopup_PaneDecisionBlocksNativePopupAndKeepsPanePath(t *testing.T) {
	parentWV := &WebView{ctx: context.Background(), id: 41}
	popupWV := &WebView{ctx: context.Background(), id: 42}
	popupWV.SetBrowsingContextHostDecision(dto.HostDecision{Kind: dto.HostDecisionCreatePane})
	parentWV.SetCallbacks(&port.WebViewCallbacks{
		OnCreate: func(req port.PopupRequest) port.WebView {
			popupWV.PreparePaneHostedBrowsingContext()
			popupWV.PrimePopupNavigation(req.TargetURI)
			return popupWV
		},
	})

	h := &handlerSet{wv: parentWV}
	blocked := h.OnBeforePopup(nil, nil, 81, "https://example.com/pane", "_blank", 0, 1, nil, nil, nil, nil, nil, nil)

	require.True(t, blocked)
	require.Equal(t, "https://example.com/pane", popupWV.pendingNavigationURI())
}

func TestOnBeforePopup_NativeDecisionAbortsHostWhenArmingFails(t *testing.T) {
	parentWV := &WebView{ctx: context.Background(), id: 51}
	popupWV := &WebView{ctx: context.Background(), id: 52}
	popupWV.markNativePopupCandidate(parentWV)
	popupWV.SetBrowsingContextHostDecision(dto.HostDecision{Kind: dto.HostDecisionCreateNativeWin})
	aborted := false
	popupWV.SetNativePopupHostAbort(func() { aborted = true })
	parentWV.SetCallbacks(&port.WebViewCallbacks{
		OnCreate: func(req port.PopupRequest) port.WebView {
			popupWV.PrimePopupNavigation(req.TargetURI)
			return popupWV
		},
	})

	h := &handlerSet{wv: parentWV}
	blocked := h.OnBeforePopup(nil, nil, 82, "https://accounts.google.com/o/oauth2/v2/auth", "oauth", 0, 1, nil, nil, nil, nil, nil, nil)

	require.True(t, blocked)
	require.True(t, aborted)
}

func TestConfigureNativePopupWindow_UsesSharedTextureWindowlessDefaults(t *testing.T) {
	windowInfo := purecef.NewWindowInfo()
	settings := purecef.NewBrowserSettings()

	configureNativePopupWindow(&windowInfo, &settings, 72, 0xFF112233)

	require.Equal(t, purecef.WindowHandle(0), windowInfo.ParentWindow)
	require.Equal(t, int32(1), windowInfo.WindowlessRenderingEnabled)
	require.Equal(t, int32(1), windowInfo.SharedTextureEnabled)
	require.Equal(t, int32(72), settings.WindowlessFrameRate)
	require.Equal(t, int32(1), settings.LocalStorage)
	require.Equal(t, uint32(0xFF112233), settings.BackgroundColor)
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

func TestOnLoadErrorMainFrameCommitsFailedNavigation(t *testing.T) {
	wv := &WebView{ctx: context.Background()}
	wv.updateLoadState(true, true, false)
	var gotEvents []port.LoadEvent
	wv.SetCallbacks(&port.WebViewCallbacks{
		OnLoadChanged: func(event port.LoadEvent) {
			gotEvents = append(gotEvents, event)
		},
	})

	h := &handlerSet{wv: wv}
	h.OnLoadError(nil, stubFrame{main: true, url: "http://localhost:9"}, -102, "CONNECTION_REFUSED", "http://localhost:9")

	require.Equal(t, []port.LoadEvent{port.LoadCommitted}, gotEvents)
	require.Equal(t, "http://localhost:9", wv.URI())
	require.True(t, wv.IsLoading())
	require.True(t, wv.CanGoBack())
}

func TestOnLoadErrorFollowedByLoadingStateFinishTerminatesNavigation(t *testing.T) {
	wv := &WebView{ctx: context.Background()}
	wv.updateLoadState(true, false, false)
	var gotEvents []port.LoadEvent
	wv.SetCallbacks(&port.WebViewCallbacks{
		OnLoadChanged: func(event port.LoadEvent) {
			gotEvents = append(gotEvents, event)
		},
	})

	h := &handlerSet{wv: wv}
	h.OnLoadError(nil, stubFrame{main: true, url: "http://localhost:9"}, -102, "CONNECTION_REFUSED", "http://localhost:9")
	h.OnLoadingStateChange(nil, 0, 0, 0)

	require.Equal(t, []port.LoadEvent{port.LoadCommitted, port.LoadFinished}, gotEvents)
	require.False(t, wv.IsLoading())
}

func TestOnLoadErrorAfterLoadStartDoesNotDispatchDuplicateEvents(t *testing.T) {
	wv := &WebView{ctx: context.Background()}
	var gotEvents []port.LoadEvent
	wv.SetCallbacks(&port.WebViewCallbacks{
		OnLoadChanged: func(event port.LoadEvent) {
			gotEvents = append(gotEvents, event)
		},
	})

	h := &handlerSet{wv: wv}
	h.OnLoadStart(nil, stubFrame{main: true, url: "http://localhost:9"}, 0)
	gotEvents = nil
	h.OnLoadError(nil, stubFrame{main: true, url: "http://localhost:9"}, -102, "CONNECTION_REFUSED", "http://localhost:9")

	require.Empty(t, gotEvents)
}

func TestOnLoadErrorAbortedNavigationDoesNotCommit(t *testing.T) {
	wv := &WebView{ctx: context.Background()}
	var gotEvents []port.LoadEvent
	wv.SetCallbacks(&port.WebViewCallbacks{
		OnLoadChanged: func(event port.LoadEvent) {
			gotEvents = append(gotEvents, event)
		},
	})

	h := &handlerSet{wv: wv}
	h.OnLoadError(nil, stubFrame{main: true, url: "https://example.com/file.zip"}, cefErrAborted, "ERR_ABORTED", "https://example.com/file.zip")

	require.Empty(t, gotEvents)
	require.Empty(t, wv.URI())
}

func TestOnLoadErrorSubFrameDoesNotTerminateNavigation(t *testing.T) {
	wv := &WebView{ctx: context.Background()}
	var gotEvents []port.LoadEvent
	wv.SetCallbacks(&port.WebViewCallbacks{
		OnLoadChanged: func(event port.LoadEvent) {
			gotEvents = append(gotEvents, event)
		},
	})

	h := &handlerSet{wv: wv}
	h.OnLoadError(nil, stubFrame{main: false, url: "http://localhost:9/frame"}, -102, "CONNECTION_REFUSED", "http://localhost:9/frame")

	assert.Empty(t, gotEvents)
}

func TestOnTextSelectionChanged_ForwardsSelectionToClipboardOrchestrator(t *testing.T) {
	recorder := &clipboardOrchestratorRecorder{}
	orchestrator := portmocks.NewMockClipboardTextOrchestrator(t)
	orchestrator.EXPECT().HandleSelectionUpdate(mock.Anything, mock.Anything).Run(func(_ context.Context, input dto.SelectionClipboardInput) {
		recorder.mu.Lock()
		defer recorder.mu.Unlock()
		recorder.selection = input
		recorder.selectionCalls++
	}).Return(nil).Once()
	zeroDelay := time.Duration(0)
	wv := &WebView{
		ctx:                    context.Background(),
		id:                     42,
		selectionDebounceDelay: &zeroDelay,
		engine: &Engine{
			clipboardTextOrchestrator: orchestrator,
		},
	}
	h := &selectionRenderHandlerForTest{wv: wv}

	h.OnTextSelectionChanged(nil, "selected text", nil)

	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	require.Equal(t, 1, recorder.selectionCalls)
	require.Equal(t, "selected text", recorder.selection.Text)
	require.Equal(t, dto.ClipboardSourceCEF, recorder.selection.SourceEngine)
	require.Equal(t, uint64(42), recorder.selection.ViewID)
}

func TestOnTextSelectionChanged_DebouncesAndCollapsesRapidUpdates(t *testing.T) {
	recorder := &clipboardOrchestratorRecorder{}
	scheduler := &controlledSelectionScheduler{}
	orchestrator := portmocks.NewMockClipboardTextOrchestrator(t)
	orchestrator.EXPECT().HandleSelectionUpdate(mock.Anything, mock.Anything).Run(func(_ context.Context, input dto.SelectionClipboardInput) {
		recorder.mu.Lock()
		defer recorder.mu.Unlock()
		recorder.selection = input
		recorder.selectionCalls++
	}).Return(nil).Once()
	wv := &WebView{
		ctx:                       context.Background(),
		id:                        42,
		selectionDebounceSchedule: scheduler.schedule,
		engine: &Engine{
			clipboardTextOrchestrator: orchestrator,
		},
	}
	h := &selectionRenderHandlerForTest{wv: wv}

	h.OnTextSelectionChanged(nil, "first selection", nil)
	h.OnTextSelectionChanged(nil, "second selection", nil)

	recorder.mu.Lock()
	gotCalls := recorder.selectionCalls
	recorder.mu.Unlock()
	require.Zero(t, gotCalls)
	require.Len(t, scheduler.callbacks, 2)

	scheduler.fire(0)
	recorder.mu.Lock()
	gotCalls = recorder.selectionCalls
	recorder.mu.Unlock()
	require.Zero(t, gotCalls)

	scheduler.fire(1)
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	require.Equal(t, 1, recorder.selectionCalls)
	require.Equal(t, "second selection", recorder.selection.Text)
	require.Equal(t, "second selection", wv.selectedTextSnapshot())
}

func TestOnTextSelectionChanged_SuppressesAutoCopyWhenFocusedNodeEditableAndResumesWhenCleared(t *testing.T) {
	recorder := &clipboardOrchestratorRecorder{}
	scheduler := &controlledSelectionScheduler{}
	orchestrator := portmocks.NewMockClipboardTextOrchestrator(t)
	orchestrator.EXPECT().HandleSelectionUpdate(mock.Anything, mock.Anything).Run(func(_ context.Context, input dto.SelectionClipboardInput) {
		recorder.mu.Lock()
		defer recorder.mu.Unlock()
		recorder.selection = input
		recorder.selectionCalls++
	}).Return(nil).Once()
	wv := &WebView{
		ctx:                       context.Background(),
		id:                        42,
		selectionDebounceSchedule: scheduler.schedule,
		engine: &Engine{
			clipboardTextOrchestrator: orchestrator,
		},
	}
	h := &selectionRenderHandlerForTest{wv: wv}
	clientHandlers := &handlerSet{wv: wv}
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
		clientHandlers.OnProcessMessageReceived(nil, nil, 0, message)
	}).Once()
	(&rendererBridgeProcessHandler{}).OnFocusedNodeChanged(nil, frame, stubEditableDomnode{editable: true})

	h.OnTextSelectionChanged(nil, "editable selection", nil)
	require.Equal(t, "editable selection", wv.selectedTextSnapshot())

	recorder.mu.Lock()
	gotCalls := recorder.selectionCalls
	recorder.mu.Unlock()
	require.Zero(t, gotCalls)
	require.Empty(t, scheduler.callbacks)

	newRendererBridgeProcessMessage = func(name string) purecef.ProcessMessage {
		return newTestBridgeProcessMessage(name, false)
	}
	frame.EXPECT().SendProcessMessage(purecef.ProcessIDPidBrowser, mock.Anything).Run(func(_ purecef.ProcessID, message purecef.ProcessMessage) {
		require.Equal(t, rendererBridgeMessageName, message.GetName())
		args := message.GetArgumentList()
		require.Equal(t, "editable_focus_changed", args.GetString(0))
		require.Equal(t, "0", args.GetString(1))
		clientHandlers.OnProcessMessageReceived(nil, nil, 0, message)
	}).Once()
	(&rendererBridgeProcessHandler{}).OnFocusedNodeChanged(nil, frame, stubEditableDomnode{editable: false})

	h.OnTextSelectionChanged(nil, "free selection", nil)
	require.Equal(t, "free selection", wv.selectedTextSnapshot())
	recorder.mu.Lock()
	gotCalls = recorder.selectionCalls
	recorder.mu.Unlock()
	require.Zero(t, gotCalls)
	require.Len(t, scheduler.callbacks, 1)

	scheduler.fire(0)
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	require.Equal(t, 1, recorder.selectionCalls)
	require.Equal(t, "free selection", recorder.selection.Text)
}

func TestDestroy_WithoutHostRunsCloseCallbacks(t *testing.T) {
	called := false
	wv := &WebView{closeCallbacks: []func(){func() { called = true }}}

	wv.Destroy()

	require.True(t, called)
}

func TestOnTextSelectionChanged_DoesNotEmitLateDebouncedUpdateAfterDestroy(t *testing.T) {
	scheduler := &controlledSelectionScheduler{}
	orchestrator := portmocks.NewMockClipboardTextOrchestrator(t)
	wv := &WebView{
		ctx:                       context.Background(),
		id:                        42,
		selectionDebounceSchedule: scheduler.schedule,
		engine: &Engine{
			clipboardTextOrchestrator: orchestrator,
		},
	}
	h := &selectionRenderHandlerForTest{wv: wv}

	h.OnTextSelectionChanged(nil, "selected text", nil)
	require.Len(t, scheduler.callbacks, 1)
	wv.Destroy()
	scheduler.fire(0)

	orchestrator.AssertNotCalled(t, "HandleSelectionUpdate", mock.Anything, mock.Anything)
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
func (n stubEditableDomnode) IsSame(that purecef.Domnode) bool {
	other, ok := that.(stubEditableDomnode)
	return ok && n == other
}
func (n stubEditableDomnode) GetName() string                          { return "" }
func (n stubEditableDomnode) GetValue() string                         { return "" }
func (n stubEditableDomnode) SetValue(string) int32                    { return 0 }
func (n stubEditableDomnode) GetAsMarkup() string                      { return "" }
func (n stubEditableDomnode) GetDocument() purecef.Domdocument         { return nil }
func (n stubEditableDomnode) GetParent() purecef.Domnode               { return nil }
func (n stubEditableDomnode) GetPreviousSibling() purecef.Domnode      { return nil }
func (n stubEditableDomnode) GetNextSibling() purecef.Domnode          { return nil }
func (n stubEditableDomnode) HasChildren() bool                        { return false }
func (n stubEditableDomnode) GetFirstChild() purecef.Domnode           { return nil }
func (n stubEditableDomnode) GetLastChild() purecef.Domnode            { return nil }
func (n stubEditableDomnode) GetElementTagName() string                { return "" }
func (n stubEditableDomnode) HasElementAttributes() bool               { return false }
func (n stubEditableDomnode) HasElementAttribute(string) bool          { return false }
func (n stubEditableDomnode) GetElementAttribute(string) string        { return "" }
func (n stubEditableDomnode) GetElementAttributes(purecef.StringMap)   {}
func (n stubEditableDomnode) SetElementAttribute(string, string) int32 { return 0 }
func (n stubEditableDomnode) GetElementInnerText() string              { return "" }
func (n stubEditableDomnode) GetElementBounds() uintptr                { return 0 }

func TestOptionalHandlersAreAlwaysEnabled(t *testing.T) {
	h := &handlerSet{}

	// AudioHandler is always enabled (required for media decoding).
	require.Same(t, h, h.GetAudioHandler())
	require.Same(t, h, h.GetContextMenuHandler())
	require.Nil(t, h.GetRenderHandler())
}

func TestRenderTextSelectionHookPreservesWebViewState(t *testing.T) {
	wv := &WebView{ctx: context.Background()}

	handleRenderTextSelectionChanged(wv, "selected via hook")

	require.Equal(t, "selected via hook", wv.selectedTextSnapshot())
}

func TestGetDownloadHandler_EnabledWhenEngineConfigured(t *testing.T) {
	eng := &Engine{}
	require.NoError(t, eng.ConfigureDownloads(context.Background(), "/tmp/downloads", nil, stubDownloadPreparer{}))

	h := &handlerSet{
		wv: &WebView{
			ctx:    context.Background(),
			engine: eng,
		},
	}

	require.Same(t, h, h.GetDownloadHandler())
}

type selectionRenderHandlerForTest struct{ wv *WebView }

func (h *selectionRenderHandlerForTest) OnTextSelectionChanged(_ purecef.Browser, selectedText string, _ *purecef.Range) {
	handleRenderTextSelectionChanged(h.wv, selectedText)
}
