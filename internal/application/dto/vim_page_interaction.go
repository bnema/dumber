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
	// VimPageVisual starts a visual selection that motions extend and y copies.
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
}
