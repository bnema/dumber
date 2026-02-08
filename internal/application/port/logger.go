package port

import (
	"context"

	"github.com/rs/zerolog"
)

// LoggerFromContext resolves the current request/session logger from context.
// Use cases depend on this boundary instead of importing infrastructure logging.
type LoggerFromContext func(ctx context.Context) *zerolog.Logger
