package content

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/application/port/mocks"
	"github.com/bnema/dumber/internal/domain/entity"
	domainerrors "github.com/bnema/dumber/internal/domain/errors"
)

// ---------------------------------------------------------------------------
// DetectPopupType
// ---------------------------------------------------------------------------

func TestDetectPopupType_BlankIsTab(t *testing.T) {
	t.Parallel()

	assert.Equal(t, PopupTypeTab, DetectPopupType("_blank"))
}

func TestDetectPopupType_EmptyIsPopup(t *testing.T) {
	t.Parallel()

	assert.Equal(t, PopupTypePopup, DetectPopupType(""))
}

func TestDetectPopupType_NamedFrameIsPopup(t *testing.T) {
	t.Parallel()

	assert.Equal(t, PopupTypePopup, DetectPopupType("myFrame"))
}

// ---------------------------------------------------------------------------
// PopupType.String()
// ---------------------------------------------------------------------------

func TestPopupTypeString_Tab(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "tab", PopupTypeTab.String())
}

func TestPopupTypeString_Popup(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "popup", PopupTypePopup.String())
}

func TestPopupTypeString_Unknown(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "unknown", PopupType(99).String())
}

// ---------------------------------------------------------------------------
// GetBehavior
// ---------------------------------------------------------------------------

func TestGetBehavior_NilConfigReturnsSplit(t *testing.T) {
	t.Parallel()

	assert.Equal(t, entity.PopupBehaviorSplit, GetBehavior(PopupTypeTab, nil))
	assert.Equal(t, entity.PopupBehaviorSplit, GetBehavior(PopupTypePopup, nil))
}

func TestGetBehavior_TabPopup_DefaultConfig(t *testing.T) {
	t.Parallel()

	// BlankTargetBehavior is empty → falls through to default "stacked"
	cfg := &entity.PopupBehaviorConfig{}
	assert.Equal(t, entity.PopupBehaviorStacked, GetBehavior(PopupTypeTab, cfg))
}

func TestGetBehavior_TabPopup_SplitConfig(t *testing.T) {
	t.Parallel()

	cfg := &entity.PopupBehaviorConfig{BlankTargetBehavior: "split"}
	assert.Equal(t, entity.PopupBehaviorSplit, GetBehavior(PopupTypeTab, cfg))
}

func TestGetBehavior_TabPopup_StackedConfig(t *testing.T) {
	t.Parallel()

	cfg := &entity.PopupBehaviorConfig{BlankTargetBehavior: "stacked"}
	assert.Equal(t, entity.PopupBehaviorStacked, GetBehavior(PopupTypeTab, cfg))
}

func TestGetBehavior_TabPopup_TabbedConfig(t *testing.T) {
	t.Parallel()

	cfg := &entity.PopupBehaviorConfig{BlankTargetBehavior: "tabbed"}
	assert.Equal(t, entity.PopupBehaviorTabbed, GetBehavior(PopupTypeTab, cfg))
}

func TestGetBehavior_JSPopup_UsesBehaviorField(t *testing.T) {
	t.Parallel()

	cfg := &entity.PopupBehaviorConfig{Behavior: entity.PopupBehaviorWindowed}
	assert.Equal(t, entity.PopupBehaviorWindowed, GetBehavior(PopupTypePopup, cfg))
}

func TestGetBehavior_JSPopup_DefaultConfig(t *testing.T) {
	t.Parallel()

	// Behavior zero-value is empty string; GetBehavior returns it as-is.
	cfg := &entity.PopupBehaviorConfig{}
	assert.Equal(t, entity.PopupBehavior(""), GetBehavior(PopupTypePopup, cfg))
}

func TestGetBehavior_TabPopup_BlocksWhenOpenInNewPaneFalse(t *testing.T) {
	t.Parallel()

	// GetBehavior itself does not honor OpenInNewPane — that is enforced by
	// handlePopupCreate. GetBehavior still returns the configured value.
	cfg := &entity.PopupBehaviorConfig{
		OpenInNewPane:       false,
		BlankTargetBehavior: "split",
	}
	assert.Equal(t, entity.PopupBehaviorSplit, GetBehavior(PopupTypeTab, cfg))
}

