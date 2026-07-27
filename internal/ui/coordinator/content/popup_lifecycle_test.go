package content

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/rs/zerolog"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/bnema/dumber/internal/application/dto"
	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/application/port/mocks"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/logging"
)

// ---------------------------------------------------------------------------
// DetectPopupType
// ---------------------------------------------------------------------------

func TestTrackOAuthPopup_StoresState(t *testing.T) {
	t.Parallel()

	c := &Coordinator{
		webViews: make(map[entity.PaneID]port.WebView),
		popups:   newPopupManager(),
	}

	popupID := port.WebViewID(1)
	parentPaneID := entity.PaneID("parent-pane")

	c.trackOAuthPopup(popupID, parentPaneID, "https://parent.example.com/login")

	c.popups.mu.RLock()
	state, ok := c.popups.popupOAuth[popupID]
	c.popups.mu.RUnlock()

	require.True(t, ok, "oauth state should be tracked after trackOAuthPopup")
	require.NotNil(t, state)
	assert.Equal(t, parentPaneID, state.ParentPaneID)
	assert.Equal(t, "https://parent.example.com/login", state.ParentURIAtOpen)
	assert.False(t, state.Seen, "Seen should be false before capturing callback URI")
}

func TestCapturePopupOAuthState_SuccessCallback(t *testing.T) {
	t.Parallel()

	c := &Coordinator{
		webViews: make(map[entity.PaneID]port.WebView),
		popups:   newPopupManager(),
	}

	popupID := port.WebViewID(2)
	c.trackOAuthPopup(popupID, entity.PaneID("parent"), "https://parent.example.com/login")
	c.capturePopupOAuthState(popupID, "https://app.example.com/callback?code=abc123")

	c.popups.mu.RLock()
	state := c.popups.popupOAuth[popupID]
	c.popups.mu.RUnlock()

	require.NotNil(t, state)
	assert.True(t, state.Seen)
	assert.True(t, state.Success)
	assert.False(t, state.Error)
	assert.Equal(t, "https://app.example.com/callback?code=abc123", state.CallbackURI)
}

func TestCapturePopupOAuthState_ErrorCallback(t *testing.T) {
	t.Parallel()

	c := &Coordinator{
		webViews: make(map[entity.PaneID]port.WebView),
		popups:   newPopupManager(),
	}

	popupID := port.WebViewID(3)
	c.trackOAuthPopup(popupID, entity.PaneID("parent"), "https://parent.example.com/login")
	c.capturePopupOAuthState(popupID, "https://app.example.com/callback?error=access_denied")

	c.popups.mu.RLock()
	state := c.popups.popupOAuth[popupID]
	c.popups.mu.RUnlock()

	require.NotNil(t, state)
	assert.True(t, state.Seen)
	assert.False(t, state.Success)
	assert.True(t, state.Error)
}

func TestCapturePopupOAuthState_UnknownPopupIsNoop(t *testing.T) {
	t.Parallel()

	c := &Coordinator{
		webViews: make(map[entity.PaneID]port.WebView),
		popups:   newPopupManager(),
	}

	// Should not panic when no tracking entry exists.
	c.capturePopupOAuthState(port.WebViewID(999), "https://example.com/callback?code=x")

	c.popups.mu.RLock()
	_, ok := c.popups.popupOAuth[port.WebViewID(999)]
	c.popups.mu.RUnlock()

	assert.False(t, ok)
}

// ---------------------------------------------------------------------------
// SetPopupConfig
// ---------------------------------------------------------------------------

func TestSetPopupConfig_SetsFields(t *testing.T) {
	t.Parallel()

	c := &Coordinator{}

	factory := &stubFactory{}
	cfg := &entity.BrowsingContextConfig{Behavior: entity.PopupBehaviorSplit}
	genID := func() string { return "test-id" }

	c.SetPopupConfig(factory, cfg, genID)

	assert.Equal(t, factory, c.popups.factory)
	assert.True(t, c.popups.hasPopupConfig)
	assert.Equal(t, *cfg, c.popups.popupConfig)
	cfg.Behavior = entity.PopupBehaviorWindowed
	assert.Equal(t, entity.PopupBehaviorSplit, c.popups.popupConfig.Behavior)
	assert.NotNil(t, c.popups.generatePaneID)
	assert.Equal(t, "test-id", c.popups.generatePaneID())
}

