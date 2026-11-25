package cef

import (
	"context"
	"net/http"
	"testing"

	"bnema.dev/dumber/internal/application/port"
	purecef "bnema.dev/purego-cef/cef"
	cefmocks "bnema.dev/purego-cef/cef/mocks"
	"github.com/stretchr/testify/require"
)

type stubHomeIconResolver map[string][]byte

func (s stubHomeIconResolver) ResolveHomeIconByID(_ context.Context, id string) ([]byte, string, error) {
	if data, ok := s[id]; ok {
		return data, "image/png", nil
	}
	return nil, "", port.ErrHomeIconNotFound
}

func homeIconTestHandler(t *testing.T) *dumbSchemeHandler {
	t.Helper()
	old := cefNewResourceHandler
	cefNewResourceHandler = func(impl purecef.ResourceHandler) purecef.ResourceHandler { return impl }
	t.Cleanup(func() { cefNewResourceHandler = old })
	h := newTestDumbSchemeHandler(t)
	h.setHomeIconResolver(stubHomeIconResolver{"jellyfin": []byte("png")})
	return h
}

func trustedHomeIconRequest(t *testing.T, query string) *cefmocks.MockRequest {
	t.Helper()
	request := cefmocks.NewMockRequest(t)
	request.EXPECT().GetHeaderByName("Origin").Return(actualInternalOrigin).Once()
	request.EXPECT().GetURL().Return(actualInternalOrigin + "/api/home-icon" + query).Once()
	return request
}

func TestHomeIconAPIServesPNGWithCacheControl(t *testing.T) {
	h := homeIconTestHandler(t)
	rh, ok := h.handleHomeIconAPI(trustedHomeIconRequest(t, "?service=jellyfin")).(*faviconResourceHandler)
	require.True(t, ok)
	rh.load()
	require.Equal(t, http.StatusOK, rh.statusCode)
	require.Equal(t, "image/png", rh.contentType)
	require.Equal(t, []byte("png"), rh.data)
	require.Equal(t, "max-age=3600", rh.headers["Cache-Control"])
}

func TestHomeIconAPIUnknownServiceIs404NoStore(t *testing.T) {
	h := homeIconTestHandler(t)
	rh, ok := h.handleHomeIconAPI(trustedHomeIconRequest(t, "?service=nope")).(*faviconResourceHandler)
	require.True(t, ok)
	rh.load()
	require.Equal(t, http.StatusNotFound, rh.statusCode)
	require.Equal(t, "no-store", rh.headers["Cache-Control"])
}

func TestHomeIconAPIRejectsBadID(t *testing.T) {
	h := homeIconTestHandler(t)
	for _, q := range []string{"", "?service=", "?service=../etc", "?service=Big%20Id"} {
		got, ok := h.handleHomeIconAPI(trustedHomeIconRequest(t, q)).(*staticResourceHandler)
		require.True(t, ok, q)
		require.Equal(t, http.StatusBadRequest, got.statusCode, q)
	}
}

func TestHomeIconAPIAppliesTrustGate(t *testing.T) {
	h := homeIconTestHandler(t)
	request := cefmocks.NewMockRequest(t)
	request.EXPECT().GetHeaderByName("Origin").Return("https://evil.example").Once()
	got, ok := h.handleHomeIconAPI(request).(*staticResourceHandler)
	require.True(t, ok)
	require.Equal(t, http.StatusForbidden, got.statusCode)
}

func TestHomeIconAPIIsRoutedForGET(t *testing.T) {
	h := homeIconTestHandler(t)
	got, handled := h.handleAPIGet("/api/home-icon", trustedHomeIconRequest(t, "?service=jellyfin"))
	require.True(t, handled)
	_, ok := got.(*faviconResourceHandler)
	require.True(t, ok)
}
