package cef

import (
	"context"
	"encoding/base64"
	"net/http"
	"regexp"
	"strings"
	"testing"

	purecef "github.com/bnema/purego-cef/cef"
	cefmocks "github.com/bnema/purego-cef/cef/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/bnema/dumber/internal/application/dto"
	"github.com/bnema/dumber/internal/application/port"
)

// expectVimPageScript expects one ExecuteJavaScript call and captures the
// script that was actually sent to the page.
func expectVimPageScript(t *testing.T) (*WebView, *string) {
	t.Helper()
	browser := cefmocks.NewMockBrowser(t)
	frame := cefmocks.NewMockFrame(t)
	var script string
	browser.EXPECT().GetMainFrame().Return(frame).Once()
	frame.EXPECT().ExecuteJavaScript(mock.Anything, "", int32(0)).Run(func(code, _ string, _ int32) { script = code }).Once()
	return &WebView{browser: browser, bridgeNonce: "shared-bridge-nonce"}, &script
}

func TestWebViewStartVimPageInteractionArmsFreshTokenWithoutBridgeNonce(t *testing.T) {
	wv, script := expectVimPageScript(t)

	err := wv.StartVimPageInteraction(context.Background(), dto.VimPageInteractionRequest{
		Kind: dto.VimPageYankObject, Object: dto.VimPageTextObjectCode, HighlightColor: "#123456",
	})
	require.NoError(t, err)
	token := wv.currentVimPageToken()
	require.NotEmpty(t, token)
	assert.NotEqual(t, "shared-bridge-nonce", token)
	assert.Contains(t, *script, "if (window.__dumberVimPage) return;", "start ships the runtime")
	assert.Contains(t, *script, `dumb:///api/vim-page`)
	assert.NotContains(t, *script, "shared-bridge-nonce")
	assert.Contains(t, *script, `window.__dumberVimPage.start("`+token+`", {"kind":"yank","object":"code","color":"#123456"});`)
}

// Keys and cancels run on the already installed runtime: shipping the whole
// runtime for every key press would cost tens of kilobytes per keystroke.
func TestWebViewSendVimPageKeyDoesNotShipRuntime(t *testing.T) {
	wv, script := expectVimPageScript(t)
	wv.vimPageToken = "tok"

	require.NoError(t, wv.SendVimPageKey(context.Background(), "\"\a"))
	require.NoError(t, wv.SendVimPageKey(context.Background(), ""))

	assert.True(t, strings.HasPrefix(*script, `window.__dumberVimPage ? window.__dumberVimPage.key("tok", "\"\u0007") : `), *script)
	assert.NotContains(t, *script, "if (window.__dumberVimPage) return;")
	assert.Less(t, len(*script), 1024)
}

// A document replaced between start and key leaves no runtime; the script must
// then report the end of that token so the UI releases its key capture.
func TestVimPageCallScriptReportsEndWhenRuntimeMissing(t *testing.T) {
	script := vimPageCallScript("cancel", "tok")

	assert.Contains(t, script, `window.__dumberVimPage ? window.__dumberVimPage.cancel("tok") : `)
	assert.Contains(t, script, `dumb:///api/vim-page`)
	match := regexp.MustCompile(`"X-Dumber-Body": "([^"]+)"`).FindStringSubmatch(script)
	require.Len(t, match, 2)
	body, err := base64.StdEncoding.DecodeString(match[1])
	require.NoError(t, err)
	assert.JSONEq(t, `{"token":"tok","type":"end"}`, string(body))
}

func TestWebViewSendVimPageKeyWithoutInteractionIsNoop(t *testing.T) {
	wv := &WebView{browser: cefmocks.NewMockBrowser(t)}

	require.NoError(t, wv.SendVimPageKey(context.Background(), "j"))
}