func TestSetPopupConfig_NilConfigAllowed(t *testing.T) {
	t.Parallel()

	c := &Coordinator{}
	c.SetPopupConfig(nil, nil, nil)

	assert.Nil(t, c.popups.factory)
	assert.False(t, c.popups.hasPopupConfig)
	assert.Equal(t, entity.BrowsingContextConfig{}, c.popups.popupConfig)
	assert.Nil(t, c.popups.generatePaneID)
}

func TestUpdatePopupConfig_CopiesLatestValue(t *testing.T) {
	t.Parallel()

	c := &Coordinator{}
	initial := &entity.BrowsingContextConfig{OpenInNewPane: true, OAuthAutoClose: false}
	c.SetPopupConfig(nil, initial, nil)

	c.UpdatePopupConfig(entity.BrowsingContextConfig{OpenInNewPane: false, OAuthAutoClose: true})

	assert.True(t, c.popups.hasPopupConfig)
	assert.False(t, c.popups.popupConfig.OpenInNewPane)
	assert.True(t, c.popups.popupConfig.OAuthAutoClose)
	assert.True(t, initial.OpenInNewPane)
	assert.False(t, initial.OAuthAutoClose)
}

type popupOpenerWebViewStub struct {
	*popupNavigationWebViewStub
}

func (*popupOpenerWebViewStub) EnablePopupOpenerBridge(port.WebView, bool) {}
func (*popupOpenerWebViewStub) AddOpenerMessageCallback(func())            {}
func (*popupOpenerWebViewStub) AddOpenerNavigationCallback(func(string))   {}
func (*popupOpenerWebViewStub) HasActivePopupOpenerBridge() bool           { return true }

type popupNavigationWebViewStub struct {
	*mocks.MockWebView
	primed                     []string
	browsingContextDecision    dto.HostDecision
	hasBrowsingContextDecision bool
}

func (s *popupNavigationWebViewStub) PrimePopupNavigation(uri string) {
	s.primed = append(s.primed, uri)
}

func (s *popupNavigationWebViewStub) SetBrowsingContextHostDecision(decision dto.HostDecision) {
	s.browsingContextDecision = decision
	s.hasBrowsingContextDecision = true
}

func (s *popupNavigationWebViewStub) BrowsingContextHostDecision() (dto.HostDecision, bool) {
	return s.browsingContextDecision, s.hasBrowsingContextDecision
}

func (*popupNavigationWebViewStub) SetOnReadyToShow(func()) {}
func (*popupNavigationWebViewStub) SetOnClose(func())       {}
func (*popupNavigationWebViewStub) Show()                   {}

// stubFactory satisfies port.WebViewFactory without importing the mocks package.
type stubFactory struct{}

func (s *stubFactory) Create(_ context.Context) (port.WebView, error) {
	return nil, nil
}

func (s *stubFactory) CreateRelated(_ context.Context, _ port.WebViewID) (port.WebView, error) {
	return nil, nil
}

// ---------------------------------------------------------------------------
// Concurrent access to pendingPopups
// ---------------------------------------------------------------------------

func TestPendingPopups_ConcurrentAccess(t *testing.T) {
	t.Parallel()

	c := &Coordinator{
		popups: newPopupManager(),
	}

	const workers = 20
	var wg sync.WaitGroup
	wg.Add(workers * 2)

	// Writers
	for i := range workers {
		go func(id int) {
			defer wg.Done()
			popupID := port.WebViewID(id)
			c.popups.mu.Lock()
			c.popups.pendingPopups[popupID] = &PendingPopup{
				FrameName: "_blank",
				PopupType: PopupTypeTab,
			}
			c.popups.mu.Unlock()
		}(i)
	}

	// Readers
	for i := range workers {
		go func(id int) {
			defer wg.Done()
			popupID := port.WebViewID(id)
			c.popups.mu.RLock()
			_ = c.popups.pendingPopups[popupID]
			c.popups.mu.RUnlock()
		}(i)
	}

	wg.Wait()

	c.popups.mu.RLock()
	count := len(c.popups.pendingPopups)
	c.popups.mu.RUnlock()

	assert.Equal(t, workers, count, "each writer uses a unique key, so all inserts should be present")
}

