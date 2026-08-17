package usecase

import (
	"context"
	"errors"

	"github.com/bnema/dumber/internal/application/dto"
	"github.com/bnema/dumber/internal/application/port"
)

var (
	errUnsupportedVimNavigationAction = errors.New("vim navigation: unsupported action")
	errUnsupportedVimNavigationEngine = errors.New("vim navigation: unsupported webview capability")
)

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
			return errUnsupportedVimNavigationEngine
		}
		focuser.FocusNextInput()
		return nil
	}

	var request dto.SemanticNavigationRequest
	switch action {
	case "heading-next":
		request = dto.SemanticNavigationRequest{
			Target:         dto.SemanticNavigationTargetHeading,
			Direction:      dto.SemanticNavigationForward,
			Count:          count,
			HighlightColor: highlightColor,
		}
	case "heading-prev":
		request = dto.SemanticNavigationRequest{
			Target:         dto.SemanticNavigationTargetHeading,
			Direction:      dto.SemanticNavigationBackward,
			Count:          count,
			HighlightColor: highlightColor,
		}
	default:
		return errUnsupportedVimNavigationAction
	}

	navigator, ok := wv.(port.SemanticNavigable)
	if !ok {
		return errUnsupportedVimNavigationEngine
	}
	return navigator.NavigateSemantic(ctx, request)
}

// ClearSemanticNavigationHighlight removes the visual target left by semantic
// navigation when the active Vim Mode session ends.
func (*VimNavigationUseCase) ClearSemanticNavigationHighlight(ctx context.Context, wv port.WebView) error {
	if wv == nil {
		return nil
	}
	clearer, ok := wv.(port.SemanticNavigationHighlightClearer)
	if !ok {
		return nil
	}
	return clearer.ClearSemanticNavigationHighlight(ctx)
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
