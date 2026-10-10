package dto

// VimPageInteractionKind identifies an in-page Vim Mode interaction such as
// link hints, visual selection, or a one-shot text-object yank.
type VimPageInteractionKind int

const (
	// VimPageHintFollow labels visible clickable elements and activates the chosen one.
	VimPageHintFollow VimPageInteractionKind = iota + 1
	// VimPageHintFollowNew labels visible links and opens the chosen one in a new pane.
	VimPageHintFollowNew
	// VimPageHintYankURL labels visible links and copies the chosen URL.
	VimPageHintYankURL
	// VimPageVisual starts the caret and visual selection flow: text-anchor
	// hints, a caret that motions move, and charwise or linewise selections
	// that y copies without leaving the caret.
	VimPageVisual
	// VimPageYankObject copies one structural text object immediately.
	VimPageYankObject
)

// CapturesKeys reports whether the interaction consumes page keys until the
// page reports its end. One-shot yanks complete without key capture.
func (k VimPageInteractionKind) CapturesKeys() bool {
	switch k {
	case VimPageHintFollow, VimPageHintFollowNew, VimPageHintYankURL, VimPageVisual:
		return true
	default:
		return false
	}
}

// VimPageMode is the sub-mode a key-capturing page interaction reports as it
// moves between states, so the UI can label it.
type VimPageMode string

const (
	// VimPageModeHints is showing text-anchor hints to place the caret.
	VimPageModeHints VimPageMode = "hints"
	// VimPageModeCaret is moving a visible caret without selecting.
	VimPageModeCaret VimPageMode = "caret"
	// VimPageModeVisual is extending a charwise selection.
	VimPageModeVisual VimPageMode = "visual"
	// VimPageModeVisualLine is extending a linewise selection.
	VimPageModeVisualLine VimPageMode = "visual-line"
)

// Valid reports whether the mode is one the page runtime may report.
func (m VimPageMode) Valid() bool {
	switch m {
	case VimPageModeHints, VimPageModeCaret, VimPageModeVisual, VimPageModeVisualLine:
		return true
	default:
		return false
	}
}

// VimPageTextObject identifies the structural block copied by a text-object yank.
type VimPageTextObject int

const (
	VimPageTextObjectSection VimPageTextObject = iota + 1
	VimPageTextObjectParagraph
	VimPageTextObjectCode
	VimPageTextObjectTable
	VimPageTextObjectList
)

// VimPageInteractionRequest describes one in-page Vim interaction.
type VimPageInteractionRequest struct {
	Kind           VimPageInteractionKind
	Object         VimPageTextObject
	HighlightColor string
}

// VimNavigationOutcome tells the UI how to continue after a Vim action.
type VimNavigationOutcome struct {
	// CapturePageKeys asks the UI to forward keys to the page interaction until
	// the page reports that the interaction ended.
	CapturePageKeys bool
	// PageKind and PageMode name the key-capturing interaction and the
	// sub-mode it starts in, so the UI labels it without knowing actions.
	// Both are zero when CapturePageKeys is false.
	PageKind VimPageInteractionKind
	PageMode VimPageMode
}
