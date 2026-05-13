package content

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/bnema/dumber/internal/application/port"
	portmocks "github.com/bnema/dumber/internal/application/port/mocks"
	"github.com/bnema/dumber/internal/domain/entity"
)

type popupOAuthAutoCloseWebViewStub struct {
	*portmocks.MockWebView
	active                  bool
	navigationCallbacks     []func(string)
	closeCallbacks          []func()
	openerMessageCallbacks  []func()
	openerNavigateCallbacks []func(string)
	closeCount              int
}

func (s *popupOAuthAutoCloseWebViewStub) AddNavigationCallback(fn func(string)) {
	s.navigationCallbacks = append(s.navigationCallbacks, fn)
}

func (s *popupOAuthAutoCloseWebViewStub) AddCloseCallback(fn func()) {
	s.closeCallbacks = append(s.closeCallbacks, fn)
}

func (s *popupOAuthAutoCloseWebViewStub) Close() { s.closeCount++ }

func (*popupOAuthAutoCloseWebViewStub) EnablePopupOpenerBridge(port.WebView, bool) {}

func (s *popupOAuthAutoCloseWebViewStub) AddOpenerMessageCallback(fn func()) {
	s.openerMessageCallbacks = append(s.openerMessageCallbacks, fn)
}

func (s *popupOAuthAutoCloseWebViewStub) AddOpenerNavigationCallback(fn func(string)) {
	s.openerNavigateCallbacks = append(s.openerNavigateCallbacks, fn)
}

func (s *popupOAuthAutoCloseWebViewStub) HasActivePopupOpenerBridge() bool { return s.active }

func TestComposeOnClose_Order(t *testing.T) {
	var calls []string

	composed := composeOnClose(
		func() { calls = append(calls, "existing") },
		func() { calls = append(calls, "next") },
	)
	composed()

	assert.Equal(t, []string{"existing", "next"}, calls)
}

func TestComposeOnURIChanged_Order(t *testing.T) {
	var calls []string

	composed := composeOnURIChanged(
		func(_ string) { calls = append(calls, "existing") },
		func(_ string) { calls = append(calls, "next") },
	)
	composed("https://example.com")

	assert.Equal(t, []string{"existing", "next"}, calls)
}

func TestComposeOnLoadChanged_Order(t *testing.T) {
	var calls []string

	composed := composeOnLoadChanged(
		func(_ port.LoadEvent) { calls = append(calls, "existing") },
		func(_ port.LoadEvent) { calls = append(calls, "next") },
	)
	composed(port.LoadCommitted)

	assert.Equal(t, []string{"existing", "next"}, calls)
}

func TestCapturePopupOAuthMessage_MarksPopupSeenAndSuccessful(t *testing.T) {
	popupID := port.WebViewID(100)
	c := &Coordinator{
		popups: newPopupManager(),
	}
	c.popups.popupOAuth[popupID] = &popupOAuthState{
		ParentPaneID: entity.PaneID("parent-pane"),
	}

	c.capturePopupOAuthMessage(popupID)

	state := c.popups.popupOAuth[popupID]
	assert.True(t, state.Seen)
	assert.True(t, state.Success)
	assert.Equal(t, "postmessage://oauth-complete", state.CallbackURI)
}

func TestObserveNativePopupAuth_CapturesOAuthStateAndSchedulesParentResumeOnClose(t *testing.T) {
	popupID := port.WebViewID(999)
	wv := &popupOAuthAutoCloseWebViewStub{MockWebView: portmocks.NewMockWebView(t)}
	wv.EXPECT().ID().Return(popupID).Once()
	wv.EXPECT().IsDestroyed().Return(false).Maybe()

	c := &Coordinator{popups: newPopupManager()}
	c.ObserveNativePopupAuth(context.Background(), NativePopupInput{
		ParentPaneID:    entity.PaneID("parent-pane"),
		ParentURIAtOpen: "https://x.com/i/flow/login",
		PopupWebView:    wv,
		TargetURI:       "https://accounts.google.com/o/oauth2/v2/auth",
	})
	require.Len(t, wv.navigationCallbacks, 1)
	require.Len(t, wv.closeCallbacks, 1)

	wv.navigationCallbacks[0]("https://x.com/googlepopupcallback?code=123")
	state := c.popups.popupOAuth[popupID]
	require.NotNil(t, state)
	assert.True(t, state.Seen)
	assert.True(t, state.Success)
	assert.Equal(t, "https://x.com/googlepopupcallback?code=123", state.CallbackURI)

	wv.closeCallbacks[0]()
	c.popups.mu.RLock()
	refreshTimer := c.popups.popupRefresh[entity.PaneID("parent-pane")]
	c.popups.mu.RUnlock()
	assert.NotNil(t, refreshTimer)
}

