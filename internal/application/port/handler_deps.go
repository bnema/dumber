package port

import "context"

// AccentKeyHandler receives accent/dead-key events from the WebView JS bridge.
type AccentKeyHandler interface {
	OnKeyPressed(ctx context.Context, char rune, shiftHeld bool) bool
	OnKeyReleased(ctx context.Context, char rune)
}

// DownloadPreparer resolves download destination paths with deduplication.
type DownloadPreparer interface {
	Execute(ctx context.Context, input DownloadPrepareInput) *DownloadPrepareOutput
}

// DownloadPrepareInput matches usecase.PrepareDownloadInput.
type DownloadPrepareInput struct {
	SuggestedFilename string
	Response          DownloadResponse
	DownloadDir       string
}

// DownloadPrepareOutput matches usecase.PrepareDownloadOutput.
type DownloadPrepareOutput struct {
	Filename        string
	DestinationPath string
}

// AutoCopyConfig provides the clipboard auto-copy setting.
type AutoCopyConfig interface {
	IsAutoCopyEnabled() bool
}

// HandlerDeps holds handler dependencies built at bootstrap time.
// Lives in the port layer so both bootstrap and ui can depend on it
// without creating a reverse dependency.
type HandlerDeps struct {
	SaveConfig             func(context.Context, WebUIConfig) error
	KeybindingsGetter      KeybindingsGetter
	KeybindingSetter       KeybindingSetter
	KeybindingResetter     KeybindingResetter
	AllKeybindingsResetter AllKeybindingsResetter
}
