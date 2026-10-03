package port

// JSDialogType identifies the kind of JavaScript dialog requested by a page.
type JSDialogType int

const (
	// JSDialogAlert is window.alert(): message + OK.
	JSDialogAlert JSDialogType = iota
	// JSDialogConfirm is window.confirm(): message + OK/Cancel.
	JSDialogConfirm
	// JSDialogPrompt is window.prompt(): message + text input + OK/Cancel.
	JSDialogPrompt
	// JSDialogBeforeUnload is the "Leave page?" confirmation (Leave/Stay).
	JSDialogBeforeUnload
)

// JSDialogRequest describes a JavaScript dialog requested by a page.
// All text is untrusted page content and must be rendered as plain text.
type JSDialogRequest struct {
	Type JSDialogType
	// Origin is the URL of the frame that requested the dialog (best effort).
	Origin string
	// Message is the page-provided message (may be empty). It is empty for
	// JSDialogBeforeUnload: CEF always passes a fixed string, so the UI shows
	// its own text (like Chrome/Firefox).
	Message string
	// DefaultPrompt is the prefilled input for JSDialogPrompt.
	DefaultPrompt string
	// IsReload is true for beforeunload triggered by a reload.
	IsReload bool
}
