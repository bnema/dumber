package cef

import (
	"context"
	"encoding/base64"
	"net/http"
	"testing"

	purecef "github.com/bnema/purego-cef/cef"
	cefmocks "github.com/bnema/purego-cef/cef/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/bnema/dumber/internal/application/dto"
	"github.com/bnema/dumber/internal/application/port"
)

func expectVimPageScript(t *testing.T, fragments ...string) *WebView {
	t.Helper()
	browser := cefmocks.NewMockBrowser(t)
	frame := cefmocks.NewMockFrame(t)
	browser.EXPECT().GetMainFrame().Return(frame).Once()
	frame.EXPECT().ExecuteJavaScript(mock.MatchedBy(mockStringContaining(t, fragments...)), "", int32(0)).Once()
	return &WebView{browser: browser, bridgeNonce: "shared-bridge-nonce"}
}

func TestWebViewStartVimPageInteractionArmsFreshTokenWithoutBridgeNonce(t *testing.T) {
	wv := expectVimPageScript(t,
		"if (window.__dumberVimPage) return;",
		`dumb:///api/vim-page`,
		`{"kind":"yank","object":"code","color":"#123456"});`,
	)

	err := wv.StartVimPageInteraction(context.Background(), dto.VimPageInteractionRequest{
		Kind: dto.VimPageYankObject, Object: dto.VimPageTextObjectCode, HighlightColor: "#123456",
	})
	require.NoError(t, err)
	token := wv.currentVimPageToken()
	require.NotEmpty(t, token)
	assert.NotEqual(t, "shared-bridge-nonce", token)
	assert.NotContains(t, vimPageScript("start", token, "{}"), "shared-bridge-nonce")
	assert.Contains(t, vimPageScript("start", token, "{}"), `window.__dumberVimPage.start("`+token+`", {});`)
}

func TestWebViewSendVimPageKeyEncodesKeyAsJSString(t *testing.T) {
	wv := expectVimPageScript(t, `window.__dumberVimPage.key("tok", "\"\u0007");`)
	wv.vimPageToken = "tok"

	require.NoError(t, wv.SendVimPageKey(context.Background(), "\"\a"))
	require.NoError(t, wv.SendVimPageKey(context.Background(), ""))
}

func TestWebViewSendVimPageKeyWithoutInteractionIsNoop(t *testing.T) {
	wv := &WebView{browser: cefmocks.NewMockBrowser(t)}

	require.NoError(t, wv.SendVimPageKey(context.Background(), "j"))
}

func TestWebViewCancelVimPageInteractionDisarmsToken(t *testing.T) {
	wv := expectVimPageScript(t, `window.__dumberVimPage.cancel("tok");`)
	wv.vimPageToken = "tok"

	require.NoError(t, wv.CancelVimPageInteraction(context.Background()))
	assert.Empty(t, wv.currentVimPageToken())
	require.NoError(t, wv.CancelVimPageInteraction(context.Background()))
}

func TestVimPageRequestJSONRejectsUnknownRequests(t *testing.T) {
	_, err := vimPageRequestJSON(dto.VimPageInteractionRequest{})
	require.Error(t, err)
	_, err = vimPageRequestJSON(dto.VimPageInteractionRequest{Kind: dto.VimPageYankObject})
	require.Error(t, err)

	got, err := vimPageRequestJSON(dto.VimPageInteractionRequest{Kind: dto.VimPageHintFollowNew})
	require.NoError(t, err)
	assert.JSONEq(t, `{"kind":"hint-follow-new"}`, got)
}

