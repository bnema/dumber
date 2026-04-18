package dto

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
	ViewID       uint64
}

// ExplicitClipboardInput carries explicit copy requests.
type ExplicitClipboardInput struct {
	Text          string
	SourceEngine  ClipboardSource
	ViewID        uint64
	Action        string
	NativeHandled bool
}