func TestPendingPopups_ConcurrentDeleteAndRead(t *testing.T) {
	t.Parallel()

	c := &Coordinator{
		popups: newPopupManager(),
	}

	// Pre-populate
	for i := range 10 {
		c.popups.pendingPopups[port.WebViewID(i)] = &PendingPopup{PopupType: PopupTypePopup}
	}

	var wg sync.WaitGroup
	wg.Add(20)

	// Deleters
	for i := range 10 {
		go func(id int) {
			defer wg.Done()
			c.popups.mu.Lock()
			delete(c.popups.pendingPopups, port.WebViewID(id))
			c.popups.mu.Unlock()
		}(i)
	}

	// Readers
	for i := range 10 {
		go func(id int) {
			defer wg.Done()
			c.popups.mu.RLock()
			_ = c.popups.pendingPopups[port.WebViewID(id)]
			c.popups.mu.RUnlock()
		}(i)
	}

	wg.Wait()

	c.popups.mu.RLock()
	defer c.popups.mu.RUnlock()
	assert.Empty(t, c.popups.pendingPopups, "all preloaded popups should have been deleted")
}

func TestPopupDeferredFeaturelessBlankNavigatesSourceAndCleansStagingExactlyOnce(t *testing.T) {
	ctx := context.Background()
	parent := mocks.NewMockWebView(t)
	parent.EXPECT().ID().Return(port.WebViewID(101)).Once()
	parent.EXPECT().LoadURI(mock.Anything, "https://example.com/open").Return(nil).Once()
	popup := &deferredPopupWebViewStub{MockWebView: mocks.NewMockWebView(t), features: dto.PopupFeatures{State: dto.PopupFeaturesNone}}
	popup.EXPECT().ID().Return(port.WebViewID(201)).Maybe()
	popup.EXPECT().Destroy().Once()
	factory := mocks.NewMockWebViewFactory(t)
	factory.EXPECT().CreateRelated(mock.Anything, port.WebViewID(101)).Return(popup, nil).Once()
	staging := &popupStagingHostStub{}
	c := &Coordinator{popups: newPopupManager()}
	c.SetPopupConfig(factory, &entity.BrowsingContextConfig{OpenInNewPane: false}, func() string { return "deferred" })
	c.SetPopupSourceHostResolver(func(entity.PaneID) dto.SourceHostKind { return dto.SourceHostFloating })
	c.SetOnStagePopup(func(context.Context, StagePopupInput) (PopupStagingHost, error) {
		staging.attached = true
		return staging, nil
	})
	c.SetOnOpenBrowserWindow(func(context.Context, BrowserWindowInput) (BrowserWindowResult, error) {
		t.Fatal("featureless _blank must not open a browser window")
		return BrowserWindowResult{}, nil
	})
	c.SetOnOpenNativePopup(func(context.Context, NativePopupInput) error {
		t.Fatal("featureless _blank must not use native host")
		return nil
	})

	got := c.handlePopupCreate(ctx, "floating", parent, port.PopupRequest{
		Engine: dto.BrowserEngineWebKit, TargetURI: "https://example.com/open", FrameName: "_blank",
		TargetDisposition: dto.WindowDispositionNewPopup, PopupFeatures: dto.PopupFeatures{State: dto.PopupFeaturesUnknown},
	})
	require.Same(t, popup, got)
	assert.True(t, staging.attached)
	assert.False(t, staging.detached)
	popup.ready()
	popup.ready()
	assert.True(t, staging.detached)
	assert.Equal(t, 1, staging.destroyCalls)
}

