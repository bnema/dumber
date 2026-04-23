package cef

import (
	"context"
	"strings"
	"testing"

	cefmocks "github.com/bnema/purego-cef/cef/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestResolvePopupOpenerNavigationTarget_ResolvesRelativeAgainstOpener(t *testing.T) {
	require.Equal(
		t,
		"https://example.com/oauth/callback",
		resolvePopupOpenerNavigationTarget("callback", "https://example.com/oauth/start"),
	)
}

func TestResolvePopupOpenerNavigationTarget_PreservesAbsoluteTarget(t *testing.T) {
	require.Equal(
		t,
		"https://accounts.example.com/finish",
		resolvePopupOpenerNavigationTarget("https://accounts.example.com/finish", "https://example.com/oauth/start"),
	)
}

func TestResolvePopupOpenerNavigationTarget_RejectsNonHTTPAbsoluteTarget(t *testing.T) {
	require.Empty(t, resolvePopupOpenerNavigationTarget("javascript:alert(1)", "https://example.com/oauth/start"))
	require.Empty(t, resolvePopupOpenerNavigationTarget("data:text/html,boom", "https://example.com/oauth/start"))
	require.Empty(t, resolvePopupOpenerNavigationTarget("file:///tmp/boom", "https://example.com/oauth/start"))
}

func TestOriginFromURL_PreservesNonDefaultPort(t *testing.T) {
	require.Equal(t, "https://example.com:8443", originFromURL("https://example.com:8443/callback"))
}

func TestOriginFromURL_NormalizesInternalConceptualURLs(t *testing.T) {
	require.Equal(t, actualInternalOrigin, originFromURL("dumb://home"))
}

func TestTargetOriginMatchesPopupOpener_NormalizesDefaultPorts(t *testing.T) {
	require.True(t, targetOriginMatchesPopupOpener("https://example.com:443", "https://example.com/callback"))
}

func TestHandlePopupOpenerPostMessage_UsesPopupCommittedURIForSourceMetadata(t *testing.T) {
	frame := cefmocks.NewMockFrame(t)
	frame.EXPECT().ExecuteJavaScript(mock.MatchedBy(func(script string) bool {
		return strings.Contains(script, "https://popup.example") &&
			strings.Contains(script, "https://popup.example/oauth/callback") &&
			!strings.Contains(script, "https://evil.example")
	}), "", int32(0)).Once()
	browser := cefmocks.NewMockBrowser(t)
	browser.EXPECT().GetMainFrame().Return(frame).Once()

	opener := &WebView{ctx: context.Background(), browser: browser, uri: "https://app.example/session"}
	popup := &WebView{
		ctx:                     context.Background(),
		uri:                     "https://popup.example/oauth/callback",
		popupOpenerBridgeParent: opener,
	}

	popup.handlePopupOpenerPostMessage(popupOpenerPostMessagePayload{
		Data:         `{"ok":true}`,
		DataKind:     "json",
		TargetOrigin: "https://app.example",
		SourceOrigin: "https://evil.example",
		SourceHref:   "https://evil.example/fake",
	})
}
