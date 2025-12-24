package usecase

import (
	"context"
	"errors"

	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/domain/repository"
	"github.com/bnema/dumber/internal/logging"
)

// DeleteSessionUseCase errors.
var (
	ErrCannotDeleteCurrentSession = errors.New("cannot delete current session")
	ErrCannotDeleteActiveSession  = errors.New("cannot delete active session")
)

// DeleteSessionUseCase handles session deletion with proper validation.
type DeleteSessionUseCase struct {
	stateRepo   repository.SessionStateRepository
	sessionRepo repository.SessionRepository
	lockDir     string
}

// NewDeleteSessionUseCase creates a new DeleteSessionUseCase.
func NewDeleteSessionUseCase(
	stateRepo repository.SessionStateRepository,
	sessionRepo repository.SessionRepository,
	lockDir string,
) *DeleteSessionUseCase {
	return &DeleteSessionUseCase{
		stateRepo:   stateRepo,
		sessionRepo: sessionRepo,
		lockDir:     lockDir,
	}
}

// DeleteSessionInput contains the parameters for session deletion.
type DeleteSessionInput struct {
	SessionID        entity.SessionID
	CurrentSessionID entity.SessionID
}

// Execute deletes a session and its state snapshot.
// It validates that the session is not current or active before deletion.
func (uc *DeleteSessionUseCase) Execute(ctx context.Context, input DeleteSessionInput) error {
	log := logging.FromContext(ctx)

	// Validate not deleting current session
	if input.SessionID == input.CurrentSessionID && input.CurrentSessionID != "" {
		return ErrCannotDeleteCurrentSession
	}

	// Check session exists
	session, err := uc.sessionRepo.FindByID(ctx, input.SessionID)
	if err != nil {
		return err
	}
	if session == nil {
		return ErrSessionNotFound
	}

	// Check not active (EndedAt is nil OR has lock file)
	if session.IsActive() || isSessionLocked(uc.lockDir, input.SessionID) {
		return ErrCannotDeleteActiveSession
	}

	log.Info().Str("session_id", string(input.SessionID)).Msg("deleting session")

	// Delete snapshot (ignore "not found" - session may not have a snapshot)
	if uc.stateRepo != nil {
		if err := uc.stateRepo.DeleteSnapshot(ctx, input.SessionID); err != nil {
			log.Debug().Err(err).Msg("failed to delete session snapshot (may not exist)")
		}
	}

	// Delete session record
	return uc.sessionRepo.Delete(ctx, input.SessionID)
}