func TestPopupDeferredFeaturedDetachesBeforeNativeHost(t *testing.T) {
	ctx := context.Background()
	parent := mocks.NewMockWebView(t)
	parent.EXPECT().ID().Return(port.WebViewID(101)).Once()
	popup := &deferredPopupWebViewStub{MockWebView: mocks.NewMockWebView(t), features: dto.PopupFeatures{State: dto.PopupFeaturesSpecified, Width: 640, WidthSet: true}}
	popup.EXPECT().ID().Return(port.WebViewID(202)).Maybe()
	factory := mocks.NewMockWebViewFactory(t)
	factory.EXPECT().CreateRelated(mock.Anything, port.WebViewID(101)).Return(popup, nil).Once()
	staging := &popupStagingHostStub{}
	c := &Coordinator{popups: newPopupManager()}
	c.SetPopupConfig(factory, &entity.BrowsingContextConfig{OpenInNewPane: false}, func() string { return "deferred" })
	c.SetPopupSourceHostResolver(func(entity.PaneID) dto.SourceHostKind { return dto.SourceHostFloating })
	c.SetOnStagePopup(func(context.Context, StagePopupInput) (PopupStagingHost, error) {
		staging.attached = true
		return staging, nil
	})
	c.SetOnOpenNativePopup(func(_ context.Context, input NativePopupInput) error {
		assert.True(t, staging.detached)
		assert.Equal(t, dto.PopupFeaturesSpecified, input.Request.PopupFeatures.State)
		assert.Same(t, popup, input.PopupWebView)
		return nil
	})

	require.Same(t, popup, c.handlePopupCreate(ctx, "floating", parent, port.PopupRequest{
		Engine: dto.BrowserEngineWebKit, TargetURI: "https://example.com/open", FrameName: "_blank",
		TargetDisposition: dto.WindowDispositionNewPopup, PopupFeatures: dto.PopupFeatures{State: dto.PopupFeaturesUnknown},
	}))
	popup.ready()
	decision, ok := popup.BrowsingContextHostDecision()
	require.True(t, ok)
	assert.Equal(t, dto.HostDecisionCreateNativePopup, decision.Kind)
}

func TestPopupDeferredNamedReuseDestroysNewlyStagedWebViewExactlyOnce(t *testing.T) {
	ctx := context.Background()
	parentPaneID := entity.PaneID("floating")
	existingPaneID := entity.PaneID("existing")
	parent := mocks.NewMockWebView(t)
	parent.EXPECT().ID().Return(port.WebViewID(101)).Once()
	staged := &deferredPopupWebViewStub{MockWebView: mocks.NewMockWebView(t), features: dto.PopupFeatures{State: dto.PopupFeaturesNone}}
	staged.EXPECT().ID().Return(port.WebViewID(205)).Maybe()
	staged.EXPECT().Destroy().Once()
	existing := mocks.NewMockWebView(t)
	existing.EXPECT().ID().Return(port.WebViewID(301)).Times(3)
	existing.EXPECT().IsDestroyed().Return(false).Twice()
	existing.EXPECT().LoadURI(mock.Anything, "https://example.com/reused").Return(nil).Once()
	factory := mocks.NewMockWebViewFactory(t)
	factory.EXPECT().CreateRelated(mock.Anything, port.WebViewID(101)).Return(staged, nil).Once()
	staging := &popupStagingHostStub{}
	c := &Coordinator{webViews: make(map[entity.PaneID]port.WebView), popups: newPopupManager()}
	c.SetPopupConfig(factory, &entity.BrowsingContextConfig{OpenInNewPane: false}, nil)
	c.SetPopupSourceHostResolver(func(entity.PaneID) dto.SourceHostKind { return dto.SourceHostFloating })
	c.SetPopupWindowIDResolver(func(paneID entity.PaneID) (string, bool) {
		windowByPane := map[entity.PaneID]string{parentPaneID: "owner", existingPaneID: "host"}
		windowID, ok := windowByPane[paneID]
		return windowID, ok
	})
	c.SetOnStagePopup(func(context.Context, StagePopupInput) (PopupStagingHost, error) { return staging, nil })

	require.Same(t, staged, c.handlePopupCreate(ctx, parentPaneID, parent, port.PopupRequest{
		Engine: dto.BrowserEngineWebKit, TargetURI: "https://example.com/reused", FrameName: "shared",
		PopupFeatures: dto.PopupFeatures{State: dto.PopupFeaturesUnknown},
	}))
	c.RegisterPopupWebView(existingPaneID, existing)
	c.popups.namedContexts.Register("owner", "host", "shared", existingPaneID, port.WebViewID(301))
	staged.ready()
	staged.ready()

	assert.True(t, staging.detached)
	assert.Equal(t, 1, staging.destroyCalls)
}

