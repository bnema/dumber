package cef

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	cefmocks "bnema.dev/purego-cef/cef/mocks"
	"github.com/stretchr/testify/require"

	"bnema.dev/dumber/internal/application/port"
)

func trustedMessageRequest(t *testing.T, body string) *cefmocks.MockRequest {
	t.Helper()
	request := cefmocks.NewMockRequest(t)
	request.EXPECT().GetHeaderByName("Origin").Return(actualInternalOrigin).Once()
	request.EXPECT().GetHeaderByName(dumberBodyHeaderName).Return(base64.StdEncoding.EncodeToString([]byte(body))).Once()
	return request
}

func TestHandleMessageAPIRunsSlowHandlerOffThread(t *testing.T) {
	useRawResourceHandlers(t)
	router := NewMessageRouter(context.Background())
	release := make(chan struct{})
	require.NoError(t, router.RegisterHandler("home_widget_data", port.WebUIMessageHandlerFunc(
		func(context.Context, port.WebViewID, json.RawMessage) (any, error) {
			<-release
			return map[string]string{"requestId": "r1"}, nil
		})))
	h, err := newDumbSchemeHandler(context.Background(), router, noopConfigPayload(), noopConfigPayload())
	require.NoError(t, err)

	returned := make(chan *asyncResourceHandler, 1)
	go func() {
		rh, _ := h.handleMessageAPI(trustedMessageRequest(t, `{"type":"home_widget_data","payload":{"requestId":"r1"}}`)).(*asyncResourceHandler)
		returned <- rh
	}()
	var rh *asyncResourceHandler
	select {
	case rh = <-returned:
	case <-time.After(2 * time.Second):
		t.Fatal("handleMessageAPI blocked on a slow handler")
	}
	require.NotNil(t, rh, "message API must use the async resource handler")

	cb := &contStubCallback{}
	require.Equal(t, int32(1), rh.Open(nil, nil, cb))
	cb.mu.Lock()
	require.Zero(t, cb.conts, "response must not be ready before the handler returns")
	cb.mu.Unlock()

	close(release)
	<-rh.done
	require.Eventually(t, func() bool { cb.mu.Lock(); defer cb.mu.Unlock(); return cb.conts == 1 }, 2*time.Second, 10*time.Millisecond)
	require.Equal(t, http.StatusOK, rh.statusCode)
	require.JSONEq(t, `{"data":{"requestId":"r1"}}`, string(rh.data))
	require.Equal(t, "*", rh.headers["Access-Control-Allow-Origin"])
}

func TestHandleMessageAPIInlineTypesStaySynchronous(t *testing.T) {
	useRawResourceHandlers(t)
	router := NewMessageRouter(context.Background())
	require.NoError(t, router.RegisterHandler("accent_key_press", port.WebUIMessageHandlerFunc(
		func(context.Context, port.WebViewID, json.RawMessage) (any, error) { return "ok", nil })))
	h, err := newDumbSchemeHandler(context.Background(), router, noopConfigPayload(), noopConfigPayload())
	require.NoError(t, err)

	got := h.handleMessageAPI(trustedMessageRequest(t, `{"type":"accent_key_press","payload":{}}`))
	_, async := got.(*asyncResourceHandler)
	require.False(t, async)
}

func TestHandleMessageAPIAsyncHandlerPanicIs500(t *testing.T) {
	useRawResourceHandlers(t)
	router := NewMessageRouter(context.Background())
	require.NoError(t, router.RegisterHandler("boom", port.WebUIMessageHandlerFunc(
		func(context.Context, port.WebViewID, json.RawMessage) (any, error) { panic("oops") })))
	h, err := newDumbSchemeHandler(context.Background(), router, noopConfigPayload(), noopConfigPayload())
	require.NoError(t, err)

	rh := h.handleMessageAPI(trustedMessageRequest(t, `{"type":"boom"}`)).(*asyncResourceHandler)
	rh.load()
	require.Equal(t, http.StatusInternalServerError, rh.statusCode)
	require.Contains(t, string(rh.data), "panicked")
}
