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

// Execute dispatches a configured Vim action to the active WebView. The
// outcome tells the UI whether the page now owns key input.
func (uc *VimNavigationUseCase) Execute(
	ctx context.Context, wv port.WebView, action string, count int, highlightColor string,
) (dto.VimNavigationOutcome, error) {
	if wv == nil {
		return dto.VimNavigationOutcome{}, errors.New("vim navigation: nil webview")
	}
	if request, ok := vimPageInteractionRequest(action, highlightColor); ok {
		return uc.startPageInteraction(ctx, wv, request)
	}
	return dto.VimNavigationOutcome{}, executeVimNavigation(ctx, wv, action, count, highlightColor)
}

func executeVimNavigation(ctx context.Context, wv port.WebView, action string, count int, highlightColor string) error {
	switch action {
	case "confirm":
		activator, ok := wv.(port.SemanticNavigationActivator)
		if !ok {
			return errUnsupportedVimNavigationEngine
		}
		return activator.ActivateSemanticNavigationTarget(ctx)
	case "focus-input":
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

var vimYankObjects = map[string]dto.VimPageTextObject{
	"yank-section":   dto.VimPageTextObjectSection,
	"yank-paragraph": dto.VimPageTextObjectParagraph,
	"yank-code":      dto.VimPageTextObjectCode,
	"yank-table":     dto.VimPageTextObjectTable,
	"yank-list":      dto.VimPageTextObjectList,
}

func vimPageInteractionRequest(action, highlightColor string) (dto.VimPageInteractionRequest, bool) {
	request := dto.VimPageInteractionRequest{HighlightColor: highlightColor}
	switch action {
	case "hint-follow":
		request.Kind = dto.VimPageHintFollow
	case "hint-follow-new":
		request.Kind = dto.VimPageHintFollowNew
	case "hint-yank-url":
		request.Kind = dto.VimPageHintYankURL
	case "visual":
		request.Kind = dto.VimPageVisual
	default:
		object, ok := vimYankObjects[action]
		if !ok {
			return dto.VimPageInteractionRequest{}, false
		}
		request.Kind = dto.VimPageYankObject
		request.Object = object
	}
	return request, true
}

func (*VimNavigationUseCase) startPageInteraction(
	ctx context.Context, wv port.WebView, request dto.VimPageInteractionRequest,
) (dto.VimNavigationOutcome, error) {
	interactor, ok := wv.(port.VimPageInteractor)
	if !ok {
		return dto.VimNavigationOutcome{}, errUnsupportedVimNavigationEngine
	}
	if err := interactor.StartVimPageInteraction(ctx, request); err != nil {
		return dto.VimNavigationOutcome{}, err
	}
	return dto.VimNavigationOutcome{CapturePageKeys: request.Kind.CapturesKeys()}, nil
}

// SendPageKey forwards one canonical key to the active in-page interaction.
func (*VimNavigationUseCase) SendPageKey(ctx context.Context, wv port.WebView, key string) error {
	interactor, ok := wv.(port.VimPageInteractor)
	if !ok {
		return errUnsupportedVimNavigationEngine
	}
	return interactor.SendVimPageKey(ctx, key)
}

// CancelPageInteraction ends any in-page interaction, removing its overlay or
// selection. WebViews without the capability have nothing to cancel.
func (*VimNavigationUseCase) CancelPageInteraction(ctx context.Context, wv port.WebView) error {
	interactor, ok := wv.(port.VimPageInteractor)
	if !ok {
		return nil
	}
	return interactor.CancelVimPageInteraction(ctx)
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
