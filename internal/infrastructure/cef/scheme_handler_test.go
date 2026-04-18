package cef

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"testing"
	"unsafe"

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
	h, err := newDumbSchemeHandler(context.Background(), nil, nil, nil, func() ([]byte, error) { return nil, nil })
	require.Error(t, err)
	require.Nil(t, h)
	require.Contains(t, err.Error(), "current config payload builder")
}

func TestNewDumbSchemeHandler_NilDefaultConfigPayloadFails(t *testing.T) {
	h, err := newDumbSchemeHandler(context.Background(), nil, nil, func() ([]byte, error) { return nil, nil }, nil)
	require.Error(t, err)
	require.Nil(t, h)
	require.Contains(t, err.Error(), "default config payload builder")
}

func TestReadBodyFromHeader_DecodesBase64Payload(t *testing.T) {
	request := cefmocks.NewMockRequest(t)
	encoded := base64.StdEncoding.EncodeToString([]byte(`{"text":"copied from js"}`))
	request.EXPECT().GetHeaderByName(dumberBodyHeaderName).Return(encoded).Once()

	body := readBodyFromHeader(request)

	require.JSONEq(t, `{"text":"copied from js"}`, string(body))
}

func TestSchemeHandler_APIClipboardSetPathWritesClipboardPayload(t *testing.T) {
	oldNewResourceHandler := cefNewResourceHandler
	cefNewResourceHandler = func(impl purecef.ResourceHandler) purecef.ResourceHandler { return impl }
	defer func() { cefNewResourceHandler = oldNewResourceHandler }()

	h, err := newDumbSchemeHandler(context.Background(), nil, nil, func() ([]byte, error) { return []byte(`{}`), nil }, func() ([]byte, error) { return []byte(`{}`), nil })
	require.NoError(t, err)

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
	handler.GetResponseHeaders(response, unsafe.Pointer(&responseLength), 0)
	require.Positive(t, responseLength)
	require.Equal(t, "copied from js", copied)
}

func TestSchemeHandler_APIFocusSyncRejectsRequestsWithoutTrustedBridgeHeader(t *testing.T) {
	oldNewResourceHandler := cefNewResourceHandler
	cefNewResourceHandler = func(impl purecef.ResourceHandler) purecef.ResourceHandler { return impl }
	defer func() { cefNewResourceHandler = oldNewResourceHandler }()

	h, err := newDumbSchemeHandler(context.Background(), nil, nil, func() ([]byte, error) { return []byte(`{}`), nil }, func() ([]byte, error) { return []byte(`{}`), nil })
	require.NoError(t, err)

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
	handler.GetResponseHeaders(response, unsafe.Pointer(&responseLength), 0)
	require.Positive(t, responseLength)
}

func TestSchemeHandler_APIFocusSyncInvokesEditableFocusCallback(t *testing.T) {
	oldNewResourceHandler := cefNewResourceHandler
	cefNewResourceHandler = func(impl purecef.ResourceHandler) purecef.ResourceHandler { return impl }
	defer func() { cefNewResourceHandler = oldNewResourceHandler }()

	h, err := newDumbSchemeHandler(context.Background(), nil, nil, func() ([]byte, error) { return []byte(`{}`), nil }, func() ([]byte, error) { return []byte(`{}`), nil })
	require.NoError(t, err)

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
	handler.GetResponseHeaders(response, unsafe.Pointer(&responseLength), 0)
	require.Positive(t, responseLength)
	require.Same(t, browser, focused)
}

func TestSchemeHandler_Create_ConceptualAPIRequestBypassesRedirect(t *testing.T) {
	oldNewResourceHandler := cefNewResourceHandler
	cefNewResourceHandler = func(impl purecef.ResourceHandler) purecef.ResourceHandler { return impl }
	defer func() { cefNewResourceHandler = oldNewResourceHandler }()

	h, err := newDumbSchemeHandler(context.Background(), nil, nil, func() ([]byte, error) { return []byte(`{}`), nil }, func() ([]byte, error) { return []byte(`{}`), nil })
	require.NoError(t, err)

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
	handler.GetResponseHeaders(response, unsafe.Pointer(&responseLength), 0)
	require.Positive(t, responseLength)
	require.Equal(t, "copied from create", copied)
}

func TestSchemeHandler_APIClipboardSetRejectsInvalidBridgeNonce(t *testing.T) {
	oldNewResourceHandler := cefNewResourceHandler
	cefNewResourceHandler = func(impl purecef.ResourceHandler) purecef.ResourceHandler { return impl }
	defer func() { cefNewResourceHandler = oldNewResourceHandler }()

	h, err := newDumbSchemeHandler(context.Background(), nil, nil, func() ([]byte, error) { return []byte(`{}`), nil }, func() ([]byte, error) { return []byte(`{}`), nil })
	require.NoError(t, err)

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
	handler.GetResponseHeaders(response, unsafe.Pointer(&responseLength), 0)
	require.Positive(t, responseLength)
}

func TestSchemeHandler_APIOptionsReturnsCORSPreflightHeaders(t *testing.T) {
	oldNewResourceHandler := cefNewResourceHandler
	cefNewResourceHandler = func(impl purecef.ResourceHandler) purecef.ResourceHandler { return impl }
	defer func() { cefNewResourceHandler = oldNewResourceHandler }()

	h, err := newDumbSchemeHandler(context.Background(), nil, nil, func() ([]byte, error) { return []byte(`{}`), nil }, func() ([]byte, error) { return []byte(`{}`), nil })
	require.NoError(t, err)

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
	handler.GetResponseHeaders(response, unsafe.Pointer(&responseLength), 0)
	require.Zero(t, responseLength)
}