func newPopupCreateCoordinatorForTest(t *testing.T, popupID port.WebViewID) (context.Context, entity.PaneID, *mocks.MockWebView, *mocks.MockWebView, *Coordinator) {
	t.Helper()

	ctx := context.Background()
	parentPaneID := entity.PaneID("parent-pane")
	parentWV := mocks.NewMockWebView(t)
	parentWV.EXPECT().ID().Return(port.WebViewID(101)).Once()

	popupWV := mocks.NewMockWebView(t)
	popupWV.EXPECT().ID().Return(popupID).Once()

	factory := mocks.NewMockWebViewFactory(t)
	factory.EXPECT().Create(mock.Anything).Return(popupWV, nil).Once()

	c := &Coordinator{
		webViews: make(map[entity.PaneID]port.WebView),
		popups:   newPopupManager(),
	}
	c.SetPopupConfig(factory, nil, func() string { return "popup-pane" })

	return ctx, parentPaneID, parentWV, popupWV, c
}

func TestHandlePopupCreate_PrimesPopupNavigationCapability(t *testing.T) {
	ctx := context.Background()
	parentPaneID := entity.PaneID("parent-pane")
	parentWV := mocks.NewMockWebView(t)
	parentWV.EXPECT().ID().Return(port.WebViewID(101)).Once()

	popupWV := &popupNavigationWebViewStub{MockWebView: mocks.NewMockWebView(t)}
	popupWV.EXPECT().ID().Return(port.WebViewID(149)).Once()
	popupWV.EXPECT().Generation().Return(uint64(1)).Once()
	popupWV.EXPECT().SetCallbacks(mock.Anything).Once()

	factory := mocks.NewMockWebViewFactory(t)
	factory.EXPECT().Create(mock.Anything).Return(popupWV, nil).Once()

	c := &Coordinator{
		webViews: make(map[entity.PaneID]port.WebView),
		popups:   newPopupManager(),
	}
	c.SetPopupConfig(factory, nil, nil)
	c.SetOnInsertPopup(func(_ context.Context, input InsertPopupInput) error {
		return nil
	})

	created := c.handlePopupCreate(ctx, parentPaneID, parentWV, port.PopupRequest{
		TargetURI:          "https://example.com/popup",
		FrameName:          "auth-popup",
		IsUserGesture:      true,
		NoJavaScriptAccess: true,
	})

	require.Same(t, popupWV, created)
	require.Equal(t, []string{"https://example.com/popup"}, popupWV.primed)
}

func TestHandlePopupCreate_RegistersPopupWebViewBeforeWorkspaceInsertion(t *testing.T) {
	ctx, parentPaneID, parentWV, popupWV, c := newPopupCreateCoordinatorForTest(t, port.WebViewID(150))
	popupWV.EXPECT().Generation().Return(uint64(1)).Once()
	popupWV.EXPECT().SetCallbacks(mock.Anything).Once()
	popupWV.EXPECT().IsLoading().Return(false).Once()
	popupWV.EXPECT().URI().Return("").Once()
	popupWV.EXPECT().LoadURI(mock.Anything, "https://example.com/popup").Return(nil).Once()

	c.SetOnInsertPopup(func(_ context.Context, input InsertPopupInput) error {
		require.Equal(t, entity.PaneID("popup-pane"), input.PopupPane.ID)
		require.Same(t, popupWV, c.getWebViewLocked(input.PopupPane.ID))
		return nil
	})

	created := c.handlePopupCreate(ctx, parentPaneID, parentWV, port.PopupRequest{
		TargetURI:          "https://example.com/popup",
		FrameName:          "auth-popup",
		IsUserGesture:      true,
		NoJavaScriptAccess: true,
	})

	require.Same(t, popupWV, created)
}

