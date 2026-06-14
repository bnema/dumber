package usecase

// PageModePolicyTransition describes what the UI should do after a Page mode
// policy evaluation.
type PageModePolicyTransition string

const (
	PageModePolicyTransitionStay            PageModePolicyTransition = "stay"
	PageModePolicyTransitionExit            PageModePolicyTransition = "exit"
	PageModePolicyTransitionBlockActivation PageModePolicyTransition = "block_activation"
)

// PageModePolicyTrigger identifies the focus or activation event being
// evaluated.
type PageModePolicyTrigger string

const (
	PageModePolicyTriggerActivationAttempt        PageModePolicyTrigger = "activation_attempt"
	PageModePolicyTriggerOmniboxFocus             PageModePolicyTrigger = "omnibox_focus"
	PageModePolicyTriggerFindBarFocus             PageModePolicyTrigger = "find_bar_focus"
	PageModePolicyTriggerOverlayFocus             PageModePolicyTrigger = "overlay_focus"
	PageModePolicyTriggerPageEditableFocusChanged PageModePolicyTrigger = "page_editable_focus_changed"
	PageModePolicyTriggerContextChanged           PageModePolicyTrigger = "context_changed"
)

// PageModePolicyInput captures the current Page mode state and the event being
// evaluated.
type PageModePolicyInput struct {
	Trigger                 PageModePolicyTrigger
	PageModeActive          bool
	PageEditableFocused     bool
	EventInActiveContext    bool
	PreserveOnContextChange bool
}

// PageModePolicyUseCase evaluates whether Page mode should stay active, exit,
// or be blocked from activating.
type PageModePolicyUseCase struct{}

// NewPageModePolicyUseCase creates a PageModePolicyUseCase.
func NewPageModePolicyUseCase() *PageModePolicyUseCase {
	return &PageModePolicyUseCase{}
}

// Evaluate decides the Page mode transition for the provided event.
func (*PageModePolicyUseCase) Evaluate(input PageModePolicyInput) PageModePolicyTransition {
	switch input.Trigger {
	case PageModePolicyTriggerActivationAttempt:
		if input.PageEditableFocused {
			return PageModePolicyTransitionBlockActivation
		}
		return PageModePolicyTransitionStay
	case PageModePolicyTriggerOmniboxFocus,
		PageModePolicyTriggerFindBarFocus,
		PageModePolicyTriggerOverlayFocus:
		if input.PageModeActive {
			return PageModePolicyTransitionExit
		}
		return PageModePolicyTransitionStay
	case PageModePolicyTriggerPageEditableFocusChanged:
		if input.PageModeActive && input.PageEditableFocused && input.EventInActiveContext {
			return PageModePolicyTransitionExit
		}
		return PageModePolicyTransitionStay
	case PageModePolicyTriggerContextChanged:
		if input.PageModeActive && !input.PreserveOnContextChange {
			return PageModePolicyTransitionExit
		}
		return PageModePolicyTransitionStay
	default:
		return PageModePolicyTransitionStay
	}
}
