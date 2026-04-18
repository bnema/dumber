package handlers

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/bnema/dumber/internal/application/dto"
	"github.com/stretchr/testify/require"
)

type recordingClipboardOrchestrator struct {
	mu        sync.Mutex
	selection dto.SelectionClipboardInput
	explicit  dto.ExplicitClipboardInput
}

func (r *recordingClipboardOrchestrator) HandleSelectionUpdate(_ context.Context, input dto.SelectionClipboardInput) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.selection = input
	return nil
}

func (r *recordingClipboardOrchestrator) HandleExplicitCopy(_ context.Context, input dto.ExplicitClipboardInput) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.explicit = input
	return nil
}

func TestClipboardHandler_HandleAutoCopySelection_DelegatesToOrchestrator(t *testing.T) {
	ctx := context.Background()
	orchestrator := &recordingClipboardOrchestrator{}
	handler := NewClipboardHandler(orchestrator)

	_, err := handler.HandleAutoCopySelection().Handle(ctx, 42, json.RawMessage(`{"text":"selected text"}`))
	require.NoError(t, err)

	orchestrator.mu.Lock()
	defer orchestrator.mu.Unlock()
	require.Equal(t, "selected text", orchestrator.selection.Text)
	require.Equal(t, dto.ClipboardSourceWebKit, orchestrator.selection.SourceEngine)
	require.Equal(t, uint64(42), orchestrator.selection.ViewID)
}

func TestClipboardHandler_HandleExplicitCopy_DelegatesToOrchestrator(t *testing.T) {
	ctx := context.Background()
	orchestrator := &recordingClipboardOrchestrator{}
	handler := NewClipboardHandler(orchestrator)

	_, err := handler.HandleExplicitCopy().Handle(ctx, 42, json.RawMessage(`{"text":"copied text","action":"copy"}`))
	require.NoError(t, err)

	orchestrator.mu.Lock()
	defer orchestrator.mu.Unlock()
	require.Equal(t, "copied text", orchestrator.explicit.Text)
	require.Equal(t, "copy", orchestrator.explicit.Action)
	require.Equal(t, dto.ClipboardSourceWebKit, orchestrator.explicit.SourceEngine)
	require.Equal(t, uint64(42), orchestrator.explicit.ViewID)
	require.True(t, orchestrator.explicit.NativeHandled)
}
