package port

import "context"

// ClipboardSource identifies the originating engine for clipboard events.
type ClipboardSource string

const (
	// ClipboardSourceCEF marks events from the CEF engine.
	ClipboardSourceCEF ClipboardSource = "cef"
	// ClipboardSourceWebKit marks events from the WebKit engine.
	ClipboardSourceWebKit ClipboardSource = "webkit"
)

// SelectionClipboardInput carries auto-copy selection updates.
type SelectionClipboardInput struct {
	Text         string
	SourceEngine ClipboardSource
	ViewID       WebViewID
}

// ExplicitClipboardInput carries explicit copy requests.
type ExplicitClipboardInput struct {
	Text         string
	SourceEngine ClipboardSource
	ViewID       WebViewID
	Action       string
}

// ClipboardTextOrchestrator coordinates shared clipboard behavior.
type ClipboardTextOrchestrator interface {
	HandleSelectionUpdate(ctx context.Context, input SelectionClipboardInput) error
	HandleExplicitCopy(ctx context.Context, input ExplicitClipboardInput) error
}
