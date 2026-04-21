package cef

import (
	"context"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/bnema/dumber/internal/application/port"
	portmocks "github.com/bnema/dumber/internal/application/port/mocks"
)

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
				require.Equal(t, port.WebViewID(17), req.ParentViewID)
				return popupWV
			},
		},
	}

	parentWV.handleSyntheticPopupNavigate("popup-1", "https://example.com/callback")
	parentWV.handleSyntheticPopupOpen("", "_blank", "popup-1", true)
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
				return popupWV
			},
		},
	}

	parentWV.handleSyntheticPopupOpen("https://example.com/start", "auth-popup", "popup-2", true)
	parentWV.handleSyntheticPopupNavigate("popup-2", "https://example.com/finish")
}
