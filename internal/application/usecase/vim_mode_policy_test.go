package usecase

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestVimModePolicyEvaluate(t *testing.T) {
	tests := []struct {
		name  string
		input VimModePolicyInput
		want  VimModePolicyTransition
	}{
		{
			name: "activation stays allowed while page editable focused",
			input: VimModePolicyInput{
				Trigger:             VimModePolicyTriggerActivationAttempt,
				PageEditableFocused: true,
			},
			want: VimModePolicyTransitionStay,
		},
		{
			name: "activation stays allowed when page is not editable",
			input: VimModePolicyInput{
				Trigger:             VimModePolicyTriggerActivationAttempt,
				PageEditableFocused: false,
			},
			want: VimModePolicyTransitionStay,
		},
		{
			name: "active page editable focus exits vim mode",
			input: VimModePolicyInput{
				Trigger:              VimModePolicyTriggerPageEditableFocusChanged,
				VimModeActive:        true,
				PageEditableFocused:  true,
				EventInActiveContext: true,
			},
			want: VimModePolicyTransitionExit,
		},
		{
			name: "background page editable focus does not exit vim mode",
			input: VimModePolicyInput{
				Trigger:              VimModePolicyTriggerPageEditableFocusChanged,
				VimModeActive:        true,
				PageEditableFocused:  true,
				EventInActiveContext: false,
			},
			want: VimModePolicyTransitionStay,
		},
		{
			name: "overlay focus exits vim mode",
			input: VimModePolicyInput{
				Trigger:       VimModePolicyTriggerOverlayFocus,
				VimModeActive: true,
			},
			want: VimModePolicyTransitionExit,
		},
		{
			name: "omnibox focus exits vim mode",
			input: VimModePolicyInput{
				Trigger:       VimModePolicyTriggerOmniboxFocus,
				VimModeActive: true,
			},
			want: VimModePolicyTransitionExit,
		},
		{
			name: "find bar focus exits vim mode",
			input: VimModePolicyInput{
				Trigger:       VimModePolicyTriggerFindBarFocus,
				VimModeActive: true,
			},
			want: VimModePolicyTransitionExit,
		},
		{
			name: "context change preserves vim mode when scoped context stays valid",
			input: VimModePolicyInput{
				Trigger:                 VimModePolicyTriggerContextChanged,
				VimModeActive:           true,
				PreserveOnContextChange: true,
			},
			want: VimModePolicyTransitionStay,
		},
		{
			name: "context change exits vim mode when scoped context is lost",
			input: VimModePolicyInput{
				Trigger:                 VimModePolicyTriggerContextChanged,
				VimModeActive:           true,
				PreserveOnContextChange: false,
			},
			want: VimModePolicyTransitionExit,
		},
		{
			name: "non vim mode focus change stays normal",
			input: VimModePolicyInput{
				Trigger:       VimModePolicyTriggerOverlayFocus,
				VimModeActive: false,
			},
			want: VimModePolicyTransitionStay,
		},
	}

	uc := NewVimModePolicyUseCase()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, uc.Evaluate(tt.input))
		})
	}
}
