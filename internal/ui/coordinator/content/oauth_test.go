package content

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/bnema/dumber/internal/application/port"
	portmocks "github.com/bnema/dumber/internal/application/port/mocks"
	"github.com/bnema/dumber/internal/domain/entity"
)

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

func TestHandlePopupOAuthClose_SuccessSchedulesParentResume(t *testing.T) {
	parentPaneID := entity.PaneID("parent-pane")
	popupID := port.WebViewID(101)

	c := &Coordinator{
		webViews:     make(map[entity.PaneID]port.WebView),
		popupOAuth:   make(map[port.WebViewID]*popupOAuthState),
		popupRefresh: make(map[entity.PaneID]*time.Timer),
	}

	c.trackOAuthPopup(popupID, parentPaneID)
	c.capturePopupOAuthState(popupID, "https://www.notion.so/googlepopupcallback?code=123")
	c.handlePopupOAuthClose(context.Background(), popupID)

	c.popupMu.RLock()
	_, exists := c.popupOAuth[popupID]
	refreshTimer := c.popupRefresh[parentPaneID]
	c.popupMu.RUnlock()

	assert.False(t, exists, "oauth state should be removed after close handling")
	assert.NotNil(t, refreshTimer, "oauth callback should schedule parent resume")

	waitFor(t, time.Second, func() bool {
		c.popupMu.RLock()
		defer c.popupMu.RUnlock()
		return c.popupRefresh[parentPaneID] == nil
	})
}

func TestResumeParentPaneAfterOAuth_SameDomainLoadsCallbackURI(t *testing.T) {
	parentPaneID := entity.PaneID("parent-pane")
	popupID := port.WebViewID(102)
	parentWV := portmocks.NewMockWebView(t)
	parentWV.EXPECT().IsDestroyed().Return(false).Once()
	parentWV.EXPECT().URI().Return("https://www.notion.so/login").Once()
	parentWV.EXPECT().LoadURI(mock.Anything, "https://www.notion.so/googlepopupcallback?code=123").Return(nil).Once()

	c := &Coordinator{
		webViews: map[entity.PaneID]port.WebView{
			parentPaneID: parentWV,
		},
	}

	c.resumeParentPaneAfterOAuth(context.Background(), parentPaneID, popupID, "https://www.notion.so/googlepopupcallback?code=123")
}

func TestResumeParentPaneAfterOAuth_DifferentDomainFallsBackToReload(t *testing.T) {
	parentPaneID := entity.PaneID("parent-pane")
	popupID := port.WebViewID(103)
	parentWV := portmocks.NewMockWebView(t)
	parentWV.EXPECT().IsDestroyed().Return(false).Once()
	parentWV.EXPECT().URI().Return("https://www.notion.so/login").Once()
	parentWV.EXPECT().Reload(mock.Anything).Return(nil).Once()

	c := &Coordinator{
		webViews: map[entity.PaneID]port.WebView{
			parentPaneID: parentWV,
		},
	}

	c.resumeParentPaneAfterOAuth(context.Background(), parentPaneID, popupID, "https://accounts.example.com/callback?code=123")
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
