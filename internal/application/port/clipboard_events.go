package port

import (
	"context"

	"bnema.dev/dumber/internal/application/dto"
)

// ClipboardTextOrchestrator coordinates shared clipboard behavior.
type ClipboardTextOrchestrator interface {
	HandleSelectionUpdate(ctx context.Context, input dto.SelectionClipboardInput) error
	HandleExplicitCopy(ctx context.Context, input dto.ExplicitClipboardInput) error
}
