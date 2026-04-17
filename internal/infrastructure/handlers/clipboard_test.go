package handlers

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/bnema/dumber/internal/application/port"
)

type recordingClipboardOrchestrator struct {
	mu        sync.Mutex
	selection port.SelectionClipboardInput
	explicit  port.ExplicitClipboardInput
}

func (r *recordingClipboardOrchestrator) HandleSelectionUpdate(_ context.Context, input port.SelectionClipboardInput) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.selection = input
	return nil
}

func (r *recordingClipboardOrchestrator) HandleExplicitCopy(_ context.Context, input port.ExplicitClipboardInput) error {
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
	if err != nil {
		t.Fatalf("Handle() error = %v, want nil", err)
	}

	orchestrator.mu.Lock()
	defer orchestrator.mu.Unlock()
	if orchestrator.selection.Text != "selected text" {
		t.Fatalf("selection text = %q, want %q", orchestrator.selection.Text, "selected text")
	}
	if orchestrator.selection.SourceEngine != port.ClipboardSourceWebKit {
		t.Fatalf("selection source = %q, want %q", orchestrator.selection.SourceEngine, port.ClipboardSourceWebKit)
	}
	if orchestrator.selection.ViewID != 42 {
		t.Fatalf("selection view id = %d, want %d", orchestrator.selection.ViewID, 42)
	}
}

func TestClipboardHandler_HandleExplicitCopy_DelegatesToOrchestrator(t *testing.T) {
	ctx := context.Background()
	orchestrator := &recordingClipboardOrchestrator{}
	handler := NewClipboardHandler(orchestrator)

	_, err := handler.HandleExplicitCopy().Handle(ctx, 42, json.RawMessage(`{"text":"copied text","action":"copy"}`))
	if err != nil {
		t.Fatalf("Handle() error = %v, want nil", err)
	}

	orchestrator.mu.Lock()
	defer orchestrator.mu.Unlock()
	if orchestrator.explicit.Text != "copied text" {
		t.Fatalf("explicit text = %q, want %q", orchestrator.explicit.Text, "copied text")
	}
	if orchestrator.explicit.Action != "copy" {
		t.Fatalf("explicit action = %q, want %q", orchestrator.explicit.Action, "copy")
	}
	if orchestrator.explicit.SourceEngine != port.ClipboardSourceWebKit {
		t.Fatalf("explicit source = %q, want %q", orchestrator.explicit.SourceEngine, port.ClipboardSourceWebKit)
	}
	if orchestrator.explicit.ViewID != 42 {
		t.Fatalf("explicit view id = %d, want %d", orchestrator.explicit.ViewID, 42)
	}
	if !orchestrator.explicit.NativeHandled {
		t.Fatal("explicit native handled = false, want true")
	}
}
