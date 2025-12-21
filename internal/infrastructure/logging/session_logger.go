package logging

import (
	"context"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/domain/entity"
	corelogging "github.com/bnema/dumber/internal/logging"
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
