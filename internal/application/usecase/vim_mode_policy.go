package usecase

// VimModePolicyTransition describes what the UI should do after a Vim mode
// policy evaluation.
type VimModePolicyTransition string

const (
	VimModePolicyTransitionStay VimModePolicyTransition = "stay"
	VimModePolicyTransitionExit VimModePolicyTransition = "exit"
)

// VimModePolicyTrigger identifies the focus or activation event being
// evaluated.
type VimModePolicyTrigger string

const (
	VimModePolicyTriggerActivationAttempt        VimModePolicyTrigger = "activation_attempt"
	VimModePolicyTriggerOmniboxFocus             VimModePolicyTrigger = "omnibox_focus"
	VimModePolicyTriggerFindBarFocus             VimModePolicyTrigger = "find_bar_focus"
	VimModePolicyTriggerOverlayFocus             VimModePolicyTrigger = "overlay_focus"
	VimModePolicyTriggerPageEditableFocusChanged VimModePolicyTrigger = "page_editable_focus_changed"
	VimModePolicyTriggerContextChanged           VimModePolicyTrigger = "context_changed"
)

// VimModePolicyInput captures the current Vim mode state and the event being
// evaluated.
type VimModePolicyInput struct {
	Trigger                 VimModePolicyTrigger
	VimModeActive           bool
	PageEditableFocused     bool
	EventInActiveContext    bool
	PreserveOnContextChange bool
}

// VimModePolicyUseCase evaluates whether Vim mode should stay active, exit,
// or be blocked from activating.
type VimModePolicyUseCase struct{}

// NewVimModePolicyUseCase creates a VimModePolicyUseCase.
func NewVimModePolicyUseCase() *VimModePolicyUseCase {
	return &VimModePolicyUseCase{}
}

// Evaluate decides the Vim mode transition for the provided event.
func (*VimModePolicyUseCase) Evaluate(input VimModePolicyInput) VimModePolicyTransition {
	switch input.Trigger {
	case VimModePolicyTriggerActivationAttempt:
		// Ctrl+Y is the explicit Vim Mode activation command. It must remain
		// available while a page input is focused so users can leave typing mode
		// without first moving focus elsewhere.
		return VimModePolicyTransitionStay
	case VimModePolicyTriggerOmniboxFocus,
		VimModePolicyTriggerFindBarFocus,
		VimModePolicyTriggerOverlayFocus:
		if input.VimModeActive {
			return VimModePolicyTransitionExit
		}
		return VimModePolicyTransitionStay
	case VimModePolicyTriggerPageEditableFocusChanged:
		if input.VimModeActive && input.PageEditableFocused && input.EventInActiveContext {
			return VimModePolicyTransitionExit
		}
		return VimModePolicyTransitionStay
	case VimModePolicyTriggerContextChanged:
		if input.VimModeActive && !input.PreserveOnContextChange {
			return VimModePolicyTransitionExit
		}
		return VimModePolicyTransitionStay
	default:
		return VimModePolicyTransitionStay
	}
}