func TestHandlePopupCreate_CleansUpPreRegisteredPopupWebViewWhenInsertionFails(t *testing.T) {
	ctx, parentPaneID, parentWV, popupWV, c := newPopupCreateCoordinatorForTest(t, port.WebViewID(151))
	popupWV.EXPECT().Generation().Return(uint64(1)).Once()
	popupWV.EXPECT().SetCallbacks(mock.Anything).Once()
	popupWV.EXPECT().Destroy().Once()

	c.SetOnInsertPopup(func(_ context.Context, input InsertPopupInput) error {
		require.Same(t, popupWV, c.getWebViewLocked(input.PopupPane.ID))
		return assert.AnError
	})

	created := c.handlePopupCreate(ctx, parentPaneID, parentWV, port.PopupRequest{
		TargetURI:          "https://example.com/popup",
		FrameName:          "auth-popup",
		IsUserGesture:      true,
		NoJavaScriptAccess: true,
	})

	require.Nil(t, created)
	require.Nil(t, c.getWebViewLocked(entity.PaneID("popup-pane")))
}

func TestHandlePopupCreate_UsesRegularWebViewWhenPopupDisablesJavaScriptAccess(t *testing.T) {
	ctx := context.Background()
	parentPaneID := entity.PaneID("parent-pane")
	parentWV := mocks.NewMockWebView(t)
	parentWV.EXPECT().ID().Return(port.WebViewID(101)).Once()

	popupWV := mocks.NewMockWebView(t)
	popupWV.EXPECT().ID().Return(port.WebViewID(150)).Once()
	popupWV.EXPECT().Generation().Return(uint64(1)).Once()
	popupWV.EXPECT().SetCallbacks(mock.Anything).Once()
	popupWV.EXPECT().IsLoading().Return(false).Once()
	popupWV.EXPECT().URI().Return("").Once()
	popupWV.EXPECT().LoadURI(mock.Anything, "https://example.com/noopener").Return(nil).Once()

	factory := mocks.NewMockWebViewFactory(t)
	factory.EXPECT().Create(mock.Anything).Return(popupWV, nil).Once()

	c := &Coordinator{
		webViews: make(map[entity.PaneID]port.WebView),
		popups:   newPopupManager(),
	}
	c.SetPopupConfig(factory, nil, nil)
	c.SetOnInsertPopup(func(_ context.Context, input InsertPopupInput) error {
		assert.Equal(t, "https://example.com/noopener", input.TargetURI)
		return nil
	})

	created := c.handlePopupCreate(ctx, parentPaneID, parentWV, port.PopupRequest{
		TargetURI:          "https://example.com/noopener",
		FrameName:          "auth-popup",
		IsUserGesture:      true,
		NoJavaScriptAccess: true,
	})

	require.Same(t, popupWV, created)
}

func TestHandlePopupCreate_SkipsRelatedCreateAfterUnsupportedFactoryError(t *testing.T) {
	ctx := context.Background()
	parentPaneID := entity.PaneID("parent-pane")
	parentWV := mocks.NewMockWebView(t)
	parentWV.EXPECT().ID().Return(port.WebViewID(101)).Twice()

	firstPopupWV := mocks.NewMockWebView(t)
	firstPopupWV.EXPECT().ID().Return(port.WebViewID(202)).Once()
	firstPopupWV.EXPECT().Generation().Return(uint64(1)).Once()
	firstPopupWV.EXPECT().SetCallbacks(mock.Anything).Once()
	firstPopupWV.EXPECT().IsLoading().Return(false).Once()
	firstPopupWV.EXPECT().URI().Return("").Once()
	firstPopupWV.EXPECT().LoadURI(mock.Anything, "https://example.com/first").Return(nil).Once()

	secondPopupWV := mocks.NewMockWebView(t)
	secondPopupWV.EXPECT().ID().Return(port.WebViewID(203)).Once()
	secondPopupWV.EXPECT().Generation().Return(uint64(1)).Once()
	secondPopupWV.EXPECT().SetCallbacks(mock.Anything).Once()
	secondPopupWV.EXPECT().IsLoading().Return(false).Once()
	secondPopupWV.EXPECT().URI().Return("").Once()
	secondPopupWV.EXPECT().LoadURI(mock.Anything, "https://example.com/second").Return(nil).Once()

	factory := mocks.NewMockWebViewFactory(t)
	factory.EXPECT().CreateRelated(mock.Anything, port.WebViewID(101)).Return(nil, domainerrors.ErrRelatedWebViewUnsupported).Once()
	factory.EXPECT().Create(mock.Anything).Return(firstPopupWV, nil).Once()
	factory.EXPECT().Create(mock.Anything).Return(secondPopupWV, nil).Once()

	c := &Coordinator{
		webViews: make(map[entity.PaneID]port.WebView),
		popups:   newPopupManager(),
	}
	c.SetPopupConfig(factory, nil, nil)
	c.SetOnInsertPopup(func(_ context.Context, input InsertPopupInput) error {
		return nil
	})

	first := c.handlePopupCreate(ctx, parentPaneID, parentWV, port.PopupRequest{
		TargetURI:     "https://example.com/first",
		FrameName:     "_blank",
		IsUserGesture: true,
	})
	second := c.handlePopupCreate(ctx, parentPaneID, parentWV, port.PopupRequest{
		TargetURI:     "https://example.com/second",
		FrameName:     "_blank",
		IsUserGesture: true,
	})

	require.Same(t, firstPopupWV, first)
	require.Same(t, secondPopupWV, second)
}

