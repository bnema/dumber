package dto

// SemanticNavigationTarget identifies a visible document target that Vim Mode
// can navigate without coupling the application layer to a browser engine.
type SemanticNavigationTarget int

const (
	SemanticNavigationTargetHeading SemanticNavigationTarget = iota
)

// SemanticNavigationDirection identifies the strict document-order direction
// for a semantic navigation request.
type SemanticNavigationDirection int

const (
	SemanticNavigationBackward SemanticNavigationDirection = -1
	SemanticNavigationForward  SemanticNavigationDirection = 1
)

// SemanticNavigationRequest describes one Vim structural movement. Count uses
// Vim semantics: adapters treat values less than one as one.
type SemanticNavigationRequest struct {
	Target         SemanticNavigationTarget
	Direction      SemanticNavigationDirection
	Count          int
	HighlightColor string
}