func TestObserveNativePopupAuth_DoesNotTrackWhenWebViewLacksOAuthCallbacks(t *testing.T) {
	popupID := port.WebViewID(1001)
	wv := portmocks.NewMockWebView(t)
	wv.EXPECT().ID().Return(popupID).Once()

	c := &Coordinator{popups: newPopupManager()}
	c.ObserveNativePopupAuth(context.Background(), NativePopupInput{
		ParentPaneID:    entity.PaneID("parent-pane"),
		ParentURIAtOpen: "https://x.com/i/flow/login",
		PopupWebView:    wv,
		TargetURI:       "https://accounts.google.com/o/oauth2/v2/auth",
	})

	c.popups.mu.RLock()
	_, ok := c.popups.popupOAuth[popupID]
	c.popups.mu.RUnlock()
	assert.False(t, ok)
}

func TestObserveNativePopupAuth_SchedulesCloseOnlyOnce(t *testing.T) {
	popupID := port.WebViewID(1002)
	wv := &popupOAuthAutoCloseWebViewStub{MockWebView: portmocks.NewMockWebView(t)}
	wv.EXPECT().ID().Return(popupID).Once()
	wv.EXPECT().IsDestroyed().Return(false).Once()

	oldSchedule := scheduleNativePopupOAuthClose
	t.Cleanup(func() {
		scheduleNativePopupOAuthClose = oldSchedule
	})

	scheduledCalls := 0
	scheduleNativePopupOAuthClose = func(delay time.Duration, fn func()) {
		scheduledCalls++
		assert.Equal(t, nativePopupOAuthCloseDelay, delay)
		fn()
	}

	c := &Coordinator{popups: newPopupManager()}
	c.ObserveNativePopupAuth(context.Background(), NativePopupInput{
		ParentPaneID:    entity.PaneID("parent-pane"),
		ParentURIAtOpen: "https://x.com/i/flow/login",
		PopupWebView:    wv,
		TargetURI:       "https://accounts.google.com/o/oauth2/v2/auth",
	})
	require.Len(t, wv.navigationCallbacks, 1)

	wv.navigationCallbacks[0]("https://x.com/googlepopupcallback?code=123")
	wv.navigationCallbacks[0]("https://x.com/googlepopupcallback?code=123")

	assert.Equal(t, 1, scheduledCalls)
	assert.Equal(t, 1, wv.closeCount)
}

func TestSetupOAuthAutoClose_NavigationIgnoresNonTerminalURI(t *testing.T) {
	popupID := port.WebViewID(1000)
	wv := &popupOAuthAutoCloseWebViewStub{MockWebView: portmocks.NewMockWebView(t)}
	c := &Coordinator{popups: newPopupManager()}
	c.trackOAuthPopup(popupID, entity.PaneID("parent-pane"), "https://example.com/start")

	c.setupOAuthAutoClose(context.Background(), entity.PaneID("popup-pane"), popupID, wv)
	require.Len(t, wv.navigationCallbacks, 1)

	wv.navigationCallbacks[0]("https://example.com/intermediate")

	state, ok := c.popups.popupOAuth[popupID]
	require.True(t, ok)
	assert.False(t, state.Seen)
	assert.Empty(t, state.CallbackURI)

	for _, fn := range wv.closeCallbacks {
		fn()
	}
}

func TestHandlePopupOAuthClose_SuccessSchedulesParentResume(t *testing.T) {
	parentPaneID := entity.PaneID("parent-pane")
	popupID := port.WebViewID(101)

	c := &Coordinator{
		webViews: make(map[entity.PaneID]port.WebView),
		popups:   newPopupManager(),
	}

	c.trackOAuthPopup(popupID, parentPaneID, "https://www.notion.so/login")
	c.capturePopupOAuthState(popupID, "https://www.notion.so/googlepopupcallback?code=123")
	c.handlePopupOAuthClose(context.Background(), popupID)

	c.popups.mu.RLock()
	_, exists := c.popups.popupOAuth[popupID]
	refreshTimer := c.popups.popupRefresh[parentPaneID]
	c.popups.mu.RUnlock()

	assert.False(t, exists, "oauth state should be removed after close handling")
	assert.NotNil(t, refreshTimer, "oauth callback should schedule parent resume")

	waitFor(t, time.Second, func() bool {
		c.popups.mu.RLock()
		defer c.popups.mu.RUnlock()
		return c.popups.popupRefresh[parentPaneID] == nil
	})
}