func TestHandlePopupCreate_FallsBackToRegularWebViewWhenRelatedCreateIsUnsupported(t *testing.T) {
	ctx := context.Background()
	parentPaneID := entity.PaneID("parent-pane")
	parentWV := mocks.NewMockWebView(t)
	parentWV.EXPECT().ID().Return(port.WebViewID(101)).Once()

	popupWV := mocks.NewMockWebView(t)
	popupWV.EXPECT().ID().Return(port.WebViewID(202)).Once()
	popupWV.EXPECT().Generation().Return(uint64(1)).Once()
	popupWV.EXPECT().SetCallbacks(mock.Anything).Once()
	popupWV.EXPECT().IsLoading().Return(false).Once()
	popupWV.EXPECT().URI().Return("").Once()
	popupWV.EXPECT().LoadURI(mock.Anything, "https://example.com/edit").Return(nil).Once()

	factory := mocks.NewMockWebViewFactory(t)
	factory.EXPECT().CreateRelated(mock.Anything, port.WebViewID(101)).Return(nil, domainerrors.ErrRelatedWebViewUnsupported).Once()
	factory.EXPECT().Create(mock.Anything).Return(popupWV, nil).Once()

	insertCalls := 0
	c := &Coordinator{
		webViews: make(map[entity.PaneID]port.WebView),
		popups:   newPopupManager(),
	}
	c.SetPopupConfig(factory, nil, nil)
	c.SetOnInsertPopup(func(_ context.Context, input InsertPopupInput) error {
		insertCalls++
		assert.Equal(t, parentPaneID, input.ParentPaneID)
		assert.Equal(t, popupWV, input.WebView)
		assert.Equal(t, "https://example.com/edit", input.TargetURI)
		return nil
	})

	created := c.handlePopupCreate(ctx, parentPaneID, parentWV, port.PopupRequest{
		TargetURI:     "https://example.com/edit",
		FrameName:     "_blank",
		IsUserGesture: true,
	})

	require.Same(t, popupWV, created)
	assert.Equal(t, 1, insertCalls)
}

func TestHandlePopupCreate_FallsBackToRegularWebViewWhenRelatedCreateFailsUnexpectedly(t *testing.T) {
	ctx := context.Background()
	parentPaneID := entity.PaneID("parent-pane")
	parentWV := mocks.NewMockWebView(t)
	parentWV.EXPECT().ID().Return(port.WebViewID(101)).Once()

	popupWV := mocks.NewMockWebView(t)
	popupWV.EXPECT().ID().Return(port.WebViewID(204)).Once()
	popupWV.EXPECT().Generation().Return(uint64(1)).Once()
	popupWV.EXPECT().SetCallbacks(mock.Anything).Once()
	popupWV.EXPECT().IsLoading().Return(false).Once()
	popupWV.EXPECT().URI().Return("").Once()
	popupWV.EXPECT().LoadURI(mock.Anything, "https://example.com/edit").Return(nil).Once()

	factory := mocks.NewMockWebViewFactory(t)
	factory.EXPECT().CreateRelated(mock.Anything, port.WebViewID(101)).Return(nil, errors.New("boom")).Once()
	factory.EXPECT().Create(mock.Anything).Return(popupWV, nil).Once()

	insertCalls := 0
	c := &Coordinator{
		webViews: make(map[entity.PaneID]port.WebView),
		popups:   newPopupManager(),
	}
	c.SetPopupConfig(factory, nil, nil)
	c.SetOnInsertPopup(func(_ context.Context, input InsertPopupInput) error {
		insertCalls++
		assert.Equal(t, popupWV, input.WebView)
		return nil
	})

	created := c.handlePopupCreate(ctx, parentPaneID, parentWV, port.PopupRequest{
		TargetURI:     "https://example.com/edit",
		FrameName:     "_blank",
		IsUserGesture: true,
	})

	require.Same(t, popupWV, created)
	assert.Equal(t, 1, insertCalls)
}

