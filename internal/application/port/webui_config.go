package port

import (
	"context"

	"github.com/bnema/dumber/internal/application/dto"
)

// SystemviewConfigReader reads the current and default systemview config payloads.
type SystemviewConfigReader interface {
	Current(ctx context.Context) (dto.SystemviewConfigPayload, error)
	Default(ctx context.Context) (dto.SystemviewConfigPayload, error)
}

// WebUIConfigSaver persists WebUI configuration changes.
type WebUIConfigSaver interface {
	SaveWebUIConfig(ctx context.Context, cfg dto.WebUIConfig) error
}
