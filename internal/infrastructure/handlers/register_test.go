package handlers

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/application/port/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestRegisterAccentHandlers_NilRouter(t *testing.T) {
	handler := mocks.NewMockAccentKeyHandler(t)
	err := RegisterAccentHandlers(context.Background(), nil, handler)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "router")
}

func TestRegisterAccentHandlers_NilHandler(t *testing.T) {
	router := mocks.NewMockWebUIHandlerRouter(t)
	err := RegisterAccentHandlers(context.Background(), router, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "handler")
}

func TestRegisterAccentHandlers_Success(t *testing.T) {
	router := mocks.NewMockWebUIHandlerRouter(t)
	handler := mocks.NewMockAccentKeyHandler(t)

	router.EXPECT().RegisterHandler("accent_key_press", mock.AnythingOfType("port.WebUIMessageHandlerFunc")).Return(nil)
	router.EXPECT().RegisterHandler("accent_key_release", mock.AnythingOfType("port.WebUIMessageHandlerFunc")).Return(nil)

	err := RegisterAccentHandlers(context.Background(), router, handler)
	require.NoError(t, err)
}

func TestRegisterAccentHandlers_KeyPress(t *testing.T) {
	ctx := context.Background()

	// Use a real router to capture the handler and invoke it.
	var captured port.WebUIMessageHandler
	router := mocks.NewMockWebUIHandlerRouter(t)
	router.EXPECT().RegisterHandler("accent_key_press", mock.AnythingOfType("port.WebUIMessageHandlerFunc")).
		Run(func(_ string, h port.WebUIMessageHandler) { captured = h }).
		Return(nil)
	router.EXPECT().RegisterHandler("accent_key_release", mock.AnythingOfType("port.WebUIMessageHandlerFunc")).Return(nil)

	handler := mocks.NewMockAccentKeyHandler(t)
	handler.EXPECT().OnKeyPressed(ctx, 'e', true).Return(false)

	err := RegisterAccentHandlers(ctx, router, handler)
	require.NoError(t, err)
	require.NotNil(t, captured)

	payload := json.RawMessage(`{"char":"e","shift":true}`)
	_, err = captured.Handle(ctx, 0, payload)
	require.NoError(t, err)
}