func TestHandlePopupCreate_PreservesOriginalTargetWhenRelatedCreateIsUnsupported(t *testing.T) {
	ctx := context.Background()
	parentPaneID := entity.PaneID("parent-pane")
	parentWV := mocks.NewMockWebView(t)
	parentWV.EXPECT().ID().Return(port.WebViewID(101)).Once()

	popupWV := &popupOpenerBridgeWebViewStub{MockWebView: mocks.NewMockWebView(t)}
	popupWV.EXPECT().ID().Return(port.WebViewID(303)).Once()
	popupWV.EXPECT().Generation().Return(uint64(1)).Once()
	popupWV.EXPECT().SetCallbacks(mock.Anything).Once()
	popupWV.EXPECT().IsLoading().Return(false).Once()
	popupWV.EXPECT().URI().Return("").Once()
	popupWV.EXPECT().LoadURI(mock.Anything, "https://example.com/popup?redirect=%2Fcallback").Return(nil).Once()

	factory := mocks.NewMockWebViewFactory(t)
	factory.EXPECT().CreateRelated(mock.Anything, port.WebViewID(101)).Return(nil, domainerrors.ErrRelatedWebViewUnsupported).Once()
	factory.EXPECT().Create(mock.Anything).Return(popupWV, nil).Once()

	insertCalls := 0
	c := &Coordinator{
		webViews: make(map[entity.PaneID]port.WebView),
		popups:   newPopupManager(),
	}
	c.SetPopupConfig(factory, nil, nil)
	c.SetOnInsertPopup(func(_ context.Context, input InsertPopupInput) error {
		insertCalls++
		assert.Equal(t, "https://example.com/popup?redirect=%2Fcallback", input.TargetURI)
		return nil
	})

	created := c.handlePopupCreate(ctx, parentPaneID, parentWV, port.PopupRequest{
		TargetURI:     "https://example.com/popup?redirect=%2Fcallback",
		FrameName:     "auth-popup",
		IsUserGesture: true,
	})

	require.Same(t, popupWV, created)
	assert.Equal(t, 1, insertCalls)
	assert.Same(t, parentWV, popupWV.parent)
	assert.False(t, popupWV.noJavaScriptAccess)
}

