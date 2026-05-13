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
	cfg := &entity.BrowsingContextConfig{}
	assert.Equal(t, entity.PopupBehaviorStacked, GetBehavior(PopupTypeTab, cfg))
}

func TestGetBehavior_TabPopup_SplitConfig(t *testing.T) {
	t.Parallel()

	cfg := &entity.BrowsingContextConfig{BlankTargetBehavior: "split"}
	assert.Equal(t, entity.PopupBehaviorSplit, GetBehavior(PopupTypeTab, cfg))
}

func TestGetBehavior_TabPopup_StackedConfig(t *testing.T) {
	t.Parallel()

	cfg := &entity.BrowsingContextConfig{BlankTargetBehavior: "stacked"}
	assert.Equal(t, entity.PopupBehaviorStacked, GetBehavior(PopupTypeTab, cfg))
}

func TestGetBehavior_TabPopup_TabbedConfig(t *testing.T) {
	t.Parallel()

	cfg := &entity.BrowsingContextConfig{BlankTargetBehavior: "tabbed"}
	assert.Equal(t, entity.PopupBehaviorTabbed, GetBehavior(PopupTypeTab, cfg))
}

func TestGetBehavior_JSPopup_UsesBehaviorField(t *testing.T) {
	t.Parallel()

	cfg := &entity.BrowsingContextConfig{Behavior: entity.PopupBehaviorWindowed}
	assert.Equal(t, entity.PopupBehaviorWindowed, GetBehavior(PopupTypePopup, cfg))
}

func TestGetBehavior_JSPopup_DefaultConfig(t *testing.T) {
	t.Parallel()

	// Behavior zero-value is empty string; GetBehavior returns it as-is.
	cfg := &entity.BrowsingContextConfig{}
	assert.Equal(t, entity.PopupBehavior(""), GetBehavior(PopupTypePopup, cfg))
}

