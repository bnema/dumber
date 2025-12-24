package usecase

import (
	"context"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/domain/entity"
)

// GetConfigSchemaUseCase retrieves configuration schema information.
type GetConfigSchemaUseCase struct {
	provider port.ConfigSchemaProvider
}

// NewGetConfigSchemaUseCase creates a new GetConfigSchemaUseCase.
func NewGetConfigSchemaUseCase(provider port.ConfigSchemaProvider) *GetConfigSchemaUseCase {
	return &GetConfigSchemaUseCase{
		provider: provider,
	}
}

// GetConfigSchemaInput contains input parameters for schema retrieval.
type GetConfigSchemaInput struct {
	// Reserved for future filtering options (e.g., by section)
}

// GetConfigSchemaOutput contains the schema information.
type GetConfigSchemaOutput struct {
	Keys []entity.ConfigKeyInfo
}

// Execute retrieves all configuration keys with their metadata.
func (uc *GetConfigSchemaUseCase) Execute(_ context.Context, _ GetConfigSchemaInput) (*GetConfigSchemaOutput, error) {
	keys := uc.provider.GetSchema()
	return &GetConfigSchemaOutput{
		Keys: keys,
	}, nil
}