func TestHandlePopupCreate_ReusesNamedPopup(t *testing.T) {
	ctx := context.Background()
	parentPaneID := entity.PaneID("parent-pane")
	parentWV := mocks.NewMockWebView(t)
	parentWV.EXPECT().ID().Return(port.WebViewID(101)).Twice()

	popupWV := mocks.NewMockWebView(t)
	popupWV.EXPECT().ID().Return(port.WebViewID(202)).Twice()
	popupWV.EXPECT().Generation().Return(uint64(1)).Maybe()
	popupWV.EXPECT().SetCallbacks(mock.Anything).Maybe()
	popupWV.EXPECT().IsLoading().Return(false).Maybe()
	currentURI := ""
	popupWV.EXPECT().URI().RunAndReturn(func() string { return currentURI }).Maybe()
	popupWV.EXPECT().IsDestroyed().Return(false).Maybe()
	popupWV.EXPECT().LoadURI(mock.Anything, "https://accounts.google.com/o/oauth2/v2/auth?first").RunAndReturn(func(context.Context, string) error {
		currentURI = "https://accounts.google.com/o/oauth2/v2/auth?first"
		return nil
	}).Once()
	popupWV.EXPECT().LoadURI(mock.Anything, "https://accounts.google.com/o/oauth2/v2/auth?second").RunAndReturn(func(context.Context, string) error {
		currentURI = "https://accounts.google.com/o/oauth2/v2/auth?second"
		return nil
	}).Once()

	factory := mocks.NewMockWebViewFactory(t)
	factory.EXPECT().CreateRelated(mock.Anything, port.WebViewID(101)).Return(popupWV, nil).Once()

	insertCalls := 0
	c := &Coordinator{
		webViews: make(map[entity.PaneID]port.WebView),
		popups:   newPopupManager(),
	}
	c.SetPopupConfig(factory, nil, nil)
	c.SetOnInsertPopup(func(context.Context, InsertPopupInput) error {
		insertCalls++
		return nil
	})

	first := c.handlePopupCreate(ctx, parentPaneID, parentWV, port.PopupRequest{
		TargetURI:     "https://accounts.google.com/o/oauth2/v2/auth?first",
		FrameName:     "g_credential_picker_x",
		IsUserGesture: true,
	})
	require.Same(t, popupWV, first)

	second := c.handlePopupCreate(ctx, parentPaneID, parentWV, port.PopupRequest{
		TargetURI:     "https://accounts.google.com/o/oauth2/v2/auth?second",
		FrameName:     "g_credential_picker_x",
		IsUserGesture: true,
	})
	require.Same(t, popupWV, second)

	assert.Equal(t, 1, insertCalls)
	assert.Equal(t, "https://accounts.google.com/o/oauth2/v2/auth?second", currentURI)
}

// ---------------------------------------------------------------------------
// trackOAuthPopup / capturePopupOAuthState
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
	cfg := &entity.PopupBehaviorConfig{Behavior: entity.PopupBehaviorSplit}
	genID := func() string { return "test-id" }

	c.SetPopupConfig(factory, cfg, genID)

	assert.Equal(t, factory, c.popups.factory)
	assert.Equal(t, cfg, c.popups.popupConfig)
	assert.NotNil(t, c.popups.generatePaneID)
	assert.Equal(t, "test-id", c.popups.generatePaneID())
}

func TestSetPopupConfig_NilConfigAllowed(t *testing.T) {
	t.Parallel()

	c := &Coordinator{}
	c.SetPopupConfig(nil, nil, nil)

	assert.Nil(t, c.popups.factory)
	assert.Nil(t, c.popups.popupConfig)
	assert.Nil(t, c.popups.generatePaneID)
}

type popupNavigationWebViewStub struct {
	*mocks.MockWebView
	primed []string
}

func (s *popupNavigationWebViewStub) PrimePopupNavigation(uri string) {
	s.primed = append(s.primed, uri)
}

func (*popupNavigationWebViewStub) SetOnReadyToShow(func()) {}
func (*popupNavigationWebViewStub) SetOnClose(func())       {}
func (*popupNavigationWebViewStub) Show()                   {}

type popupOpenerBridgeWebViewStub struct {
	*mocks.MockWebView
	parent             port.WebView
	noJavaScriptAccess bool
}

func (s *popupOpenerBridgeWebViewStub) EnablePopupOpenerBridge(parent port.WebView, noJavaScriptAccess bool) {
	s.parent = parent
	s.noJavaScriptAccess = noJavaScriptAccess
}

func (*popupOpenerBridgeWebViewStub) AddOpenerMessageCallback(func()) {}
func (*popupOpenerBridgeWebViewStub) AddOpenerNavigationCallback(func(string)) {
}
func (*popupOpenerBridgeWebViewStub) HasActivePopupOpenerBridge() bool { return false }

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
	for i := 0; i < workers; i++ {
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
	for i := 0; i < workers; i++ {
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
	for i := 0; i < 10; i++ {
		c.popups.pendingPopups[port.WebViewID(i)] = &PendingPopup{PopupType: PopupTypePopup}
	}

	var wg sync.WaitGroup
	wg.Add(20)

	// Deleters
	for i := 0; i < 10; i++ {
		go func(id int) {
			defer wg.Done()
			c.popups.mu.Lock()
			delete(c.popups.pendingPopups, port.WebViewID(id))
			c.popups.mu.Unlock()
		}(i)
	}

	// Readers
	for i := 0; i < 10; i++ {
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
