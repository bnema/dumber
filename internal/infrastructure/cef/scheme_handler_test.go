package cef

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"testing"

	purecef "github.com/bnema/purego-cef/cef"
	cefmocks "github.com/bnema/purego-cef/cef/mocks"
	"github.com/stretchr/testify/require"
)

func expectAPIResponseHeaders(response *cefmocks.MockResponse) {
	response.EXPECT().SetHeaderByName("Access-Control-Allow-Origin", "*", int32(1)).Once()
	response.EXPECT().SetHeaderByName("Access-Control-Allow-Methods", "GET, POST, OPTIONS", int32(1)).Once()
	response.EXPECT().SetHeaderByName(
		"Access-Control-Allow-Headers",
		"Content-Type, X-Dumber-Body, X-Dumber-Bridge-Action, X-Dumber-Bridge-Nonce",
		int32(1),
	).Once()
	response.EXPECT().SetHeaderByName("Access-Control-Max-Age", "86400", int32(1)).Once()
	response.EXPECT().SetHeaderByName("Cache-Control", "no-store", int32(1)).Once()
}

func expectPrivateAPIResponseHeaders(response *cefmocks.MockResponse) {
	response.EXPECT().SetHeaderByName("Cache-Control", "no-store", int32(1)).Once()
}

type panicFaviconService struct{}

func (panicFaviconService) GetCached(context.Context, string) ([]byte, bool) {
	panic("unexpected favicon lookup")
}
func (panicFaviconService) Get(context.Context, string) ([]byte, error) {
	panic("unexpected favicon lookup")
}
func (panicFaviconService) DiskPathPNG(string) string           { panic("unexpected favicon lookup") }
func (panicFaviconService) HasPNGOnDisk(string) bool            { panic("unexpected favicon lookup") }
func (panicFaviconService) HasPNGSizedOnDisk(string, int) bool  { panic("unexpected favicon lookup") }
func (panicFaviconService) DiskPathPNGSized(string, int) string { panic("unexpected favicon lookup") }
func (panicFaviconService) EnsureSizedPNG(context.Context, string, int) error {
	panic("unexpected favicon lookup")
}
func (panicFaviconService) EnsureCacheDir() error { panic("unexpected favicon lookup") }
func (panicFaviconService) EnsureDiskCache(context.Context, string) {
	panic("unexpected favicon lookup")
}
func (panicFaviconService) Close() {}

// noopConfigPayload returns a builder that always returns an empty JSON object.
func noopConfigPayload() func() ([]byte, error) {
	return func() ([]byte, error) { return []byte(`{}`), nil }
}

// newTestDumbSchemeHandler creates a dumbSchemeHandler with noop config payloads.
// It calls t.Helper() and fails the test if construction errors.
func newTestDumbSchemeHandler(t *testing.T) *dumbSchemeHandler {
	t.Helper()
	h, err := newDumbSchemeHandler(context.Background(), nil, noopConfigPayload(), noopConfigPayload())
	require.NoError(t, err)
	return h
}

func TestResolveConfigPayload_UsesInjectedBuilder(t *testing.T) {
	data, err := resolveConfigPayload(func() ([]byte, error) {
		return []byte(`{"engine_type":"cef"}`), nil
	})
	require.NoError(t, err)
	require.JSONEq(t, `{"engine_type":"cef"}`, string(data))
}

func TestResolveConfigPayload_PropagatesBuilderError(t *testing.T) {
	want := errors.New("boom")
	_, err := resolveConfigPayload(func() ([]byte, error) {
		return nil, want
	})
	require.ErrorIs(t, err, want)
}

func TestResolveConfigPayload_NilBuilderFails(t *testing.T) {
	_, err := resolveConfigPayload(nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "config payload builder")
}

func TestNewDumbSchemeHandler_NilCurrentConfigPayloadFails(t *testing.T) {
	h, err := newDumbSchemeHandler(context.Background(), nil, nil, func() ([]byte, error) { return nil, nil })
	require.Error(t, err)
	require.Nil(t, h)
	require.Contains(t, err.Error(), "current config payload builder")
}

func TestNewDumbSchemeHandler_NilDefaultConfigPayloadFails(t *testing.T) {
	h, err := newDumbSchemeHandler(context.Background(), nil, func() ([]byte, error) { return nil, nil }, nil)
	require.Error(t, err)
	require.Nil(t, h)
	require.Contains(t, err.Error(), "default config payload builder")
}

