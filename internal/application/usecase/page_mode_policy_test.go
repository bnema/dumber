package usecase

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPageModePolicyEvaluate(t *testing.T) {
	tests := []struct {
		name  string
		input PageModePolicyInput
		want  PageModePolicyTransition
	}{
		{
			name: "activation blocked while page editable focused",
			input: PageModePolicyInput{
				Trigger:             PageModePolicyTriggerActivationAttempt,
				PageEditableFocused: true,
			},
			want: PageModePolicyTransitionBlockActivation,
		},
		{
			name: "activation stays allowed when page is not editable",
			input: PageModePolicyInput{
				Trigger:             PageModePolicyTriggerActivationAttempt,
				PageEditableFocused: false,
			},
			want: PageModePolicyTransitionStay,
		},
		{
			name: "active page editable focus exits page mode",
			input: PageModePolicyInput{
				Trigger:              PageModePolicyTriggerPageEditableFocusChanged,
				PageModeActive:       true,
				PageEditableFocused:  true,
				EventInActiveContext: true,
			},
			want: PageModePolicyTransitionExit,
		},
		{
			name: "background page editable focus does not exit page mode",
			input: PageModePolicyInput{
				Trigger:              PageModePolicyTriggerPageEditableFocusChanged,
				PageModeActive:       true,
				PageEditableFocused:  true,
				EventInActiveContext: false,
			},
			want: PageModePolicyTransitionStay,
		},
		{
			name: "overlay focus exits page mode",
			input: PageModePolicyInput{
				Trigger:        PageModePolicyTriggerOverlayFocus,
				PageModeActive: true,
			},
			want: PageModePolicyTransitionExit,
		},
		{
			name: "omnibox focus exits page mode",
			input: PageModePolicyInput{
				Trigger:        PageModePolicyTriggerOmniboxFocus,
				PageModeActive: true,
			},
			want: PageModePolicyTransitionExit,
		},
		{
			name: "find bar focus exits page mode",
			input: PageModePolicyInput{
				Trigger:        PageModePolicyTriggerFindBarFocus,
				PageModeActive: true,
			},
			want: PageModePolicyTransitionExit,
		},
		{
			name: "context change preserves page mode when scoped context stays valid",
			input: PageModePolicyInput{
				Trigger:                 PageModePolicyTriggerContextChanged,
				PageModeActive:          true,
				PreserveOnContextChange: true,
			},
			want: PageModePolicyTransitionStay,
		},
		{
			name: "context change exits page mode when scoped context is lost",
			input: PageModePolicyInput{
				Trigger:                 PageModePolicyTriggerContextChanged,
				PageModeActive:          true,
				PreserveOnContextChange: false,
			},
			want: PageModePolicyTransitionExit,
		},
		{
			name: "non page mode focus change stays normal",
			input: PageModePolicyInput{
				Trigger:        PageModePolicyTriggerOverlayFocus,
				PageModeActive: false,
			},
			want: PageModePolicyTransitionStay,
		},
	}

	uc := NewPageModePolicyUseCase()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, uc.Evaluate(tt.input))
		})
	}
}
