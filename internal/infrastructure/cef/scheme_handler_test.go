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
	request.EXPECT().GetHeaderByName("X-Dumber-Body").Return(encoded).Once()

	body := readBodyFromHeader(request)

	require.JSONEq(t, `{"text":"copied from js"}`, string(body))
}

func TestSchemeHandler_APIClipboardSetPathReturnsNotFound(t *testing.T) {
	oldNewResourceHandler := cefNewResourceHandler
	cefNewResourceHandler = func(impl purecef.ResourceHandler) purecef.ResourceHandler { return impl }
	defer func() { cefNewResourceHandler = oldNewResourceHandler }()

	h, err := newDumbSchemeHandler(context.Background(), nil, nil, func() ([]byte, error) { return []byte(`{}`), nil }, func() ([]byte, error) { return []byte(`{}`), nil })
	require.NoError(t, err)

	response := cefmocks.NewMockResponse(t)
	response.EXPECT().SetStatus(int32(http.StatusNotFound)).Once()
	response.EXPECT().SetStatusText(http.StatusText(http.StatusNotFound)).Once()
	response.EXPECT().SetMimeType("application/json").Once()

	handler := h.handleAPI(nil, http.MethodPost, "/api/clipboard-set", nil)
	require.NotNil(t, handler)

	var responseLength int64
	handler.GetResponseHeaders(response, unsafe.Pointer(&responseLength), 0)
	require.Positive(t, responseLength)
}