func TestPopupDeferredNamedReuseRaceLogsHostUnavailableAndCleansStagingExactlyOnce(t *testing.T) {
	var output bytes.Buffer
	logger := zerolog.New(&output).Level(zerolog.WarnLevel)
	ctx := logging.WithContext(context.Background(), logger)
	parentPaneID := entity.PaneID("floating")
	existingPaneID := entity.PaneID("existing")
	parent := mocks.NewMockWebView(t)
	parent.EXPECT().ID().Return(port.WebViewID(101)).Once()
	staged := &deferredPopupWebViewStub{MockWebView: mocks.NewMockWebView(t), features: dto.PopupFeatures{State: dto.PopupFeaturesNone}}
	staged.EXPECT().ID().Return(port.WebViewID(208)).Maybe()
	staged.EXPECT().Destroy().Once()
	existing := mocks.NewMockWebView(t)
	destroyedChecks := 0
	existing.EXPECT().IsDestroyed().RunAndReturn(func() bool {
		destroyedChecks++
		return destroyedChecks > 1
	}).Twice()
	existing.EXPECT().ID().Return(port.WebViewID(301)).Once()
	factory := mocks.NewMockWebViewFactory(t)
	factory.EXPECT().CreateRelated(mock.Anything, port.WebViewID(101)).Return(staged, nil).Once()
	staging := &popupStagingHostStub{}
	c := &Coordinator{webViews: make(map[entity.PaneID]port.WebView), popups: newPopupManager()}
	c.SetPopupConfig(factory, &entity.BrowsingContextConfig{OpenInNewPane: false}, nil)
	c.SetPopupSourceHostResolver(func(entity.PaneID) dto.SourceHostKind { return dto.SourceHostFloating })
	c.SetPopupWindowIDResolver(func(paneID entity.PaneID) (string, bool) {
		windowByPane := map[entity.PaneID]string{parentPaneID: "owner", existingPaneID: "host"}
		windowID, ok := windowByPane[paneID]
		return windowID, ok
	})
	c.SetOnStagePopup(func(context.Context, StagePopupInput) (PopupStagingHost, error) { return staging, nil })

	require.Same(t, staged, c.handlePopupCreate(ctx, parentPaneID, parent, port.PopupRequest{
		Engine: dto.BrowserEngineWebKit, TargetURI: "https://example.com/reused", FrameName: "shared",
		PopupFeatures: dto.PopupFeatures{State: dto.PopupFeaturesUnknown},
	}))
	c.RegisterPopupWebView(existingPaneID, existing)
	c.popups.namedContexts.Register("owner", "host", "shared", existingPaneID, port.WebViewID(301))
	staged.ready()
	staged.ready()

	assert.Equal(t, 2, destroyedChecks)
	assert.Equal(t, 1, staging.destroyCalls)
	record := decodeBrowsingContextLog(t, output.Bytes())
	assert.Equal(t, "host-unavailable", record["reason_code"])
}