func TestRejectForbiddenAPIOrigin_AllowsInternalOrigin(t *testing.T) {
	h := newTestDumbSchemeHandler(t)

	request := cefmocks.NewMockRequest(t)
	request.EXPECT().GetHeaderByName("Origin").Return(actualInternalOrigin).Once()

	require.Nil(t, h.rejectForbiddenAPIOrigin(request))
}

func TestRejectForbiddenAPIOrigin_RejectsExternalOrigin(t *testing.T) {
	oldNewResourceHandler := cefNewResourceHandler
	cefNewResourceHandler = func(impl purecef.ResourceHandler) purecef.ResourceHandler { return impl }
	defer func() { cefNewResourceHandler = oldNewResourceHandler }()

	h := newTestDumbSchemeHandler(t)

	request := cefmocks.NewMockRequest(t)
	request.EXPECT().GetHeaderByName("Origin").Return("https://evil.example").Once()

	response := cefmocks.NewMockResponse(t)
	response.EXPECT().SetStatus(int32(http.StatusForbidden)).Once()
	response.EXPECT().SetStatusText(http.StatusText(http.StatusForbidden)).Once()
	response.EXPECT().SetMimeType("application/json").Once()
	expectAPIResponseHeaders(response)

	handler := h.rejectForbiddenAPIOrigin(request)
	require.NotNil(t, handler)
	var responseLength int64
	handler.GetResponseHeaders(response, &responseLength, 0)
	require.Positive(t, responseLength)
}

func TestRejectUntrustedConfigRequesterRequiresTrustedOriginOrReferrer(t *testing.T) {
	oldNewResourceHandler := cefNewResourceHandler
	cefNewResourceHandler = func(impl purecef.ResourceHandler) purecef.ResourceHandler { return impl }
	defer func() { cefNewResourceHandler = oldNewResourceHandler }()

	h := newTestDumbSchemeHandler(t)

	trustedOrigin := cefmocks.NewMockRequest(t)
	trustedOrigin.EXPECT().GetHeaderByName("Origin").Return(actualInternalOrigin).Once()
	require.Nil(t, h.rejectUntrustedConfigRequester(trustedOrigin))

	trustedReferrer := cefmocks.NewMockRequest(t)
	trustedReferrer.EXPECT().GetHeaderByName("Origin").Return("").Once()
	trustedReferrer.EXPECT().GetReferrerURL().Return("dumb://history").Once()
	require.Nil(t, h.rejectUntrustedConfigRequester(trustedReferrer))

	emptyContext := cefmocks.NewMockRequest(t)
	emptyContext.EXPECT().GetHeaderByName("Origin").Return("").Once()
	emptyContext.EXPECT().GetReferrerURL().Return("").Once()
	emptyContext.EXPECT().GetHeaderByName("Referer").Return("").Once()
	denied := h.rejectUntrustedConfigRequester(emptyContext)
	require.NotNil(t, denied)

	response := cefmocks.NewMockResponse(t)
	response.EXPECT().SetStatus(int32(http.StatusForbidden)).Once()
	response.EXPECT().SetStatusText(http.StatusText(http.StatusForbidden)).Once()
	response.EXPECT().SetMimeType("application/json").Once()
	expectPrivateAPIResponseHeaders(response)
	var responseLength int64
	denied.GetResponseHeaders(response, &responseLength, 0)
	require.Positive(t, responseLength)

	untrustedOrigin := cefmocks.NewMockRequest(t)
	untrustedOrigin.EXPECT().GetHeaderByName("Origin").Return("https://evil.example").Once()
	require.NotNil(t, h.rejectUntrustedConfigRequester(untrustedOrigin))
}

