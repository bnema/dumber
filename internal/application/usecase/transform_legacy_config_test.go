package usecase

import (
	"testing"

	"github.com/bnema/dumber/internal/application/port/mocks"
	"github.com/stretchr/testify/mock"
)

func TestTransformLegacyConfigUseCase_Execute(t *testing.T) {
	mockTransformer := mocks.NewMockConfigTransformer(t)

	rawConfig := map[string]any{"test": "data"}

	mockTransformer.EXPECT().TransformLegacyActions(mock.Anything).Return()

	uc := NewTransformLegacyConfigUseCase(mockTransformer)
	uc.Execute(rawConfig)

	// AssertExpectations is called automatically via t.Cleanup
}

func TestTransformLegacyConfigUseCase_Execute_CallsTransformer(t *testing.T) {
	mockTransformer := mocks.NewMockConfigTransformer(t)

	rawConfig := map[string]any{
		"workspace": map[string]any{
			"pane_mode": map[string]any{
				"actions": map[string]any{
					"focus-left": []any{"h"},
				},
			},
		},
	}

	// Verify the transformer receives the exact rawConfig
	mockTransformer.EXPECT().TransformLegacyActions(rawConfig).Return()

	uc := NewTransformLegacyConfigUseCase(mockTransformer)
	uc.Execute(rawConfig)
}
