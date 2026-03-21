package handlers

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubRouter implements port.WebUIHandlerRouter for testing.
type stubRouter struct {
	handlers map[string]port.WebUIMessageHandler
}

func newStubRouter() *stubRouter {
	return &stubRouter{handlers: make(map[string]port.WebUIMessageHandler)}
}

func (r *stubRouter) RegisterHandler(msgType string, handler port.WebUIMessageHandler) error {
	r.handlers[msgType] = handler
	return nil
}

func (r *stubRouter) RegisterHandlerWithCallbacks(msgType, _, _, _ string, handler port.WebUIMessageHandler) error {
	r.handlers[msgType] = handler
	return nil
}

func TestRegisterAccentHandlers_NilRouter(t *testing.T) {
	ctx := context.Background()
	handler := &stubAccentHandler{}
	err := RegisterAccentHandlers(ctx, nil, handler)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "router")
}

func TestRegisterAccentHandlers_NilHandler(t *testing.T) {
	ctx := context.Background()
	router := newStubRouter()
	err := RegisterAccentHandlers(ctx, router, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "handler")
}

func TestRegisterAccentHandlers_Success(t *testing.T) {
	ctx := context.Background()
	router := newStubRouter()
	handler := &stubAccentHandler{}
	err := RegisterAccentHandlers(ctx, router, handler)
	require.NoError(t, err)
	assert.Contains(t, router.handlers, "accent_key_press")
	assert.Contains(t, router.handlers, "accent_key_release")
}

func TestRegisterAccentHandlers_KeyPress(t *testing.T) {
	ctx := context.Background()
	router := newStubRouter()
	handler := &stubAccentHandler{}
	err := RegisterAccentHandlers(ctx, router, handler)
	require.NoError(t, err)

	h := router.handlers["accent_key_press"]
	require.NotNil(t, h)

	payload := json.RawMessage(`{"char":"e","shift":true}`)
	_, err = h.Handle(ctx, 0, payload)
	require.NoError(t, err)
	assert.Equal(t, 'e', handler.lastPressed)
	assert.True(t, handler.lastShift)
}

// stubAccentHandler satisfies port.AccentKeyHandler for tests.
type stubAccentHandler struct {
	lastPressed rune
	lastShift   bool
}

func (h *stubAccentHandler) OnKeyPressed(_ context.Context, r rune, shift bool) bool {
	h.lastPressed = r
	h.lastShift = shift
	return false
}
func (*stubAccentHandler) OnKeyReleased(_ context.Context, _ rune) {}