func TestHandleMessageAPIRequiresTrustedOriginOrReferrer(t *testing.T) {
	oldNewResourceHandler := cefNewResourceHandler
	cefNewResourceHandler = func(impl purecef.ResourceHandler) purecef.ResourceHandler { return impl }
	defer func() { cefNewResourceHandler = oldNewResourceHandler }()

	h, err := newDumbSchemeHandler(context.Background(), NewMessageRouter(context.Background()), noopConfigPayload(), noopConfigPayload())
	require.NoError(t, err)

	trusted := cefmocks.NewMockRequest(t)
	trusted.EXPECT().GetHeaderByName("Origin").Return("").Once()
	trusted.EXPECT().GetReferrerURL().Return("dumb://history").Once()
	trusted.EXPECT().GetHeaderByName(dumberBodyHeaderName).Return(base64.StdEncoding.EncodeToString([]byte(`{"type":"missing"}`))).Once()
	trustedHandler := h.handleMessageAPI(trusted)
	require.NotNil(t, trustedHandler)

	untrusted := cefmocks.NewMockRequest(t)
	untrusted.EXPECT().GetHeaderByName("Origin").Return("").Once()
	untrusted.EXPECT().GetReferrerURL().Return("").Once()
	untrusted.EXPECT().GetHeaderByName("Referer").Return("").Once()
	denied := h.handleMessageAPI(untrusted)
	require.NotNil(t, denied)

	response := cefmocks.NewMockResponse(t)
	response.EXPECT().SetStatus(int32(http.StatusForbidden)).Once()
	response.EXPECT().SetStatusText(http.StatusText(http.StatusForbidden)).Once()
	response.EXPECT().SetMimeType("application/json").Once()
	expectPrivateAPIResponseHeaders(response)
	var responseLength int64
	denied.GetResponseHeaders(response, &responseLength, 0)
	require.Positive(t, responseLength)
}

func TestHandleConfigAPIUsesPrivateNoCORSHeaders(t *testing.T) {
	oldNewResourceHandler := cefNewResourceHandler
	cefNewResourceHandler = func(impl purecef.ResourceHandler) purecef.ResourceHandler { return impl }
	defer func() { cefNewResourceHandler = oldNewResourceHandler }()

	h, err := newDumbSchemeHandler(context.Background(), nil, func() ([]byte, error) { return []byte(`{"ok":true}`), nil }, noopConfigPayload())
	require.NoError(t, err)

	response := cefmocks.NewMockResponse(t)
	response.EXPECT().SetStatus(int32(http.StatusOK)).Once()
	response.EXPECT().SetStatusText(http.StatusText(http.StatusOK)).Once()
	response.EXPECT().SetMimeType("application/json").Once()
	expectPrivateAPIResponseHeaders(response)

	handler := h.handleConfigAPI(h.currentConfigPayload)
	var responseLength int64
	handler.GetResponseHeaders(response, &responseLength, 0)
	require.Positive(t, responseLength)
}

func TestHandleConfigAPIOptionsUsesPrivateNoCORSHeaders(t *testing.T) {
	oldNewResourceHandler := cefNewResourceHandler
	cefNewResourceHandler = func(impl purecef.ResourceHandler) purecef.ResourceHandler { return impl }
	defer func() { cefNewResourceHandler = oldNewResourceHandler }()

	h := newTestDumbSchemeHandler(t)

	request := cefmocks.NewMockRequest(t)
	request.EXPECT().GetHeaderByName("Origin").Return(actualInternalOrigin).Once()

	response := cefmocks.NewMockResponse(t)
	response.EXPECT().SetStatus(int32(http.StatusNoContent)).Once()
	response.EXPECT().SetStatusText(http.StatusText(http.StatusNoContent)).Once()
	response.EXPECT().SetMimeType("text/plain").Once()
	response.EXPECT().SetCharset("utf-8").Once()
	expectPrivateAPIResponseHeaders(response)

	handler := h.handleAPI(nil, http.MethodOptions, "/api/config", request)
	var responseLength int64
	handler.GetResponseHeaders(response, &responseLength, 0)
	require.Zero(t, responseLength)
}

func TestIsTrustedSystemviewURL(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{
		"https://dumber.invalid/history",
		"dumb://history",
		"dumb:history",
	} {
		require.True(t, isTrustedSystemviewURL(raw), raw)
	}
	for _, raw := range []string{
		"",
		"https://evil.example/history",
		"dumb://api/favicon",
		"dumb://evil/history",
	} {
		require.False(t, isTrustedSystemviewURL(raw), raw)
	}
}

func TestRejectUntrustedFaviconRequesterAllowsTrustedOriginWithoutReferrer(t *testing.T) {
	h := newTestDumbSchemeHandler(t)

	request := cefmocks.NewMockRequest(t)
	request.EXPECT().GetHeaderByName("Origin").Return(actualInternalOrigin).Once()

	require.Nil(t, h.rejectUntrustedFaviconRequester(request))
}

func TestRejectUntrustedFaviconRequesterRejectsUntrustedOriginEvenWithTrustedReferrer(t *testing.T) {
	oldNewResourceHandler := cefNewResourceHandler
	cefNewResourceHandler = func(impl purecef.ResourceHandler) purecef.ResourceHandler { return impl }
	defer func() { cefNewResourceHandler = oldNewResourceHandler }()

	h := newTestDumbSchemeHandler(t)

	request := cefmocks.NewMockRequest(t)
	request.EXPECT().GetHeaderByName("Origin").Return("https://evil.example").Once()

	require.NotNil(t, h.rejectUntrustedFaviconRequester(request))
}

