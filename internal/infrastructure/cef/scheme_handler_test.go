package cef

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"testing"

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

func TestDecodeClipboardSetBody_ReturnsText(t *testing.T) {
	text, err := decodeClipboardSetBody([]byte(`{"text":"copied from js"}`))
	require.NoError(t, err)
	require.Equal(t, "copied from js", text)
}

func TestDecodeClipboardSetBody_RejectsOversizePayload(t *testing.T) {
	body := []byte(`{"text":"` + strings.Repeat("x", maxClipboardBytes) + `"}`)

	_, err := decodeClipboardSetBody(body)

	require.Error(t, err)
	require.Contains(t, err.Error(), "payload too large")
}

func TestDecodeClipboardSetBody_RejectsInvalidPayload(t *testing.T) {
	_, err := decodeClipboardSetBody([]byte(`{"text":""}`))
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid payload")
}
