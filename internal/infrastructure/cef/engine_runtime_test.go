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

func TestEngineRegisterHandlers_StoresClipboardWiring(t *testing.T) {
	orchestrator := &stubClipboardTextOrchestrator{}
	onCopied := func(int) {}
	eng := &Engine{
		messageRouter: NewMessageRouter(context.Background()),
		registerHandlers: func(context.Context, port.WebUIHandlerRouter, port.HandlerDependencies) error {
			return nil
		},
	}

	require.NoError(t, eng.RegisterHandlers(context.Background(), port.HandlerDependencies{
		ClipboardTextOrchestrator: orchestrator,
		OnClipboardCopied:         onCopied,
	}))
	require.Same(t, orchestrator, eng.clipboardTextOrchestrator)
	require.NotNil(t, eng.onClipboardCopied)
}

func TestEngineClipboardSelectionUpdate_ForwardsViewID(t *testing.T) {
	orchestrator := &recordingClipboardTextOrchestrator{}
	eng := &Engine{clipboardTextOrchestrator: orchestrator}

	eng.handleClipboardSelectionUpdate(99, "selected")

	require.Equal(t, port.SelectionClipboardInput{Text: "selected", SourceEngine: port.ClipboardSourceCEF, ViewID: 99}, orchestrator.selection)
}

func TestEngineExplicitClipboardCopy_ForwardsViewID(t *testing.T) {
	orchestrator := &recordingClipboardTextOrchestrator{}
	eng := &Engine{clipboardTextOrchestrator: orchestrator}

	eng.handleExplicitClipboardBridgeText(77, "copy", "copied")

	require.Equal(t, port.ExplicitClipboardInput{Text: "copied", SourceEngine: port.ClipboardSourceCEF, ViewID: 77, Action: "copy"}, orchestrator.explicit)
}

type stubClipboardTextOrchestrator struct{}

type recordingClipboardTextOrchestrator struct {
	selection port.SelectionClipboardInput
	explicit  port.ExplicitClipboardInput
}

func (stubClipboardTextOrchestrator) HandleSelectionUpdate(context.Context, port.SelectionClipboardInput) error {
	return nil
}

func (stubClipboardTextOrchestrator) HandleExplicitCopy(context.Context, port.ExplicitClipboardInput) error {
	return nil
}

func (r *recordingClipboardTextOrchestrator) HandleSelectionUpdate(_ context.Context, input port.SelectionClipboardInput) error {
	r.selection = input
	return nil
}

func (r *recordingClipboardTextOrchestrator) HandleExplicitCopy(_ context.Context, input port.ExplicitClipboardInput) error {
	r.explicit = input
	return nil
}