func TestRejectUntrustedFaviconRequesterRequiresTrustedReferrerWhenOriginAbsent(t *testing.T) {
	oldNewResourceHandler := cefNewResourceHandler
	cefNewResourceHandler = func(impl purecef.ResourceHandler) purecef.ResourceHandler { return impl }
	defer func() { cefNewResourceHandler = oldNewResourceHandler }()

	h := newTestDumbSchemeHandler(t)

	trusted := cefmocks.NewMockRequest(t)
	trusted.EXPECT().GetHeaderByName("Origin").Return("").Once()
	trusted.EXPECT().GetReferrerURL().Return("dumb://history").Once()
	require.Nil(t, h.rejectUntrustedFaviconRequester(trusted))

	untrusted := cefmocks.NewMockRequest(t)
	untrusted.EXPECT().GetHeaderByName("Origin").Return("").Once()
	untrusted.EXPECT().GetReferrerURL().Return("").Once()
	untrusted.EXPECT().GetHeaderByName("Referer").Return("").Once()
	require.NotNil(t, h.rejectUntrustedFaviconRequester(untrusted))
}

func TestHandleFaviconAPIDefersFaviconDiskChecksUntilResourceOpen(t *testing.T) {
	oldNewResourceHandler := cefNewResourceHandler
	cefNewResourceHandler = func(impl purecef.ResourceHandler) purecef.ResourceHandler { return impl }
	defer func() { cefNewResourceHandler = oldNewResourceHandler }()

	h := newTestDumbSchemeHandler(t)
	h.setFaviconService(panicFaviconService{})

	request := cefmocks.NewMockRequest(t)
	request.EXPECT().GetHeaderByName("Origin").Return(actualInternalOrigin).Once()
	request.EXPECT().GetURL().Return(actualInternalOrigin + "/api/favicon?domain=example.com&size=32").Once()

	require.NotNil(t, h.handleFaviconAPI(request))
}

func TestReadBodyFromHeader_DecodesBase64Payload(t *testing.T) {
	request := cefmocks.NewMockRequest(t)
	encoded := base64.StdEncoding.EncodeToString([]byte(`{"text":"copied from js"}`))
	request.EXPECT().GetHeaderByName(dumberBodyHeaderName).Return(encoded).Once()

	body := readBodyFromHeader(request)

	require.JSONEq(t, `{"text":"copied from js"}`, string(body))
}

func TestDecodePopupOpenerPostMessagePayload_RejectsMissingTargetOrigin(t *testing.T) {
	_, err := decodePopupOpenerPostMessagePayload([]byte(`{"data":"{}","data_kind":"json","target_origin":"   "}`))
	require.Error(t, err)
	require.Contains(t, err.Error(), "target origin")
}

func TestDecodePopupOpenerPostMessagePayload_TrimsAndAcceptsTargetOrigin(t *testing.T) {
	payload, err := decodePopupOpenerPostMessagePayload([]byte(`{"data":"{}","data_kind":"json","target_origin":" https://example.com ","source_origin":" https://popup.example.com ","source_href":" https://popup.example.com/callback "}`))
	require.NoError(t, err)
	require.Equal(t, "https://example.com", payload.TargetOrigin)
	require.Equal(t, "https://popup.example.com", payload.SourceOrigin)
	require.Equal(t, "https://popup.example.com/callback", payload.SourceHref)
}

