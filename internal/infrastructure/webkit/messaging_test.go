package webkit

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/stretchr/testify/require"
)

func TestHandleAllowlistedEditableFocusMessage_AllowsUntrustedPage(t *testing.T) {
	router := NewMessageRouter(context.Background())
	wv := &WebView{uri: "https://example.com", editableFocusBridgeToken: "secret-token"}

	var states []bool
	wv.SetCallbacks(&port.WebViewCallbacks{
		OnEditableFocusChanged: func(editable bool) {
			states = append(states, editable)
		},
	})

	handled := router.handleAllowlistedBridgeMessage(wv, Message{
		Type:    "editable_focus_changed",
		Payload: json.RawMessage(`{"editable":true,"token":"secret-token"}`),
	})

	require.True(t, handled)
	require.Equal(t, []bool{true}, states)
}

func TestHandleAllowlistedEditableFocusMessage_IsNoopAfterPoolReuse(t *testing.T) {
	router := NewMessageRouter(context.Background())
	wv := &WebView{uri: "https://example.com", editableFocusBridgeToken: "secret-token"}
	called := false
	wv.SetCallbacks(&port.WebViewCallbacks{
		OnEditableFocusChanged: func(bool) {
			called = true
		},
	})
	wv.ResetForPoolReuse()

	handled := router.handleAllowlistedBridgeMessage(wv, Message{
		Type:    "editable_focus_changed",
		Payload: json.RawMessage(`{"editable":true,"token":"secret-token"}`),
	})

	require.True(t, handled)
	require.False(t, called)
}

func TestResetForPoolReuse_RotatesEditableFocusBridgeToken(t *testing.T) {
	wv := &WebView{editableFocusBridgeToken: "secret-token"}

	wv.ResetForPoolReuse()

	require.NotEmpty(t, wv.editableFocusBridgeToken)
	require.NotEqual(t, "secret-token", wv.editableFocusBridgeToken)
}

func TestIsTrustedBridgeURI(t *testing.T) {
	t.Parallel()

	trusted := []string{
		"dumb://history",
		"dumb://favorites",
		"dumb://config",
		"dumb://error",
		"dumb://crash",
		"dumb://history/path?cursor=1",
	}
	for _, raw := range trusted {
		t.Run(raw, func(t *testing.T) {
			t.Parallel()
			require.True(t, isTrustedBridgeURI(raw))
		})
	}

	untrusted := []string{
		"",
		"https://example.com",
		"http://localhost",
		"file:///tmp/history.html",
		"dumb://evil/history",
		"dumb://example.com",
		"javascript:alert(1)",
	}
	for _, raw := range untrusted {
		t.Run(raw, func(t *testing.T) {
			t.Parallel()
			require.False(t, isTrustedBridgeURI(raw))
		})
	}
}
