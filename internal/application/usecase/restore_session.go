package usecase

import (
	"context"
	"errors"
	"fmt"

	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/domain/repository"
	"github.com/bnema/dumber/internal/logging"
)

// ErrSessionNotFound is returned when a session state cannot be found.
var ErrSessionNotFound = errors.New("session not found")

// ErrVersionMismatch is returned when the session state version is incompatible.
var ErrVersionMismatch = errors.New("session state version mismatch")

// RestoreSessionUseCase handles restoring session state from a snapshot.
type RestoreSessionUseCase struct {
	stateRepo   repository.SessionStateRepository
	sessionRepo repository.SessionRepository
}

// NewRestoreSessionUseCase creates a new RestoreSessionUseCase.
func NewRestoreSessionUseCase(
	stateRepo repository.SessionStateRepository,
	sessionRepo repository.SessionRepository,
) *RestoreSessionUseCase {
	return &RestoreSessionUseCase{
		stateRepo:   stateRepo,
		sessionRepo: sessionRepo,
	}
}

// RestoreInput contains the parameters for restoring a session.
type RestoreInput struct {
	SessionID entity.SessionID
}

// RestoreOutput contains the restored session state.
type RestoreOutput struct {
	State *entity.SessionState
}

// Execute loads and validates a session state for restoration.
func (uc *RestoreSessionUseCase) Execute(ctx context.Context, input RestoreInput) (*RestoreOutput, error) {
	log := logging.FromContext(ctx)

	if input.SessionID == "" {
		return nil, fmt.Errorf("session id required")
	}

	log.Info().
		Str("session_id", string(input.SessionID)).
		Msg("restoring session state")

	state, err := uc.stateRepo.GetSnapshot(ctx, input.SessionID)
	if err != nil {
		return nil, fmt.Errorf("get session snapshot: %w", err)
	}
	if state == nil {
		return nil, ErrSessionNotFound
	}

	// Validate version compatibility
	if state.Version > entity.SessionStateVersion {
		log.Warn().
			Int("state_version", state.Version).
			Int("current_version", entity.SessionStateVersion).
			Msg("session state version is newer than current version")
		return nil, ErrVersionMismatch
	}

	log.Info().
		Str("session_id", string(input.SessionID)).
		Int("tab_count", len(state.Tabs)).
		Int("pane_count", state.CountPanes()).
		Msg("session state loaded for restoration")

	return &RestoreOutput{State: state}, nil
}

// DeleteSnapshot removes a session's snapshot (for cleanup after failed restore or user deletion).
func (uc *RestoreSessionUseCase) DeleteSnapshot(ctx context.Context, sessionID entity.SessionID) error {
	return uc.stateRepo.DeleteSnapshot(ctx, sessionID)
}