func TestSchemeHandler_APIClipboardSetPathWritesClipboardPayload(t *testing.T) {
	oldNewResourceHandler := cefNewResourceHandler
	cefNewResourceHandler = func(impl purecef.ResourceHandler) purecef.ResourceHandler { return impl }
	defer func() { cefNewResourceHandler = oldNewResourceHandler }()

	h := newTestDumbSchemeHandler(t)

	const bridgeNonce = "bridge-nonce"
	browser := cefmocks.NewMockBrowser(t)
	h.bridgeNonceValidator = func(got purecef.Browser, gotNonce string) bool {
		return got == browser && gotNonce == bridgeNonce
	}

	var copied string
	h.onClipboardSet = func(text string) { copied = text }

	request := cefmocks.NewMockRequest(t)
	encoded := base64.StdEncoding.EncodeToString([]byte(`{"text":"copied from js"}`))
	request.EXPECT().GetHeaderByName(dumberBridgeNonceHeaderName).Return(bridgeNonce).Once()
	request.EXPECT().GetHeaderByName(dumberBodyHeaderName).Return(encoded).Once()

	response := cefmocks.NewMockResponse(t)
	response.EXPECT().SetStatus(int32(http.StatusOK)).Once()
	response.EXPECT().SetStatusText(http.StatusText(http.StatusOK)).Once()
	response.EXPECT().SetMimeType("application/json").Once()
	expectAPIResponseHeaders(response)

	handler := h.handleAPI(browser, http.MethodPost, "/api/clipboard-set", request)
	require.NotNil(t, handler)

	var responseLength int64
	handler.GetResponseHeaders(response, &responseLength, 0)
	require.Positive(t, responseLength)
	require.Equal(t, "copied from js", copied)
}

func TestSchemeHandler_APIFocusSyncRejectsRequestsWithoutTrustedBridgeHeader(t *testing.T) {
	oldNewResourceHandler := cefNewResourceHandler
	cefNewResourceHandler = func(impl purecef.ResourceHandler) purecef.ResourceHandler { return impl }
	defer func() { cefNewResourceHandler = oldNewResourceHandler }()

	h := newTestDumbSchemeHandler(t)

	request := cefmocks.NewMockRequest(t)
	request.EXPECT().GetHeaderByName(dumberBridgeActionHeaderName).Return("").Once()

	response := cefmocks.NewMockResponse(t)
	response.EXPECT().SetStatus(int32(http.StatusForbidden)).Once()
	response.EXPECT().SetStatusText(http.StatusText(http.StatusForbidden)).Once()
	response.EXPECT().SetMimeType("application/json").Once()
	expectAPIResponseHeaders(response)

	handler := h.handleAPI(nil, http.MethodPost, "/api/focus-sync", request)
	require.NotNil(t, handler)

	var responseLength int64
	handler.GetResponseHeaders(response, &responseLength, 0)
	require.Positive(t, responseLength)
}

func TestSchemeHandler_APIFocusSyncInvokesEditableFocusCallback(t *testing.T) {
	oldNewResourceHandler := cefNewResourceHandler
	cefNewResourceHandler = func(impl purecef.ResourceHandler) purecef.ResourceHandler { return impl }
	defer func() { cefNewResourceHandler = oldNewResourceHandler }()

	h := newTestDumbSchemeHandler(t)

	const bridgeNonce = "bridge-nonce"
	browser := cefmocks.NewMockBrowser(t)
	h.bridgeNonceValidator = func(got purecef.Browser, gotNonce string) bool {
		return got == browser && gotNonce == bridgeNonce
	}

	var focused purecef.Browser
	h.onEditableFocus = func(got purecef.Browser) { focused = got }

	request := cefmocks.NewMockRequest(t)
	request.EXPECT().GetHeaderByName(dumberBridgeActionHeaderName).Return(dumberBridgeActionFocusSync).Once()
	request.EXPECT().GetHeaderByName(dumberBridgeNonceHeaderName).Return(bridgeNonce).Once()

	response := cefmocks.NewMockResponse(t)
	response.EXPECT().SetStatus(int32(http.StatusOK)).Once()
	response.EXPECT().SetStatusText(http.StatusText(http.StatusOK)).Once()
	response.EXPECT().SetMimeType("application/json").Once()
	expectAPIResponseHeaders(response)

	handler := h.handleAPI(browser, http.MethodPost, "/api/focus-sync", request)
	require.NotNil(t, handler)

	var responseLength int64
	handler.GetResponseHeaders(response, &responseLength, 0)
	require.Positive(t, responseLength)
	require.Same(t, browser, focused)
}