func TestPopupDeferredNativeAbortWithoutFallbackLogsStructuredFailure(t *testing.T) {
	var output bytes.Buffer
	logger := zerolog.New(&output).Level(zerolog.WarnLevel)
	ctx := logging.WithContext(context.Background(), logger)
	parent := mocks.NewMockWebView(t)
	parent.EXPECT().ID().Return(port.WebViewID(101)).Once()
	popup := &deferredPopupWebViewStub{MockWebView: mocks.NewMockWebView(t), features: dto.PopupFeatures{State: dto.PopupFeaturesSpecified, Width: 640, WidthSet: true}}
	popup.EXPECT().ID().Return(port.WebViewID(206)).Maybe()
	popup.EXPECT().Destroy().Once()
	factory := mocks.NewMockWebViewFactory(t)
	factory.EXPECT().CreateRelated(mock.Anything, port.WebViewID(101)).Return(popup, nil).Once()
	c := &Coordinator{popups: newPopupManager()}
	c.SetPopupConfig(factory, &entity.BrowsingContextConfig{OpenInNewPane: false}, nil)
	c.SetPopupSourceHostResolver(func(entity.PaneID) dto.SourceHostKind { return dto.SourceHostFloating })
	c.SetOnStagePopup(func(context.Context, StagePopupInput) (PopupStagingHost, error) { return &popupStagingHostStub{}, nil })
	c.SetOnOpenNativePopup(func(abortCtx context.Context, input NativePopupInput) error {
		assert.False(t, input.AllowBrowserWindowFallback)
		if !input.OnNativeHostAbort(abortCtx, input.PopupWebView) {
			input.PopupWebView.Destroy()
		}
		return nil
	})

	require.Same(t, popup, c.handlePopupCreate(ctx, "floating", parent, port.PopupRequest{
		Engine: dto.BrowserEngineWebKit, TargetURI: "https://example.com/visual", TargetDisposition: dto.WindowDispositionNewPopup,
		PopupFeatures: dto.PopupFeatures{State: dto.PopupFeaturesUnknown},
	}))
	popup.ready()

	record := decodeBrowsingContextLog(t, output.Bytes())
	assert.Equal(t, "webkit", record["engine"])
	assert.Equal(t, "floating", record["source_host"])
	assert.Equal(t, "create-native-popup", record["decision"])
	assert.Equal(t, "new-popup", record["target_disposition"])
	assert.Equal(t, "native-arm-failed", record["reason_code"])
}

func TestPopupDeferredBrowserHostFailureCleansUpExactlyOnce(t *testing.T) {
	parent := mocks.NewMockWebView(t)
	parent.EXPECT().ID().Return(port.WebViewID(101)).Once()
	popup := &deferredPopupWebViewStub{MockWebView: mocks.NewMockWebView(t), features: dto.PopupFeatures{State: dto.PopupFeaturesNone}}
	popup.EXPECT().ID().Return(port.WebViewID(204)).Maybe()
	popup.EXPECT().Generation().Return(uint64(1)).Maybe()
	popup.EXPECT().SetCallbacks(mock.Anything).Once()
	popup.EXPECT().Destroy().Once()
	factory := mocks.NewMockWebViewFactory(t)
	factory.EXPECT().CreateRelated(mock.Anything, port.WebViewID(101)).Return(popup, nil).Once()
	staging := &popupStagingHostStub{}
	c := &Coordinator{popups: newPopupManager()}
	c.SetPopupConfig(factory, &entity.BrowsingContextConfig{OpenInNewPane: false}, nil)
	c.SetPopupSourceHostResolver(func(entity.PaneID) dto.SourceHostKind { return dto.SourceHostFloating })
	c.SetOnStagePopup(func(context.Context, StagePopupInput) (PopupStagingHost, error) { return staging, nil })
	c.SetOnOpenBrowserWindow(func(context.Context, BrowserWindowInput) (BrowserWindowResult, error) {
		return BrowserWindowResult{}, errors.New("host failed")
	})

	require.Same(t, popup, c.handlePopupCreate(context.Background(), "floating", parent, port.PopupRequest{
		Engine: dto.BrowserEngineWebKit, TargetURI: "https://example.com/open", FrameName: "shared",
		TargetDisposition: dto.WindowDispositionNewPopup, PopupFeatures: dto.PopupFeatures{State: dto.PopupFeaturesUnknown},
	}))
	popup.ready()
	popup.ready()
	assert.True(t, staging.detached)
	assert.Equal(t, 1, staging.destroyCalls)
}

