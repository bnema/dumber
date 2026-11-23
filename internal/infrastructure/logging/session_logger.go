package logging

import (
	"context"

	"bnema.dev/dumber/internal/application/port"
	"bnema.dev/dumber/internal/domain/entity"
	corelogging "bnema.dev/dumber/internal/logging"
	"github.com/rs/zerolog"
)

type SessionLoggerAdapter struct{}

var _ port.SessionLogger = (*SessionLoggerAdapter)(nil)

func NewSessionLoggerAdapter() *SessionLoggerAdapter {
	return &SessionLoggerAdapter{}
}

func (*SessionLoggerAdapter) CreateLogger(
	_ context.Context,
	sessionID entity.SessionID,
	cfg port.SessionLogConfig,
) (zerolog.Logger, func(), error) {
	logger, cleanup, err := corelogging.NewWithFile(
		corelogging.Config{
			Level:      corelogging.ParseLevel(cfg.Level),
			Format:     cfg.Format,
			TimeFormat: cfg.TimeFormat,
		},
		corelogging.FileConfig{
			Enabled:       cfg.EnableFileLog,
			LogDir:        cfg.LogDir,
			SessionID:     string(sessionID),
			WriteToStderr: cfg.WriteToStderr,
		},
	)
	if err != nil {
		fallback := corelogging.NewFromConfigValues(cfg.Level, cfg.Format)
		return fallback, func() {}, err
	}
	return logger, cleanup, nil
}