func TestSchemeHandler_Create_ConceptualAPIRequestBypassesRedirect(t *testing.T) {
	oldNewResourceHandler := cefNewResourceHandler
	cefNewResourceHandler = func(impl purecef.ResourceHandler) purecef.ResourceHandler { return impl }
	defer func() { cefNewResourceHandler = oldNewResourceHandler }()

	h := newTestDumbSchemeHandler(t)

	const bridgeNonce = "bridge-nonce"
	browser := cefmocks.NewMockBrowser(t)
	h.bridgeNonceValidator = func(got purecef.Browser, gotNonce string) bool {
		return got == browser && gotNonce == bridgeNonce
	}

	var copied string
	h.onClipboardSet = func(text string) { copied = text }

	request := cefmocks.NewMockRequest(t)
	request.EXPECT().GetURL().Return("dumb://api/clipboard-set").Once()
	request.EXPECT().GetMethod().Return(http.MethodPost).Once()
	request.EXPECT().GetHeaderByName(dumberBridgeNonceHeaderName).Return(bridgeNonce).Once()
	encoded := base64.StdEncoding.EncodeToString([]byte(`{"text":"copied from create"}`))
	request.EXPECT().GetHeaderByName(dumberBodyHeaderName).Return(encoded).Once()

	response := cefmocks.NewMockResponse(t)
	response.EXPECT().SetStatus(int32(http.StatusOK)).Once()
	response.EXPECT().SetStatusText(http.StatusText(http.StatusOK)).Once()
	response.EXPECT().SetMimeType("application/json").Once()
	expectAPIResponseHeaders(response)

	handler := h.Create(browser, nil, "", request)
	require.NotNil(t, handler)

	var responseLength int64
	handler.GetResponseHeaders(response, &responseLength, 0)
	require.Positive(t, responseLength)
	require.Equal(t, "copied from create", copied)
}

func TestSchemeHandler_APIClipboardSetRejectsInvalidBridgeNonce(t *testing.T) {
	oldNewResourceHandler := cefNewResourceHandler
	cefNewResourceHandler = func(impl purecef.ResourceHandler) purecef.ResourceHandler { return impl }
	defer func() { cefNewResourceHandler = oldNewResourceHandler }()

	h := newTestDumbSchemeHandler(t)

	browser := cefmocks.NewMockBrowser(t)
	h.bridgeNonceValidator = func(_ purecef.Browser, _ string) bool { return false }

	request := cefmocks.NewMockRequest(t)
	request.EXPECT().GetHeaderByName(dumberBridgeNonceHeaderName).Return("invalid").Once()

	response := cefmocks.NewMockResponse(t)
	response.EXPECT().SetStatus(int32(http.StatusForbidden)).Once()
	response.EXPECT().SetStatusText(http.StatusText(http.StatusForbidden)).Once()
	response.EXPECT().SetMimeType("application/json").Once()
	expectAPIResponseHeaders(response)

	handler := h.handleAPI(browser, http.MethodPost, "/api/clipboard-set", request)
	require.NotNil(t, handler)

	var responseLength int64
	handler.GetResponseHeaders(response, &responseLength, 0)
	require.Positive(t, responseLength)
}

func TestSchemeHandler_APIPopupOpenInvokesPopupCallback(t *testing.T) {
	oldNewResourceHandler := cefNewResourceHandler
	cefNewResourceHandler = func(impl purecef.ResourceHandler) purecef.ResourceHandler { return impl }
	defer func() { cefNewResourceHandler = oldNewResourceHandler }()

	h := newTestDumbSchemeHandler(t)

	const bridgeNonce = "bridge-nonce"
	browser := cefmocks.NewMockBrowser(t)
	h.bridgeNonceValidator = func(got purecef.Browser, gotNonce string) bool {
		return got == browser && gotNonce == bridgeNonce
	}

	var got rendererBridgePopupOpenPayload
	h.onPopupOpen = func(gotBrowser purecef.Browser, payload rendererBridgePopupOpenPayload) {
		require.Same(t, browser, gotBrowser)
		got = payload
	}

	request := cefmocks.NewMockRequest(t)
	request.EXPECT().GetHeaderByName(dumberBridgeNonceHeaderName).Return(bridgeNonce).Once()
	encoded := base64.StdEncoding.EncodeToString([]byte(`{"proxy_id":"popup-1","url":"https://example.com/login","frame_name":"Google login","user_gesture":true,"no_javascript_access":true}`))
	request.EXPECT().GetHeaderByName(dumberBodyHeaderName).Return(encoded).Once()

	response := cefmocks.NewMockResponse(t)
	response.EXPECT().SetStatus(int32(http.StatusOK)).Once()
	response.EXPECT().SetStatusText(http.StatusText(http.StatusOK)).Once()
	response.EXPECT().SetMimeType("application/json").Once()
	expectAPIResponseHeaders(response)

	handler := h.handleAPI(browser, http.MethodPost, "/api/popup-open", request)
	require.NotNil(t, handler)

	var responseLength int64
	handler.GetResponseHeaders(response, &responseLength, 0)
	require.Positive(t, responseLength)
	require.Equal(t, "popup-1", got.ProxyID)
	require.Equal(t, "https://example.com/login", got.URL)
	require.Equal(t, "Google login", got.FrameName)
	require.True(t, got.UserGesture)
	require.True(t, got.NoJavaScriptAccess)
}