func TestPopupDeferredStagingDetachFailureCleansUpExactlyOnce(t *testing.T) {
	parent := mocks.NewMockWebView(t)
	parent.EXPECT().ID().Return(port.WebViewID(101)).Once()
	popup := &deferredPopupWebViewStub{MockWebView: mocks.NewMockWebView(t), features: dto.PopupFeatures{State: dto.PopupFeaturesNone}}
	popup.EXPECT().ID().Return(port.WebViewID(209)).Maybe()
	popup.EXPECT().Destroy().Once()
	factory := mocks.NewMockWebViewFactory(t)
	factory.EXPECT().CreateRelated(mock.Anything, port.WebViewID(101)).Return(popup, nil).Once()
	staging := &popupStagingHostStub{detachErr: errors.New("detach failed")}
	c := &Coordinator{popups: newPopupManager()}
	c.SetPopupConfig(factory, &entity.BrowsingContextConfig{OpenInNewPane: false}, nil)
	c.SetPopupSourceHostResolver(func(entity.PaneID) dto.SourceHostKind { return dto.SourceHostFloating })
	c.SetOnStagePopup(func(context.Context, StagePopupInput) (PopupStagingHost, error) { return staging, nil })

	require.Same(t, popup, c.handlePopupCreate(context.Background(), "floating", parent, port.PopupRequest{
		Engine: dto.BrowserEngineWebKit, TargetURI: "https://example.com/open",
		PopupFeatures: dto.PopupFeatures{State: dto.PopupFeaturesUnknown},
	}))
	popup.ready()
	popup.ready()

	assert.True(t, staging.detached)
	assert.Equal(t, 1, staging.destroyCalls)
}

func TestPopupDeferredStagingFailureDestroysCallerOwnedWebViewOnce(t *testing.T) {
	parent := mocks.NewMockWebView(t)
	parent.EXPECT().ID().Return(port.WebViewID(101)).Once()
	popup := &deferredPopupWebViewStub{MockWebView: mocks.NewMockWebView(t)}
	popup.EXPECT().Destroy().Once()
	factory := mocks.NewMockWebViewFactory(t)
	factory.EXPECT().CreateRelated(mock.Anything, port.WebViewID(101)).Return(popup, nil).Once()
	c := &Coordinator{popups: newPopupManager()}
	c.SetPopupConfig(factory, &entity.BrowsingContextConfig{OpenInNewPane: false}, nil)
	c.SetPopupSourceHostResolver(func(entity.PaneID) dto.SourceHostKind { return dto.SourceHostFloating })
	c.SetOnStagePopup(func(context.Context, StagePopupInput) (PopupStagingHost, error) {
		return nil, errors.New("stage failed")
	})

	assert.Nil(t, c.handlePopupCreate(context.Background(), "floating", parent, port.PopupRequest{Engine: dto.BrowserEngineWebKit, TargetURI: "https://example.com/open", PopupFeatures: dto.PopupFeatures{State: dto.PopupFeaturesUnknown}}))
}

func TestPopupDeferredExplicitTeardownCleansUpWithoutLifecycleSignals(t *testing.T) {
	for _, tc := range []struct {
		name    string
		prepare func(*Coordinator)
		trigger func(*Coordinator)
	}{
		{name: "parent release", trigger: func(c *Coordinator) { c.ReleaseWebView(context.Background(), "floating") }},
		{name: "owner window teardown", prepare: func(c *Coordinator) {
			c.SetPopupWindowIDResolver(func(entity.PaneID) (string, bool) { return "owner", true })
		}, trigger: func(c *Coordinator) { c.ClearPopupNamedContextsForWindow("owner") }},
		{name: "coordinator shutdown", trigger: func(c *Coordinator) { c.ShutdownPopups() }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parent := mocks.NewMockWebView(t)
			parent.EXPECT().ID().Return(port.WebViewID(101)).Once()
			popup := &deferredPopupWebViewStub{MockWebView: mocks.NewMockWebView(t), features: dto.PopupFeatures{State: dto.PopupFeaturesNone}}
			popup.EXPECT().ID().Return(port.WebViewID(207)).Maybe()
			popup.EXPECT().Destroy().Once()
			factory := mocks.NewMockWebViewFactory(t)
			factory.EXPECT().CreateRelated(mock.Anything, port.WebViewID(101)).Return(popup, nil).Once()
			staging := &popupStagingHostStub{}
			c := &Coordinator{popups: newPopupManager()}
			c.SetPopupConfig(factory, &entity.BrowsingContextConfig{OpenInNewPane: false}, nil)
			c.SetPopupSourceHostResolver(func(entity.PaneID) dto.SourceHostKind { return dto.SourceHostFloating })
			c.SetOnStagePopup(func(context.Context, StagePopupInput) (PopupStagingHost, error) { return staging, nil })
			if tc.prepare != nil {
				tc.prepare(c)
			}

			require.Same(t, popup, c.handlePopupCreate(context.Background(), "floating", parent, port.PopupRequest{
				Engine: dto.BrowserEngineWebKit, TargetURI: "https://example.com/open",
				PopupFeatures: dto.PopupFeatures{State: dto.PopupFeaturesUnknown},
			}))
			require.Len(t, c.popups.deferredPopups, 1)
			tc.trigger(c)
			tc.trigger(c)

			assert.Empty(t, c.popups.deferredPopups)
			assert.Empty(t, c.popups.pendingPopups)
			assert.Equal(t, 1, staging.destroyCalls)
			assert.Nil(t, popup.onReady)
			assert.Nil(t, popup.onClose)
		})
	}
}

