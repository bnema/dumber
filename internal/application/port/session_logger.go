package port

import (
	"context"

	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/rs/zerolog"
)

type SessionLogConfig struct {
	Level         string
	Format        string
	TimeFormat    string
	LogDir        string
	WriteToStderr bool
	EnableFileLog bool
}

// SessionLogger creates a logger tied to a session.
type SessionLogger interface {
	CreateLogger(ctx context.Context, sessionID entity.SessionID, cfg SessionLogConfig) (zerolog.Logger, func(), error)
}