func TestSchemeHandler_APIPopupNavigateInvokesPopupCallback(t *testing.T) {
	oldNewResourceHandler := cefNewResourceHandler
	cefNewResourceHandler = func(impl purecef.ResourceHandler) purecef.ResourceHandler { return impl }
	defer func() { cefNewResourceHandler = oldNewResourceHandler }()

	h := newTestDumbSchemeHandler(t)

	const bridgeNonce = "bridge-nonce"
	browser := cefmocks.NewMockBrowser(t)
	h.bridgeNonceValidator = func(got purecef.Browser, gotNonce string) bool {
		return got == browser && gotNonce == bridgeNonce
	}

	var got rendererBridgePopupNavigatePayload
	h.onPopupNavigate = func(gotBrowser purecef.Browser, payload rendererBridgePopupNavigatePayload) {
		require.Same(t, browser, gotBrowser)
		got = payload
	}

	request := cefmocks.NewMockRequest(t)
	request.EXPECT().GetHeaderByName(dumberBridgeNonceHeaderName).Return(bridgeNonce).Once()
	encoded := base64.StdEncoding.EncodeToString([]byte(`{"proxy_id":"popup-1","url":"https://example.com/callback"}`))
	request.EXPECT().GetHeaderByName(dumberBodyHeaderName).Return(encoded).Once()

	response := cefmocks.NewMockResponse(t)
	response.EXPECT().SetStatus(int32(http.StatusOK)).Once()
	response.EXPECT().SetStatusText(http.StatusText(http.StatusOK)).Once()
	response.EXPECT().SetMimeType("application/json").Once()
	expectAPIResponseHeaders(response)

	handler := h.handleAPI(browser, http.MethodPost, "/api/popup-navigate", request)
	require.NotNil(t, handler)

	var responseLength int64
	handler.GetResponseHeaders(response, &responseLength, 0)
	require.Positive(t, responseLength)
	require.Equal(t, "popup-1", got.ProxyID)
	require.Equal(t, "https://example.com/callback", got.URL)
}

func TestSchemeHandler_APIPopupCloseInvokesPopupCallback(t *testing.T) {
	oldNewResourceHandler := cefNewResourceHandler
	cefNewResourceHandler = func(impl purecef.ResourceHandler) purecef.ResourceHandler { return impl }
	defer func() { cefNewResourceHandler = oldNewResourceHandler }()

	h := newTestDumbSchemeHandler(t)

	const bridgeNonce = "bridge-nonce"
	browser := cefmocks.NewMockBrowser(t)
	h.bridgeNonceValidator = func(got purecef.Browser, gotNonce string) bool {
		return got == browser && gotNonce == bridgeNonce
	}

	var got rendererBridgePopupClosePayload
	h.onPopupClose = func(gotBrowser purecef.Browser, payload rendererBridgePopupClosePayload) {
		require.Same(t, browser, gotBrowser)
		got = payload
	}

	request := cefmocks.NewMockRequest(t)
	request.EXPECT().GetHeaderByName(dumberBridgeNonceHeaderName).Return(bridgeNonce).Once()
	encoded := base64.StdEncoding.EncodeToString([]byte(`{"proxy_id":"popup-1"}`))
	request.EXPECT().GetHeaderByName(dumberBodyHeaderName).Return(encoded).Once()

	response := cefmocks.NewMockResponse(t)
	response.EXPECT().SetStatus(int32(http.StatusOK)).Once()
	response.EXPECT().SetStatusText(http.StatusText(http.StatusOK)).Once()
	response.EXPECT().SetMimeType("application/json").Once()
	expectAPIResponseHeaders(response)

	handler := h.handleAPI(browser, http.MethodPost, "/api/popup-close", request)
	require.NotNil(t, handler)

	var responseLength int64
	handler.GetResponseHeaders(response, &responseLength, 0)
	require.Positive(t, responseLength)
	require.Equal(t, "popup-1", got.ProxyID)
}