func TestGetBehavior_TabPopup_BlocksWhenOpenInNewPaneFalse(t *testing.T) {
	t.Parallel()

	// GetBehavior itself does not honor OpenInNewPane — that is enforced by
	// handlePopupCreate. GetBehavior still returns the configured value.
	cfg := &entity.BrowsingContextConfig{
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
	factory.EXPECT().CreateRelated(mock.Anything, port.WebViewID(101)).Return(popupWV, nil).Once()

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
	factory.EXPECT().CreateRelated(mock.Anything, port.WebViewID(101)).Return(popupWV, nil).Once()

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

func TestHandlePopupCreate_UsesRelatedWebViewWhenPopupDisablesJavaScriptAccess(t *testing.T) {
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
	factory.EXPECT().CreateRelated(mock.Anything, port.WebViewID(101)).Return(popupWV, nil).Once()

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

func TestHandlePopupCreate_DeniesWhenRelatedCreateIsUnsupported(t *testing.T) {
	ctx := context.Background()
	parentPaneID := entity.PaneID("parent-pane")
	parentWV := mocks.NewMockWebView(t)
	parentWV.EXPECT().ID().Return(port.WebViewID(101)).Once()

	factory := mocks.NewMockWebViewFactory(t)
	factory.EXPECT().CreateRelated(mock.Anything, port.WebViewID(101)).Return(nil, domainerrors.ErrRelatedWebViewUnsupported).Once()

	insertCalls := 0
	c := &Coordinator{
		webViews: make(map[entity.PaneID]port.WebView),
		popups:   newPopupManager(),
	}
	c.SetPopupConfig(factory, nil, nil)
	c.SetOnInsertPopup(func(_ context.Context, input InsertPopupInput) error {
		insertCalls++
		return nil
	})

	created := c.handlePopupCreate(ctx, parentPaneID, parentWV, port.PopupRequest{
		TargetURI:     "https://example.com/edit",
		FrameName:     "_blank",
		IsUserGesture: true,
	})

	require.Nil(t, created)
	assert.Equal(t, 0, insertCalls)
}

func TestHandlePopupCreate_DeniesWhenRelatedCreateFailsUnexpectedly(t *testing.T) {
	ctx := context.Background()
	parentPaneID := entity.PaneID("parent-pane")
	parentWV := mocks.NewMockWebView(t)
	parentWV.EXPECT().ID().Return(port.WebViewID(101)).Once()

	factory := mocks.NewMockWebViewFactory(t)
	factory.EXPECT().CreateRelated(mock.Anything, port.WebViewID(101)).Return(nil, errors.New("boom")).Once()

	insertCalls := 0
	c := &Coordinator{
		webViews: make(map[entity.PaneID]port.WebView),
		popups:   newPopupManager(),
	}
	c.SetPopupConfig(factory, nil, nil)
	c.SetOnInsertPopup(func(_ context.Context, input InsertPopupInput) error {
		insertCalls++
		return nil
	})

	created := c.handlePopupCreate(ctx, parentPaneID, parentWV, port.PopupRequest{
		TargetURI:     "https://example.com/edit",
		FrameName:     "_blank",
		IsUserGesture: true,
	})

	require.Nil(t, created)
	assert.Equal(t, 0, insertCalls)
}

func TestHandleLinkMiddleClick_UsesRelatedWebView(t *testing.T) {
	ctx := context.Background()
	parentPaneID := entity.PaneID("parent-pane")
	parentWV := mocks.NewMockWebView(t)
	parentWV.EXPECT().ID().Return(port.WebViewID(101)).Twice()

	newWV := &popupNavigationWebViewStub{MockWebView: mocks.NewMockWebView(t)}
	newWV.EXPECT().ID().Return(port.WebViewID(301)).Maybe()
	newWV.EXPECT().Generation().Return(uint64(1)).Maybe()
	newWV.EXPECT().SetCallbacks(mock.Anything).Maybe()
	newWV.EXPECT().LoadURI(mock.Anything, "https://example.com/middle-click").Return(nil).Maybe()

	factory := mocks.NewMockWebViewFactory(t)
	factory.EXPECT().CreateRelated(mock.Anything, port.WebViewID(101)).Return(newWV, nil).Maybe()

	insertCalls := 0
	c := &Coordinator{
		webViews: map[entity.PaneID]port.WebView{parentPaneID: parentWV},
		popups:   newPopupManager(),
	}
	c.SetPopupConfig(factory, nil, nil)
	c.SetOnInsertPopup(func(_ context.Context, input InsertPopupInput) error {
		insertCalls++
		assert.Equal(t, parentPaneID, input.ParentPaneID)
		assert.Same(t, newWV, input.WebView)
		assert.Equal(t, entity.PopupBehaviorSplit, input.Behavior)
		return nil
	})

	handled := c.handleLinkMiddleClick(ctx, parentPaneID, "https://example.com/middle-click")
	assert.True(t, handled)
	assert.Equal(t, 1, insertCalls)
	decision, hasDecision := newWV.BrowsingContextHostDecision()
	require.True(t, hasDecision)
	assert.Equal(t, port.HostDecisionCreatePane, decision.Kind)
}

func TestHandlePopupCreate_OpensNativePopupForAuthIntent(t *testing.T) {
	ctx := context.Background()
	parentPaneID := entity.PaneID("parent-pane")
	parentWV := mocks.NewMockWebView(t)
	parentWV.EXPECT().ID().Return(port.WebViewID(101)).Once()
	parentWV.EXPECT().URI().Return("https://x.com/i/flow/login").Once()

	popupWV := &popupNavigationWebViewStub{MockWebView: mocks.NewMockWebView(t)}
	popupWV.EXPECT().ID().Return(port.WebViewID(401)).Maybe()

	factory := mocks.NewMockWebViewFactory(t)
	factory.EXPECT().CreateRelated(mock.Anything, port.WebViewID(101)).Return(popupWV, nil).Once()

	insertCalls := 0
	nativeCalls := 0
	c := &Coordinator{
		webViews: make(map[entity.PaneID]port.WebView),
		popups:   newPopupManager(),
	}
	c.SetPopupConfig(factory, &entity.BrowsingContextConfig{OpenInNewPane: true, OAuthAutoClose: true}, nil)
	c.SetOnInsertPopup(func(context.Context, InsertPopupInput) error {
		insertCalls++
		return nil
	})
	c.SetOnOpenNativePopup(func(_ context.Context, input NativePopupInput) error {
		nativeCalls++
		assert.Equal(t, parentPaneID, input.ParentPaneID)
		assert.Equal(t, popupWV, input.PopupWebView)
		assert.Equal(t, "https://accounts.google.com/o/oauth2/v2/auth", input.TargetURI)
		assert.True(t, input.ObserveOAuthAutoClose)
		return nil
	})

	created := c.handlePopupCreate(ctx, parentPaneID, parentWV, port.PopupRequest{
		TargetURI:     "https://accounts.google.com/o/oauth2/v2/auth",
		FrameName:     "oauth-popup",
		IsUserGesture: true,
	})

	require.Equal(t, popupWV, created)
	assert.Equal(t, 0, insertCalls)
	assert.Equal(t, 1, nativeCalls)
	assert.Equal(t, []string{"https://accounts.google.com/o/oauth2/v2/auth"}, popupWV.primed)
}

func TestHandlePopupCreate_NativePopupDoesNotForceOAuthObservationWhenDisabled(t *testing.T) {
	ctx := context.Background()
	parentPaneID := entity.PaneID("parent-pane")
	parentWV := mocks.NewMockWebView(t)
	parentWV.EXPECT().ID().Return(port.WebViewID(101)).Once()

	popupWV := &popupNavigationWebViewStub{MockWebView: mocks.NewMockWebView(t)}
	popupWV.EXPECT().ID().Return(port.WebViewID(402)).Maybe()

	factory := mocks.NewMockWebViewFactory(t)
	factory.EXPECT().CreateRelated(mock.Anything, port.WebViewID(101)).Return(popupWV, nil).Once()

	c := &Coordinator{
		webViews: make(map[entity.PaneID]port.WebView),
		popups:   newPopupManager(),
	}
	c.SetPopupConfig(factory, &entity.BrowsingContextConfig{OpenInNewPane: true, OAuthAutoClose: false}, nil)
	c.SetOnOpenNativePopup(func(_ context.Context, input NativePopupInput) error {
		assert.False(t, input.ObserveOAuthAutoClose)
		return nil
	})

	created := c.handlePopupCreate(ctx, parentPaneID, parentWV, port.PopupRequest{
		TargetURI:     "https://accounts.google.com/o/oauth2/v2/auth",
		FrameName:     "oauth-popup",
		IsUserGesture: true,
	})

	require.Equal(t, popupWV, created)
}

func TestHandlePopupCreate_DoesNotReuseNamedPopupWhenNoJavaScriptAccess(t *testing.T) {
	ctx := context.Background()
	parentPaneID := entity.PaneID("parent-pane")
	parentWV := mocks.NewMockWebView(t)
	parentWV.EXPECT().ID().Return(port.WebViewID(101)).Twice()

	firstWV := mocks.NewMockWebView(t)
	firstWV.EXPECT().ID().Return(port.WebViewID(501)).Maybe()
	firstWV.EXPECT().Generation().Return(uint64(1)).Maybe()
	firstWV.EXPECT().SetCallbacks(mock.Anything).Maybe()
	firstWV.EXPECT().IsLoading().Return(false).Maybe()
	firstWV.EXPECT().URI().Return("").Maybe()
	firstWV.EXPECT().LoadURI(mock.Anything, "https://example.com/first").Return(nil).Maybe()

	secondWV := mocks.NewMockWebView(t)
	secondWV.EXPECT().ID().Return(port.WebViewID(502)).Maybe()
	secondWV.EXPECT().Generation().Return(uint64(1)).Maybe()
	secondWV.EXPECT().SetCallbacks(mock.Anything).Maybe()
	secondWV.EXPECT().IsLoading().Return(false).Maybe()
	secondWV.EXPECT().URI().Return("").Maybe()
	secondWV.EXPECT().LoadURI(mock.Anything, "https://example.com/second").Return(nil).Maybe()

	factory := mocks.NewMockWebViewFactory(t)
	factory.EXPECT().CreateRelated(mock.Anything, port.WebViewID(101)).Return(firstWV, nil).Once()
	factory.EXPECT().CreateRelated(mock.Anything, port.WebViewID(101)).Return(secondWV, nil).Once()

	insertCalls := 0
	c := &Coordinator{
		webViews: make(map[entity.PaneID]port.WebView),
		popups:   newPopupManager(),
	}
	c.SetPopupConfig(factory, nil, nil)
	c.SetPopupWindowIDResolver(func(entity.PaneID) (string, bool) { return "window-1", true })
	c.SetOnInsertPopup(func(context.Context, InsertPopupInput) error {
		insertCalls++
		return nil
	})

	first := c.handlePopupCreate(ctx, parentPaneID, parentWV, port.PopupRequest{
		TargetURI:          "https://example.com/first",
		FrameName:          "shared-pane",
		IsUserGesture:      true,
		NoJavaScriptAccess: true,
	})
	second := c.handlePopupCreate(ctx, parentPaneID, parentWV, port.PopupRequest{
		TargetURI:          "https://example.com/second",
		FrameName:          "shared-pane",
		IsUserGesture:      true,
		NoJavaScriptAccess: true,
	})

	require.Same(t, firstWV, first)
	require.Same(t, secondWV, second)
	assert.Equal(t, 2, insertCalls)
}

func TestHandlePopupCreate_ReusesNamedPopup(t *testing.T) {
	ctx := context.Background()
	parentPaneID := entity.PaneID("parent-pane")
	parentWV := mocks.NewMockWebView(t)
	parentWV.EXPECT().ID().Return(port.WebViewID(101)).Twice()

	popupWV := mocks.NewMockWebView(t)
	popupWV.EXPECT().ID().Return(port.WebViewID(202)).Maybe()
	popupWV.EXPECT().Generation().Return(uint64(1)).Maybe()
	popupWV.EXPECT().SetCallbacks(mock.Anything).Maybe()
	popupWV.EXPECT().IsLoading().Return(false).Maybe()
	currentURI := ""
	popupWV.EXPECT().URI().RunAndReturn(func() string { return currentURI }).Maybe()
	popupWV.EXPECT().IsDestroyed().Return(false).Maybe()
	popupWV.EXPECT().LoadURI(mock.Anything, "https://example.com/first").RunAndReturn(func(context.Context, string) error {
		currentURI = "https://example.com/first"
		return nil
	}).Once()
	popupWV.EXPECT().LoadURI(mock.Anything, "https://example.com/second").RunAndReturn(func(context.Context, string) error {
		currentURI = "https://example.com/second"
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
	c.SetPopupWindowIDResolver(func(entity.PaneID) (string, bool) { return "window-1", true })
	c.SetOnInsertPopup(func(context.Context, InsertPopupInput) error {
		insertCalls++
		return nil
	})

	first := c.handlePopupCreate(ctx, parentPaneID, parentWV, port.PopupRequest{
		TargetURI:     "https://example.com/first",
		FrameName:     "shared-pane",
		IsUserGesture: true,
	})
	require.Same(t, popupWV, first)

	second := c.handlePopupCreate(ctx, parentPaneID, parentWV, port.PopupRequest{
		TargetURI:     "https://example.com/second",
		FrameName:     "shared-pane",
		IsUserGesture: true,
	})
	require.Same(t, popupWV, second)

	assert.Equal(t, 1, insertCalls)
	assert.Equal(t, "https://example.com/second", currentURI)
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
	cfg := &entity.BrowsingContextConfig{Behavior: entity.PopupBehaviorSplit}
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
	primed                     []string
	browsingContextDecision    port.HostDecision
	hasBrowsingContextDecision bool
}

func (s *popupNavigationWebViewStub) PrimePopupNavigation(uri string) {
	s.primed = append(s.primed, uri)
}

func (s *popupNavigationWebViewStub) SetBrowsingContextHostDecision(decision port.HostDecision) {
	s.browsingContextDecision = decision
	s.hasBrowsingContextDecision = true
}

func (s *popupNavigationWebViewStub) BrowsingContextHostDecision() (port.HostDecision, bool) {
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
