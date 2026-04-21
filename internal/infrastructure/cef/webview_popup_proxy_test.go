package cef

import (
	"context"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/bnema/dumber/internal/application/port"
	portmocks "github.com/bnema/dumber/internal/application/port/mocks"
)

type closeablePopupWebViewStub struct {
	*portmocks.MockWebView
	closed bool
}

func (s *closeablePopupWebViewStub) AddCloseCallback(func())            {}
func (s *closeablePopupWebViewStub) AddNavigationCallback(func(string)) {}
func (s *closeablePopupWebViewStub) Close()                             { s.closed = true }

func TestHandleSyntheticPopupOpen_ReplaysQueuedNavigation(t *testing.T) {
	popupWV := portmocks.NewMockWebView(t)
	popupWV.EXPECT().IsDestroyed().Return(false).Once()
	popupWV.EXPECT().LoadURI(mock.Anything, "https://example.com/callback").Return(nil).Once()

	parentWV := &WebView{
		ctx: context.Background(),
		id:  port.WebViewID(17),
		callbacks: &port.WebViewCallbacks{
			OnCreate: func(req port.PopupRequest) port.WebView {
				require.Equal(t, "", req.TargetURI)
				require.Equal(t, "_blank", req.FrameName)
				require.True(t, req.IsUserGesture)
				require.True(t, req.NoJavaScriptAccess)
				require.Equal(t, port.WebViewID(17), req.ParentViewID)
				return popupWV
			},
		},
	}

	parentWV.handleSyntheticPopupNavigate("popup-1", "https://example.com/callback")
	parentWV.handleSyntheticPopupOpen("", "_blank", "popup-1", true, true)
}

func TestHandleSyntheticPopupNavigate_LoadsMappedPopup(t *testing.T) {
	popupWV := portmocks.NewMockWebView(t)
	popupWV.EXPECT().IsDestroyed().Return(false).Once()
	popupWV.EXPECT().LoadURI(mock.Anything, "https://example.com/finish").Return(nil).Once()

	parentWV := &WebView{
		ctx: context.Background(),
		id:  port.WebViewID(23),
		callbacks: &port.WebViewCallbacks{
			OnCreate: func(req port.PopupRequest) port.WebView {
				require.Equal(t, "https://example.com/start", req.TargetURI)
				require.Equal(t, "auth-popup", req.FrameName)
				require.False(t, req.NoJavaScriptAccess)
				return popupWV
			},
		},
	}

	parentWV.handleSyntheticPopupOpen("https://example.com/start", "auth-popup", "popup-2", true, false)
	parentWV.handleSyntheticPopupNavigate("popup-2", "https://example.com/finish")
}

func TestHandleSyntheticPopupOpen_CleansUpStateWhenPopupIsBlocked(t *testing.T) {
	parentWV := &WebView{
		ctx: context.Background(),
		id:  port.WebViewID(28),
		callbacks: &port.WebViewCallbacks{
			OnCreate: func(port.PopupRequest) port.WebView {
				return nil
			},
		},
	}

	parentWV.handleSyntheticPopupNavigate("popup-blocked", "https://example.com/blocked")
	parentWV.handleSyntheticPopupOpen("https://example.com/blocked", "_blank", "popup-blocked", true, false)

	require.Empty(t, parentWV.syntheticPopups)
}

func TestHandleSyntheticPopupClose_ClosesMappedPopup(t *testing.T) {
	popupWV := &closeablePopupWebViewStub{MockWebView: portmocks.NewMockWebView(t)}
	parentWV := &WebView{
		ctx: context.Background(),
		id:  port.WebViewID(29),
		syntheticPopups: map[string]*syntheticPopupState{
			"popup-3": {WebView: popupWV},
		},
	}

	parentWV.handleSyntheticPopupClose("popup-3")

	require.True(t, popupWV.closed)
	require.Empty(t, parentWV.syntheticPopups)
}