func TestDecodeVimPageBridgePayload(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr bool
	}{
		{"copy", `{"type":"copy","token":"t","text":"hello"}`, false},
		{"copy empty", `{"type":"copy","token":"t"}`, true},
		{"missing token", `{"type":"end"}`, true},
		{"open", `{"type":"open-new","token":"t","url":"https://example.com/a"}`, false},
		{"open empty", `{"type":"open-new","token":"t"}`, true},
		{"end", `{"type":"end","token":"t"}`, false},
		{"unknown", `{"type":"eval","token":"t"}`, true},
		{"malformed", `{`, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := decodeVimPageBridgePayload([]byte(tt.body))
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestIsNavigableVimHintURL(t *testing.T) {
	assert.True(t, isNavigableVimHintURL("https://example.com/a", "https://site.test/"))
	assert.False(t, isNavigableVimHintURL("https:///nohost", "https://site.test/"))
	assert.False(t, isNavigableVimHintURL("javascript:alert(1)", "https://site.test/"))
	assert.False(t, isNavigableVimHintURL("dumb://config", "https://site.test/"))
	assert.False(t, isNavigableVimHintURL("file:///etc/passwd", "https://site.test/"))
	assert.True(t, isNavigableVimHintURL("file:///tmp/b.html", "file:///tmp/a.html"))
}

func newVimResultWebView(opened *string, ended *int) *WebView {
	return &WebView{ctx: context.Background(), uri: "https://site.test/", callbacks: &port.WebViewCallbacks{
		OnLinkMiddleClick:         func(uri string) bool { *opened = uri; return true },
		OnVimPageInteractionEnded: func() { *ended++ },
	}}
}

func TestWebViewHandleVimPageResultAcceptsArmedTokenOnce(t *testing.T) {
	var opened string
	ended := 0
	wv := newVimResultWebView(&opened, &ended)
	wv.vimPageToken, wv.vimPageKind = "tok", dto.VimPageHintFollowNew

	wv.handleVimPageResult(vimPageBridgePayload{Type: vimPageMessageOpenNew, Token: "tok", URL: "https://example.com"})
	wv.handleVimPageResult(vimPageBridgePayload{Type: vimPageMessageOpenNew, Token: "tok", URL: "https://evil.test"})

	assert.Equal(t, "https://example.com", opened)
	assert.Equal(t, 1, ended)
	assert.Empty(t, wv.currentVimPageToken())
}

func TestWebViewHandleVimPageResultRejectsStaleOrForgedTokens(t *testing.T) {
	var opened string
	ended := 0
	wv := newVimResultWebView(&opened, &ended)
	wv.vimPageToken, wv.vimPageKind = "current", dto.VimPageHintFollowNew

	wv.handleVimPageResult(vimPageBridgePayload{Type: vimPageMessageEnd, Token: "previous"})
	wv.handleVimPageResult(vimPageBridgePayload{Type: vimPageMessageOpenNew, Token: "forged", URL: "https://example.com"})

	assert.Empty(t, opened)
	assert.Zero(t, ended)
	assert.Equal(t, "current", wv.currentVimPageToken())
}

func TestWebViewHandleVimPageResultRejectsDisallowedURL(t *testing.T) {
	var opened string
	ended := 0
	wv := newVimResultWebView(&opened, &ended)
	wv.vimPageToken, wv.vimPageKind = "tok", dto.VimPageHintFollowNew

	wv.handleVimPageResult(vimPageBridgePayload{Type: vimPageMessageOpenNew, Token: "tok", URL: "file:///etc/passwd"})

	assert.Empty(t, opened)
	assert.Equal(t, 1, ended, "the interaction still ends so key capture is released")
}

func TestWebViewEndVimPageInteractionOnNavigation(t *testing.T) {
	var opened string
	ended := 0
	wv := newVimResultWebView(&opened, &ended)
	wv.vimPageToken, wv.vimPageKind = "tok", dto.VimPageHintFollowNew

	wv.endVimPageInteractionOnNavigation()
	wv.endVimPageInteractionOnNavigation()

	assert.Equal(t, 1, ended)
	assert.Empty(t, wv.currentVimPageToken())
}

func TestSchemeHandler_APIVimPageForwardsValidPayloads(t *testing.T) {
	oldNewResourceHandler := cefNewResourceHandler
	cefNewResourceHandler = func(impl purecef.ResourceHandler) purecef.ResourceHandler { return impl }
	defer func() { cefNewResourceHandler = oldNewResourceHandler }()

	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantCalled bool
	}{
		{"valid", `{"type":"copy","token":"tok","text":"yanked"}`, http.StatusOK, true},
		{"missing token", `{"type":"copy","text":"yanked"}`, http.StatusBadRequest, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newTestDumbSchemeHandler(t)
			browser := cefmocks.NewMockBrowser(t)
			var received vimPageBridgePayload
			called := false
			h.onVimPage = func(_ purecef.Browser, payload vimPageBridgePayload) {
				called = true
				received = payload
			}

			request := cefmocks.NewMockRequest(t)
			encoded := base64.StdEncoding.EncodeToString([]byte(tt.body))
			request.EXPECT().GetHeaderByName(dumberBodyHeaderName).Return(encoded).Once()
			response := cefmocks.NewMockResponse(t)
			response.EXPECT().SetStatus(int32(tt.wantStatus)).Once()
			response.EXPECT().SetStatusText(http.StatusText(tt.wantStatus)).Once()
			response.EXPECT().SetMimeType("application/json").Once()
			expectAPIResponseHeaders(response)

			handler := h.handleAPI(browser, http.MethodPost, "/api/vim-page", request)
			require.NotNil(t, handler)
			var length int64
			handler.GetResponseHeaders(response, &length, 0)

			assert.Equal(t, tt.wantCalled, called)
			if tt.wantCalled {
				assert.Equal(t, vimPageBridgePayload{Type: "copy", Token: "tok", Text: "yanked"}, received)
			}
		})
	}
}

func TestWebViewHandleVimPageResultRejectsTypeNotMatchingInteraction(t *testing.T) {
	var opened string
	ended := 0
	wv := newVimResultWebView(&opened, &ended)
	wv.vimPageToken, wv.vimPageKind = "tok", dto.VimPageYankObject

	wv.handleVimPageResult(vimPageBridgePayload{Type: vimPageMessageOpenNew, Token: "tok", URL: "https://example.com"})

	assert.Empty(t, opened)
	assert.Empty(t, wv.currentVimPageToken(), "the token is still spent")
}

func TestWebViewVimPageEndedCallbackSkippedAfterRearm(t *testing.T) {
	var opened string
	ended := 0
	wv := newVimResultWebView(&opened, &ended)
	_, err := wv.armVimPageInteraction(dto.VimPageHintFollow)
	require.NoError(t, err)
	_, generation := wv.disarmVimPageInteraction()
	_, err = wv.armVimPageInteraction(dto.VimPageHintFollow)
	require.NoError(t, err)

	wv.notifyVimPageInteractionEnded(generation)

	assert.Zero(t, ended)
}