func TestSchemeHandler_APIPopupBridgeRejectsInvalidPayloads(t *testing.T) {
	oldNewResourceHandler := cefNewResourceHandler
	cefNewResourceHandler = func(impl purecef.ResourceHandler) purecef.ResourceHandler { return impl }
	defer func() { cefNewResourceHandler = oldNewResourceHandler }()

	tests := []struct {
		name string
		path string
		body string
		wire func(*dumbSchemeHandler, *bool)
	}{
		{
			name: "open malformed json",
			path: "/api/popup-open",
			body: `{invalid`,
			wire: func(h *dumbSchemeHandler, called *bool) {
				h.onPopupOpen = func(purecef.Browser, rendererBridgePopupOpenPayload) { *called = true }
			},
		},
		{
			name: "open missing proxy id",
			path: "/api/popup-open",
			body: `{"url":"https://example.com/login"}`,
			wire: func(h *dumbSchemeHandler, called *bool) {
				h.onPopupOpen = func(purecef.Browser, rendererBridgePopupOpenPayload) { *called = true }
			},
		},
		{
			name: "navigate malformed json",
			path: "/api/popup-navigate",
			body: `{invalid`,
			wire: func(h *dumbSchemeHandler, called *bool) {
				h.onPopupNavigate = func(purecef.Browser, rendererBridgePopupNavigatePayload) { *called = true }
			},
		},
		{
			name: "navigate missing proxy id",
			path: "/api/popup-navigate",
			body: `{"url":"https://example.com/callback"}`,
			wire: func(h *dumbSchemeHandler, called *bool) {
				h.onPopupNavigate = func(purecef.Browser, rendererBridgePopupNavigatePayload) { *called = true }
			},
		},
		{
			name: "close malformed json",
			path: "/api/popup-close",
			body: `{invalid`,
			wire: func(h *dumbSchemeHandler, called *bool) {
				h.onPopupClose = func(purecef.Browser, rendererBridgePopupClosePayload) { *called = true }
			},
		},
		{
			name: "close missing proxy id",
			path: "/api/popup-close",
			body: `{"url":"https://example.com/callback"}`,
			wire: func(h *dumbSchemeHandler, called *bool) {
				h.onPopupClose = func(purecef.Browser, rendererBridgePopupClosePayload) { *called = true }
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			h := newTestDumbSchemeHandler(t)

			const bridgeNonce = "bridge-nonce"
			browser := cefmocks.NewMockBrowser(t)
			h.bridgeNonceValidator = func(got purecef.Browser, gotNonce string) bool {
				return got == browser && gotNonce == bridgeNonce
			}

			called := false
			test.wire(h, &called)

			request := cefmocks.NewMockRequest(t)
			request.EXPECT().GetHeaderByName(dumberBridgeNonceHeaderName).Return(bridgeNonce).Once()
			encoded := base64.StdEncoding.EncodeToString([]byte(test.body))
			request.EXPECT().GetHeaderByName(dumberBodyHeaderName).Return(encoded).Once()

			response := cefmocks.NewMockResponse(t)
			response.EXPECT().SetStatus(int32(http.StatusBadRequest)).Once()
			response.EXPECT().SetStatusText(http.StatusText(http.StatusBadRequest)).Once()
			response.EXPECT().SetMimeType("application/json").Once()
			expectAPIResponseHeaders(response)

			handler := h.handleAPI(browser, http.MethodPost, test.path, request)
			require.NotNil(t, handler)

			var responseLength int64
			handler.GetResponseHeaders(response, &responseLength, 0)
			require.Positive(t, responseLength)
			require.False(t, called)
		})
	}
}

func TestSchemeHandler_APIOptionsReturnsCORSPreflightHeaders(t *testing.T) {
	oldNewResourceHandler := cefNewResourceHandler
	cefNewResourceHandler = func(impl purecef.ResourceHandler) purecef.ResourceHandler { return impl }
	defer func() { cefNewResourceHandler = oldNewResourceHandler }()

	h := newTestDumbSchemeHandler(t)

	request := cefmocks.NewMockRequest(t)
	response := cefmocks.NewMockResponse(t)
	response.EXPECT().SetStatus(int32(http.StatusNoContent)).Once()
	response.EXPECT().SetStatusText(http.StatusText(http.StatusNoContent)).Once()
	response.EXPECT().SetMimeType("text/plain").Once()
	response.EXPECT().SetCharset("utf-8").Once()
	expectAPIResponseHeaders(response)

	handler := h.handleAPI(nil, http.MethodOptions, "/api/clipboard-set", request)
	require.NotNil(t, handler)

	var responseLength int64
	handler.GetResponseHeaders(response, &responseLength, 0)
	require.Zero(t, responseLength)
}