func TestPopupDeferredUnknownAndCloseCleanupExactlyOnce(t *testing.T) {
	for _, tc := range []struct {
		name  string
		close bool
	}{{name: "unknown at ready"}, {name: "close before ready", close: true}} {
		t.Run(tc.name, func(t *testing.T) {
			parent := mocks.NewMockWebView(t)
			parent.EXPECT().ID().Return(port.WebViewID(101)).Once()
			popup := &deferredPopupWebViewStub{MockWebView: mocks.NewMockWebView(t), features: dto.PopupFeatures{State: dto.PopupFeaturesUnknown}}
			popup.EXPECT().ID().Return(port.WebViewID(203)).Maybe()
			popup.EXPECT().Destroy().Once()
			factory := mocks.NewMockWebViewFactory(t)
			factory.EXPECT().CreateRelated(mock.Anything, port.WebViewID(101)).Return(popup, nil).Once()
			staging := &popupStagingHostStub{}
			c := &Coordinator{popups: newPopupManager()}
			c.SetPopupConfig(factory, &entity.BrowsingContextConfig{OpenInNewPane: false}, nil)
			c.SetPopupSourceHostResolver(func(entity.PaneID) dto.SourceHostKind { return dto.SourceHostFloating })
			c.SetOnStagePopup(func(context.Context, StagePopupInput) (PopupStagingHost, error) { return staging, nil })
			require.Same(t, popup, c.handlePopupCreate(context.Background(), "floating", parent, port.PopupRequest{Engine: dto.BrowserEngineWebKit, TargetURI: "https://example.com/open", PopupFeatures: dto.PopupFeatures{State: dto.PopupFeaturesUnknown}}))
			if tc.close {
				popup.close()
				popup.close()
			} else {
				popup.ready()
				popup.ready()
			}
			assert.Equal(t, 1, staging.destroyCalls)
		})
	}
}

// deferredPopupWebViewStub exposes the optional late-feature and lifecycle ports.
type deferredPopupWebViewStub struct {
	*mocks.MockWebView
	features                   dto.PopupFeatures
	onReady                    func()
	onClose                    func()
	browsingContextDecision    dto.HostDecision
	hasBrowsingContextDecision bool
}

func (s *deferredPopupWebViewStub) ResolvePopupFeatures() dto.PopupFeatures { return s.features }
func (s *deferredPopupWebViewStub) PrimePopupNavigation(string)             {}
func (s *deferredPopupWebViewStub) SetOnReadyToShow(fn func())              { s.onReady = fn }
func (s *deferredPopupWebViewStub) SetOnClose(fn func())                    { s.onClose = fn }
func (s *deferredPopupWebViewStub) Show()                                   {}
func (s *deferredPopupWebViewStub) SetBrowsingContextHostDecision(decision dto.HostDecision) {
	s.browsingContextDecision = decision
	s.hasBrowsingContextDecision = true
}
func (s *deferredPopupWebViewStub) BrowsingContextHostDecision() (dto.HostDecision, bool) {
	return s.browsingContextDecision, s.hasBrowsingContextDecision
}
func (s *deferredPopupWebViewStub) ready() {
	if s.onReady != nil {
		s.onReady()
	}
}
func (s *deferredPopupWebViewStub) close() {
	if s.onClose != nil {
		s.onClose()
	}
}

type popupStagingHostStub struct {
	attached, detached bool
	destroyCalls       int
	detachErr          error
}

func (s *popupStagingHostStub) Detach() error { s.detached = true; return s.detachErr }
func (s *popupStagingHostStub) Destroy()      { s.destroyCalls++ }
