package cef

import (
	"context"
	"net/http"
	"testing"

	purecef "bnema.dev/purego-cef/cef"
	cefmocks "bnema.dev/purego-cef/cef/mocks"
	"github.com/stretchr/testify/require"
)

func requireForbidden(t *testing.T, handler purecef.ResourceHandler) {
	t.Helper()
	require.NotNil(t, handler)
	response := cefmocks.NewMockResponse(t)
	response.EXPECT().SetStatus(int32(http.StatusForbidden)).Once()
	response.EXPECT().SetStatusText(http.StatusText(http.StatusForbidden)).Once()
	response.EXPECT().SetMimeType("application/json").Once()
	expectPrivateAPIResponseHeaders(response)
	var length int64
	handler.GetResponseHeaders(response, &length, 0)
}

func useRawResourceHandlers(t *testing.T) {
	t.Helper()
	old := cefNewResourceHandler
	cefNewResourceHandler = func(impl purecef.ResourceHandler) purecef.ResourceHandler { return impl }
	t.Cleanup(func() { cefNewResourceHandler = old })
}

func TestHandleMessageAPIRejectsEmbeddedFrameOrigins(t *testing.T) {
	useRawResourceHandlers(t)
	h, err := newDumbSchemeHandler(context.Background(), NewMessageRouter(context.Background()), noopConfigPayload(), noopConfigPayload())
	require.NoError(t, err)

	// A fetch from an iframe embedded in dumb://home carries the iframe's
	// own origin, or "null" when sandboxed without allow-same-origin. Both
	// are rejected, even with a trusted-looking referrer.
	for _, origin := range []string{"https://grafana.local", "http://192.168.1.10:3000", "null"} {
		request := cefmocks.NewMockRequest(t)
		request.EXPECT().GetHeaderByName("Origin").Return(origin).Once()
		requireForbidden(t, h.handleMessageAPI(request))
	}
}

func TestRejectSubframeSystemviewAPI(t *testing.T) {
	useRawResourceHandlers(t)
	h := newTestDumbSchemeHandler(t)

	for path := range subframeDeniedAPIPaths {
		sub := cefmocks.NewMockFrame(t)
		sub.EXPECT().IsMain().Return(false).Once()
		requireForbidden(t, h.rejectSubframeSystemviewAPI(sub, path))

		main := cefmocks.NewMockFrame(t)
		main.EXPECT().IsMain().Return(true).Once()
		require.Nil(t, h.rejectSubframeSystemviewAPI(main, path), path)

		requireForbidden(t, h.rejectSubframeSystemviewAPI(nil, path))
	}

	// Bridges used by external pages (nonce-gated) stay reachable from subframes.
	sub := cefmocks.NewMockFrame(t)
	require.Nil(t, h.rejectSubframeSystemviewAPI(sub, "/api/clipboard-set"))
}

func TestCreateRejectsMessageAPIFromSubframe(t *testing.T) {
	useRawResourceHandlers(t)
	h, err := newDumbSchemeHandler(context.Background(), NewMessageRouter(context.Background()), noopConfigPayload(), noopConfigPayload())
	require.NoError(t, err)

	request := cefmocks.NewMockRequest(t)
	request.EXPECT().GetURL().Return(actualInternalOrigin + "/api/message").Once()
	request.EXPECT().GetMethod().Return(http.MethodPost).Once()
	frame := cefmocks.NewMockFrame(t)
	frame.EXPECT().IsMain().Return(false).Once()

	requireForbidden(t, h.Create(nil, frame, "https", request))
}