func TestWebViewCancelVimPageInteractionDisarmsToken(t *testing.T) {
	wv, script := expectVimPageScript(t)
	wv.vimPageToken = "tok"

	require.NoError(t, wv.CancelVimPageInteraction(context.Background()))
	assert.True(t, strings.HasPrefix(*script, `window.__dumberVimPage ? window.__dumberVimPage.cancel("tok") : `), *script)
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

// The accent reaches the page as a CSS color; only hex colors are sent.
func TestVimPageRequestJSONValidatesAccentColor(t *testing.T) {
	for color, want := range map[string]string{
		"#fbbf24":                  `{"kind":"visual","color":"#fbbf24"}`,
		"#FFF":                     `{"kind":"visual","color":"#FFF"}`,
		"#aabbccdd":                `{"kind":"visual","color":"#aabbccdd"}`,
		"":                         `{"kind":"visual"}`,
		"red":                      `{"kind":"visual"}`,
		"#12":                      `{"kind":"visual"}`,
		"#123456789":               `{"kind":"visual"}`,
		"#abc;} body{display:none": `{"kind":"visual"}`,
		"url(javascript:alert(1))": `{"kind":"visual"}`,
		"#abc\"; alert(1); //":     `{"kind":"visual"}`,
	} {
		got, err := vimPageRequestJSON(dto.VimPageInteractionRequest{Kind: dto.VimPageVisual, HighlightColor: color})
		require.NoError(t, err)
		assert.JSONEq(t, want, got, "color %q", color)
	}
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
		{"mode caret", `{"type":"mode","token":"t","mode":"caret"}`, false},
		{"mode visual line", `{"type":"mode","token":"t","mode":"visual-line"}`, false},
		{"mode unknown", `{"type":"mode","token":"t","mode":"insert"}`, true},
		{"mode missing", `{"type":"mode","token":"t"}`, true},
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

// Mode messages and copies of the caret/visual flow are not terminal: the
// token stays armed so the user can keep moving and chain copies, and the UI
// is told about each sub-mode without being asked to release key capture.
func TestWebViewHandleVimPageResultVisualFlowStaysArmedAcrossModeAndCopy(t *testing.T) {
	var opened string
	ended := 0
	var modes []dto.VimPageMode
	wv := newVimResultWebView(&opened, &ended)
	wv.callbacks.OnVimPageModeChanged = func(mode dto.VimPageMode) { modes = append(modes, mode) }
	wv.vimPageToken, wv.vimPageKind = "tok", dto.VimPageVisual
	wv.vimPageCopyAllowance = 1

	wv.handleVimPageResult(vimPageBridgePayload{Type: vimPageMessageMode, Token: "tok", Mode: "caret"})
	wv.handleVimPageResult(vimPageBridgePayload{Type: vimPageMessageMode, Token: "tok", Mode: "visual-line"})
	wv.handleVimPageResult(vimPageBridgePayload{Type: vimPageMessageCopy, Token: "tok", Text: "copied"})

	assert.Equal(t, []dto.VimPageMode{dto.VimPageModeCaret, dto.VimPageModeVisualLine}, modes)
	assert.Zero(t, ended, "mode and copy must not end the interaction")
	assert.Equal(t, "tok", wv.currentVimPageToken())

	wv.handleVimPageResult(vimPageBridgePayload{Type: vimPageMessageEnd, Token: "tok"})
	assert.Equal(t, 1, ended)
	assert.Empty(t, wv.currentVimPageToken())
}

func TestWebViewHandleVimPageResultCopyEndsHintYankAndYankObject(t *testing.T) {
	for _, kind := range []dto.VimPageInteractionKind{dto.VimPageHintYankURL, dto.VimPageYankObject} {
		var opened string
		ended := 0
		wv := newVimResultWebView(&opened, &ended)
		wv.vimPageToken, wv.vimPageKind = "tok", kind

		wv.handleVimPageResult(vimPageBridgePayload{Type: vimPageMessageCopy, Token: "tok", Text: "x"})

		assert.Empty(t, wv.currentVimPageToken(), "kind %d: copy stays terminal", kind)
		assert.Equal(t, map[bool]int{true: 1, false: 0}[kind.CapturesKeys()], ended, "kind %d", kind)
	}
}

func TestWebViewHandleVimPageResultModeRejectedForLinkHints(t *testing.T) {
	var opened string
	ended := 0
	modes := 0
	wv := newVimResultWebView(&opened, &ended)
	wv.callbacks.OnVimPageModeChanged = func(dto.VimPageMode) { modes++ }
	wv.vimPageToken, wv.vimPageKind = "tok", dto.VimPageHintFollow

	wv.handleVimPageResult(vimPageBridgePayload{Type: vimPageMessageMode, Token: "tok", Mode: "caret"})

	assert.Zero(t, modes, "hint interactions never report sub-modes")
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

// handleVimPageResult runs on the CEF IO thread. The clipboard write must be
// handed to GTK instead of running there.
func newVimClipboardWebView(t *testing.T, kind dto.VimPageInteractionKind) (*WebView, *recordingClipboardTextOrchestrator, *[]func()) {
	t.Helper()
	orchestrator := &recordingClipboardTextOrchestrator{}
	queued := &[]func(){}
	wv := &WebView{
		ctx: context.Background(), id: 7, uri: "https://site.test/",
		engine:      &Engine{clipboardTextOrchestrator: orchestrator},
		gtkDispatch: func(fn func()) { *queued = append(*queued, fn) },
		callbacks:   &port.WebViewCallbacks{},
	}
	wv.vimPageToken, wv.vimPageKind = "tok", kind
	return wv, orchestrator, queued
}

func TestWebViewHandleVimPageResultCopyRunsOnGTK(t *testing.T) {
	wv, orchestrator, queued := newVimClipboardWebView(t, dto.VimPageYankObject)

	wv.handleVimPageResult(vimPageBridgePayload{Type: vimPageMessageCopy, Token: "tok", Text: "yanked"})

	assert.Empty(t, orchestrator.explicit.Text, "the clipboard must not be written on the IO thread")
	require.Len(t, *queued, 1)
	(*queued)[0]()
	assert.Equal(t, dto.ExplicitClipboardInput{
		Text: "yanked", Action: "copy", SourceEngine: dto.ClipboardSourceCEF, ViewID: 7,
	}, orchestrator.explicit)
}

// A hostile page that read the token must not be able to write the clipboard
// again and again: visual copies are non-terminal, so each needs a y/Enter the
// user pressed and Go forwarded.
func TestWebViewHandleVimPageResultVisualCopyNeedsForwardedCopyKey(t *testing.T) {
	wv, orchestrator, queued := newVimClipboardWebView(t, dto.VimPageVisual)
	copyResult := vimPageBridgePayload{Type: vimPageMessageCopy, Token: "tok", Text: "stolen"}

	wv.handleVimPageResult(copyResult)
	assert.Empty(t, *queued, "no key was forwarded: the copy is refused")
	assert.Equal(t, "tok", wv.currentVimPageToken(), "a refused copy leaves the interaction running")

	wv.grantVimPageCopyAllowance("tok", "j")
	wv.handleVimPageResult(copyResult)
	assert.Empty(t, *queued, "motion keys grant no copy")

	wv.grantVimPageCopyAllowance("tok", "y")
	wv.handleVimPageResult(copyResult)
	wv.handleVimPageResult(copyResult)
	require.Len(t, *queued, 1, "one forwarded key allows exactly one copy")
	(*queued)[0]()
	assert.Equal(t, "stolen", orchestrator.explicit.Text)

	wv.grantVimPageCopyAllowance("tok", "<Return>")
	wv.grantVimPageCopyAllowance("other", "y")
	wv.handleVimPageResult(copyResult)
	assert.Len(t, *queued, 2)
}

func TestWebViewSendVimPageKeyGrantsCopyAllowanceOnlyForVisual(t *testing.T) {
	for kind, want := range map[dto.VimPageInteractionKind]int{dto.VimPageVisual: 2, dto.VimPageHintYankURL: 0} {
		browser := cefmocks.NewMockBrowser(t)
		frame := cefmocks.NewMockFrame(t)
		browser.EXPECT().GetMainFrame().Return(frame).Times(3)
		frame.EXPECT().ExecuteJavaScript(mock.Anything, "", int32(0)).Times(3)
		wv := &WebView{browser: browser, vimPageToken: "tok", vimPageKind: kind}

		require.NoError(t, wv.SendVimPageKey(context.Background(), "y"))
		require.NoError(t, wv.SendVimPageKey(context.Background(), "j"))
		require.NoError(t, wv.SendVimPageKey(context.Background(), "<Return>"))

		assert.Equal(t, want, wv.vimPageCopyAllowance, "kind %d", kind)
	}
}

func TestWebViewArmVimPageInteractionResetsCopyAllowance(t *testing.T) {
	wv := &WebView{vimPageToken: "old", vimPageKind: dto.VimPageVisual, vimPageCopyAllowance: 3}

	_, err := wv.armVimPageInteraction(dto.VimPageVisual)
	require.NoError(t, err)
	assert.Zero(t, wv.vimPageCopyAllowance)
	wv.vimPageCopyAllowance = 2
	wv.disarmVimPageInteraction()
	assert.Zero(t, wv.vimPageCopyAllowance)
}

func TestWebViewHandleVimPageResultIgnoredAfterDestroy(t *testing.T) {
	wv, orchestrator, queued := newVimClipboardWebView(t, dto.VimPageYankObject)
	ended := 0
	wv.callbacks.OnVimPageInteractionEnded = func() { ended++ }
	wv.destroyed.Store(true)

	wv.handleVimPageResult(vimPageBridgePayload{Type: vimPageMessageCopy, Token: "tok", Text: "late"})

	assert.Empty(t, *queued)
	assert.Empty(t, orchestrator.explicit.Text)
	assert.Zero(t, ended)
}

func TestWebViewDestroyClearsVimPageInteraction(t *testing.T) {
	wv := &WebView{ctx: context.Background(), vimPageToken: "tok", vimPageKind: dto.VimPageVisual, vimPageCopyAllowance: 1}

	wv.Destroy()

	assert.Empty(t, wv.currentVimPageToken())
	assert.Zero(t, wv.vimPageKind)
	assert.Zero(t, wv.vimPageCopyAllowance)
}

// A crashed renderer takes the page runtime with it, so it can never report
// the end; the render-process-terminated handler must release the capture.
func TestHandlerSetOnRenderProcessTerminatedEndsVimPageInteraction(t *testing.T) {
	var opened string
	ended := 0
	wv := newVimResultWebView(&opened, &ended)
	wv.vimPageToken, wv.vimPageKind = "tok", dto.VimPageVisual
	h := &handlerSet{wv: wv}

	h.OnRenderProcessTerminated(nil, purecef.TerminationStatusTsProcessCrashed, 0, "")

	assert.Equal(t, 1, ended)
	assert.Empty(t, wv.currentVimPageToken())
}

func TestSchemeHandler_APIVimPageRejectsOversizedPayloadWith413(t *testing.T) {
	oldNewResourceHandler := cefNewResourceHandler
	cefNewResourceHandler = func(impl purecef.ResourceHandler) purecef.ResourceHandler { return impl }
	defer func() { cefNewResourceHandler = oldNewResourceHandler }()

	h := newTestDumbSchemeHandler(t)
	h.onVimPage = func(purecef.Browser, vimPageBridgePayload) { t.Fatal("oversized payload must not reach the handler") }
	request := cefmocks.NewMockRequest(t)
	encoded := base64.StdEncoding.EncodeToString([]byte(`{"type":"copy","token":"t","text":"` + strings.Repeat("a", maxClipboardBytes) + `"}`))
	request.EXPECT().GetHeaderByName(dumberBodyHeaderName).Return(encoded).Once()
	response := cefmocks.NewMockResponse(t)
	response.EXPECT().SetStatus(int32(http.StatusRequestEntityTooLarge)).Once()
	response.EXPECT().SetStatusText(http.StatusText(http.StatusRequestEntityTooLarge)).Once()
	response.EXPECT().SetMimeType("application/json").Once()
	expectAPIResponseHeaders(response)

	handler := h.handleAPI(cefmocks.NewMockBrowser(t), http.MethodPost, "/api/vim-page", request)
	require.NotNil(t, handler)
	var length int64
	handler.GetResponseHeaders(response, &length, 0)
}

func TestDecodeVimPageBridgePayloadOversizedIsSentinel(t *testing.T) {
	_, err := decodeVimPageBridgePayload(make([]byte, maxClipboardBytes+1))
	require.ErrorIs(t, err, errVimPagePayloadTooLarge)
	_, err = decodeVimPageBridgePayload([]byte(`{`))
	require.Error(t, err)
	require.NotErrorIs(t, err, errVimPagePayloadTooLarge)
}
