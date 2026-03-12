package handlers

import (
	"context"
	"testing"

	"github.com/bnema/dumber/internal/infrastructure/webkit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegisterAccentHandlers_NilRouter(t *testing.T) {
	ctx := context.Background()
	handler := &stubAccentHandler{}
	err := RegisterAccentHandlers(ctx, nil, handler)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "router")
}

func TestRegisterAccentHandlers_NilHandler(t *testing.T) {
	ctx := context.Background()
	router := webkit.NewMessageRouter(ctx)
	err := RegisterAccentHandlers(ctx, router, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "handler")
}

func TestRegisterAccentHandlers_Success(t *testing.T) {
	ctx := context.Background()
	router := webkit.NewMessageRouter(ctx)
	handler := &stubAccentHandler{}
	err := RegisterAccentHandlers(ctx, router, handler)
	require.NoError(t, err)
}

// stubAccentHandler satisfies AccentKeyHandler for tests.
type stubAccentHandler struct{}

func (*stubAccentHandler) OnKeyPressed(_ context.Context, _ rune, _ bool) bool { return false }
func (*stubAccentHandler) OnKeyReleased(_ context.Context, _ rune)             {}
