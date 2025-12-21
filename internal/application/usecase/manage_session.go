package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/domain/repository"
	"github.com/bnema/dumber/internal/logging"
	"github.com/rs/zerolog"
)

type ManageSessionUseCase struct {
	sessionRepo repository.SessionRepository
	loggerPort  port.SessionLogger
}

func NewManageSessionUseCase(sessionRepo repository.SessionRepository, loggerPort port.SessionLogger) *ManageSessionUseCase {
	return &ManageSessionUseCase{sessionRepo: sessionRepo, loggerPort: loggerPort}
}

type StartSessionInput struct {
	Type      entity.SessionType
	SessionID entity.SessionID
	Now       time.Time
	LogConfig port.SessionLogConfig
}

type StartSessionOutput struct {
	Session    *entity.Session
	Logger     zerolog.Logger
	LogCleanup func()
}

func (uc *ManageSessionUseCase) StartSession(ctx context.Context, input StartSessionInput) (*StartSessionOutput, error) {
	log := logging.FromContext(ctx)

	now := input.Now
	if now.IsZero() {
		now = time.Now()
	}

	sessionID := input.SessionID
	if sessionID == "" {
		sessionID = entity.SessionID(logging.GenerateSessionID())
	}

	sType := input.Type
	if sType == "" {
		sType = entity.SessionTypeBrowser
	}

	session := &entity.Session{
		ID:        sessionID,
		Type:      sType,
		StartedAt: now.UTC(),
	}
	if err := session.Validate(); err != nil {
		return nil, err
	}

	logger, cleanup, err := uc.loggerPort.CreateLogger(ctx, session.ID, input.LogConfig)
	if err != nil {
		log.Warn().Err(err).Msg("failed to create session logger")
		// proceed with fallback logger returned by adapter
	}

	sessionCtx := logging.WithContext(ctx, logger)
	if saveErr := uc.sessionRepo.Save(sessionCtx, session); saveErr != nil {
		if cleanup != nil {
			cleanup()
		}
		return nil, fmt.Errorf("save session: %w", saveErr)
	}

	logger.Info().Str("session_id", string(session.ID)).Str("type", string(session.Type)).Msg("session started")
	return &StartSessionOutput{Session: session, Logger: logger, LogCleanup: cleanup}, nil
}

func (uc *ManageSessionUseCase) EndSession(ctx context.Context, sessionID entity.SessionID, endedAt time.Time) error {
	log := logging.FromContext(ctx)
	if sessionID == "" {
		return fmt.Errorf("session id required")
	}
	if endedAt.IsZero() {
		endedAt = time.Now()
	}
	if err := uc.sessionRepo.MarkEnded(ctx, sessionID, endedAt); err != nil {
		return fmt.Errorf("mark session ended: %w", err)
	}
	log.Info().Str("session_id", string(sessionID)).Msg("session ended")
	return nil
}

func (uc *ManageSessionUseCase) GetActiveSession(ctx context.Context) (*entity.Session, error) {
	return uc.sessionRepo.GetActive(ctx)
}

func (uc *ManageSessionUseCase) GetRecentSessions(ctx context.Context, limit int) ([]*entity.Session, error) {
	return uc.sessionRepo.GetRecent(ctx, limit)
}
