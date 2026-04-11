package cef

import (
	"context"
	"errors"
	"testing"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/stretchr/testify/require"
)

type stubAccentHandler struct{}

func (stubAccentHandler) OnKeyPressed(context.Context, rune, bool) bool { return true }
func (stubAccentHandler) OnKeyReleased(context.Context, rune)           {}

func TestEngineRegisterHandlers_UsesInjectedRegistrar(t *testing.T) {
	called := false
	eng := &Engine{
		messageRouter: NewMessageRouter(context.Background()),
		registerHandlers: func(context.Context, port.WebUIHandlerRouter, port.HandlerDependencies) error {
			called = true
			return nil
		},
	}
	require.NoError(t, eng.RegisterHandlers(context.Background(), port.HandlerDependencies{}))
	require.True(t, called)
}

func TestEngineRegisterAccentHandlers_UsesInjectedRegistrar(t *testing.T) {
	called := false
	eng := &Engine{
		messageRouter: NewMessageRouter(context.Background()),
		registerAccentHandlers: func(context.Context, port.WebUIHandlerRouter, port.AccentKeyHandler) error {
			called = true
			return nil
		},
	}
	require.NoError(t, eng.RegisterAccentHandlers(context.Background(), stubAccentHandler{}))
	require.True(t, called)
}

func TestEngineRegisterHandlers_PropagatesRegistrarError(t *testing.T) {
	want := errors.New("boom")
	eng := &Engine{
		messageRouter: NewMessageRouter(context.Background()),
		registerHandlers: func(context.Context, port.WebUIHandlerRouter, port.HandlerDependencies) error {
			return want
		},
	}
	err := eng.RegisterHandlers(context.Background(), port.HandlerDependencies{})
	require.ErrorIs(t, err, want)
}
