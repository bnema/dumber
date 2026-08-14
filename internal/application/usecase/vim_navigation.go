package usecase

import (
	"context"
	"errors"

	"github.com/bnema/dumber/internal/application/port"
)

var errUnsupportedVimNavigationAction = errors.New("vim navigation: unsupported action")

// VimNavigationUseCase maps configured Vim Mode navigation actions to optional
// WebView capabilities. The UI layer supplies the active WebView and remains
// responsible only for event and theme context.
type VimNavigationUseCase struct{}

// NewVimNavigationUseCase creates a stateless Vim navigation use case.
func NewVimNavigationUseCase() *VimNavigationUseCase {
	return &VimNavigationUseCase{}
}

// Execute dispatches a configured semantic Vim action to the active WebView.
func (*VimNavigationUseCase) Execute(ctx context.Context, wv port.WebView, action string, count int, highlightColor string) error {
	if wv == nil {
		return errors.New("vim navigation: nil webview")
	}

	if action == "focus-input" {
		focuser, ok := wv.(port.PageInputFocuser)
		if !ok {
			return nil
		}
		focuser.FocusNextInput()
		return nil
	}

	var request port.SemanticNavigationRequest
	switch action {
	case "heading-next":
		request = port.SemanticNavigationRequest{
			Target:         port.SemanticNavigationTargetHeading,
			Direction:      port.SemanticNavigationForward,
			Count:          count,
			HighlightColor: highlightColor,
		}
	case "heading-prev":
		request = port.SemanticNavigationRequest{
			Target:         port.SemanticNavigationTargetHeading,
			Direction:      port.SemanticNavigationBackward,
			Count:          count,
			HighlightColor: highlightColor,
		}
	default:
		return errUnsupportedVimNavigationAction
	}

	navigator, ok := wv.(port.SemanticNavigable)
	if !ok {
		return nil
	}
	return navigator.NavigateSemantic(ctx, request)
}

// NavigatePageFocus keeps focus traversal inside a page when the active page
// control would otherwise hand the host window a Tab event.
func (*VimNavigationUseCase) NavigatePageFocus(wv port.WebView, backward bool) bool {
	if wv == nil {
		return false
	}
	navigator, ok := wv.(port.PageFocusNavigator)
	if !ok {
		return false
	}
	navigator.NavigatePageFocus(backward)
	return true
}
