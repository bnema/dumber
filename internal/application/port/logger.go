package port

import (
	"context"

	"github.com/rs/zerolog"
)

// LoggerFromContext resolves the current request/session logger from context.
// Use cases depend on this boundary instead of importing infrastructure logging.
type LoggerFromContext func(ctx context.Context) *zerolog.Logger

// LogField is a structured logging field carried across application boundaries.
type LogField struct {
	Key   string
	Value any
}

// Field creates a structured log field.
func Field(key string, value any) LogField {
	return LogField{Key: key, Value: value}
}

// Logger is the logging boundary used by adapters that must not import a
// concrete logging implementation directly.
type Logger interface {
	Debug(msg string, fields ...LogField)
	Info(msg string, fields ...LogField)
	Warn(msg string, fields ...LogField)
}