func TestResumeParentPaneAfterOAuth_UnchangedParentDefersSameSiteFallbackReload(t *testing.T) {
	parentPaneID := entity.PaneID("parent-pane")
	popupID := port.WebViewID(102)
	parentWV := portmocks.NewMockWebView(t)
	parentWV.EXPECT().IsDestroyed().Return(false).Once()
	parentWV.EXPECT().URI().Return("https://www.notion.so/login").Once()

	c := &Coordinator{
		webViews: map[entity.PaneID]port.WebView{
			parentPaneID: parentWV,
		},
		popups: newPopupManager(),
	}

	c.resumeParentPaneAfterOAuth(context.Background(), parentPaneID, popupID, "https://www.notion.so/login", "https://www.notion.so/googlepopupcallback?code=123")

	c.popups.mu.Lock()
	refreshTimer := c.popups.popupRefresh[parentPaneID]
	if refreshTimer != nil {
		refreshTimer.Stop()
		delete(c.popups.popupRefresh, parentPaneID)
	}
	c.popups.mu.Unlock()

	assert.NotNil(t, refreshTimer, "same-site popup callback should get a grace retry before forcing reload")
}

func TestResumeParentPaneAfterOAuth_ChangedParentSkipsIntervention(t *testing.T) {
	parentPaneID := entity.PaneID("parent-pane")
	popupID := port.WebViewID(103)
	parentWV := portmocks.NewMockWebView(t)
	parentWV.EXPECT().IsDestroyed().Return(false).Once()
	parentWV.EXPECT().URI().Return("https://www.notion.so/oauth2callback?state=abc").Once()

	c := &Coordinator{
		webViews: map[entity.PaneID]port.WebView{
			parentPaneID: parentWV,
		},
		popups: newPopupManager(),
	}

	c.resumeParentPaneAfterOAuth(context.Background(), parentPaneID, popupID, "https://www.notion.so/login", "https://www.notion.so/googlepopupcallback?code=123")
	parentWV.AssertNotCalled(t, "Reload", mock.Anything)
}

func TestResumeParentPaneAfterOAuthAttempt_GraceExhaustedFallsBackToReload(t *testing.T) {
	parentPaneID := entity.PaneID("parent-pane")
	popupID := port.WebViewID(104)
	parentWV := portmocks.NewMockWebView(t)
	parentWV.EXPECT().IsDestroyed().Return(false).Once()
	parentWV.EXPECT().URI().Return("https://www.notion.so/login").Once()
	parentWV.EXPECT().Reload(mock.Anything).Return(nil).Once()

	c := &Coordinator{
		webViews: map[entity.PaneID]port.WebView{
			parentPaneID: parentWV,
		},
		popups: newPopupManager(),
	}

	c.resumeParentPaneAfterOAuthAttempt(context.Background(), parentPaneID, popupID, "https://www.notion.so/login", "https://www.notion.so/googlepopupcallback?code=123", 0)
}

func TestResumeParentPaneAfterOAuth_DifferentDomainFallsBackToReload(t *testing.T) {
	parentPaneID := entity.PaneID("parent-pane")
	popupID := port.WebViewID(105)
	parentWV := portmocks.NewMockWebView(t)
	parentWV.EXPECT().IsDestroyed().Return(false).Once()
	parentWV.EXPECT().URI().Return("https://www.notion.so/login").Once()
	parentWV.EXPECT().Reload(mock.Anything).Return(nil).Once()

	c := &Coordinator{
		webViews: map[entity.PaneID]port.WebView{
			parentPaneID: parentWV,
		},
		popups: newPopupManager(),
	}

	c.resumeParentPaneAfterOAuth(context.Background(), parentPaneID, popupID, "https://www.notion.so/login", "https://accounts.example.com/callback?code=123")
}

func TestIsOAuthURL_DoesNotTreatGenericStateParamAsOAuth(t *testing.T) {
	url := "https://github.com/apps/linear/installations/new?state=Ar4EJk1ao3eyEDgSbYOtG8Cr4"

	assert.False(t, IsOAuthURL(url))
}

func TestIsOAuthURL_DetectsOAuthAuthorizeRequest(t *testing.T) {
	url := "https://auth.openai.com/oauth/authorize?response_type=code&client_id=app_123&redirect_uri=http%3A%2F%2Flocalhost%3A1455%2Fauth%2Fcallback&scope=openid+profile+email+offline_access&state=abc"

	assert.True(t, IsOAuthURL(url))
}

func TestShouldForceCloseOnSafetyTimeout(t *testing.T) {
	assert.False(t, shouldForceCloseOnSafetyTimeout())
}

func waitFor(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("condition not met before timeout (%s)", timeout)
}
