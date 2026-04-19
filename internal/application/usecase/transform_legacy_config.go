package usecase

import "github.com/bnema/dumber/internal/application/port"

// TransformLegacyConfigUseCase handles legacy config format transformation.
type TransformLegacyConfigUseCase struct {
	transformer port.ConfigTransformer
}

// NewTransformLegacyConfigUseCase creates a new use case instance.
func NewTransformLegacyConfigUseCase(transformer port.ConfigTransformer) *TransformLegacyConfigUseCase {
	return &TransformLegacyConfigUseCase{transformer: transformer}
}

// Execute transforms legacy config format in place.
func (uc *TransformLegacyConfigUseCase) Execute(rawConfig map[string]any) {
	uc.transformer.TransformLegacyActions(rawConfig)
}
