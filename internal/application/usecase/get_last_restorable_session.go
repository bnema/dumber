package usecase

import (
	"context"

	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/domain/repository"
	"github.com/bnema/dumber/internal/logging"
)

const (
	// maxSessionsToCheck is the number of recent sessions to check for restoration.
	maxSessionsToCheck = 10
)

// GetLastRestorableSessionUseCase finds the most recent session that can be auto-restored.
// A session is restorable if:
//   - It is not the current session
//   - It is not actively running (has ended or crashed without lock file)
//   - It has a valid snapshot with at least one tab
type GetLastRestorableSessionUseCase struct {
	sessionRepo repository.SessionRepository
	stateRepo   repository.SessionStateRepository
	lockDir     string
}

// NewGetLastRestorableSessionUseCase creates a new GetLastRestorableSessionUseCase.
func NewGetLastRestorableSessionUseCase(
	sessionRepo repository.SessionRepository,
	stateRepo repository.SessionStateRepository,
	lockDir string,
) *GetLastRestorableSessionUseCase {
	return &GetLastRestorableSessionUseCase{
		sessionRepo: sessionRepo,
		stateRepo:   stateRepo,
		lockDir:     lockDir,
	}
}

// GetLastRestorableSessionInput contains the parameters for finding a restorable session.
type GetLastRestorableSessionInput struct {
	// ExcludeSessionID is the current session ID to exclude from results.
	ExcludeSessionID entity.SessionID
}

// GetLastRestorableSessionOutput contains the found restorable session, if any.
type GetLastRestorableSessionOutput struct {
	// SessionID is the ID of the restorable session, or empty if none found.
	SessionID entity.SessionID
	// State is the session state to restore, or nil if none found.
	State *entity.SessionState
}

// Execute finds the most recent restorable session.
// Returns an empty output (not an error) if no restorable session is found.
func (uc *GetLastRestorableSessionUseCase) Execute(
	ctx context.Context,
	input GetLastRestorableSessionInput,
) (*GetLastRestorableSessionOutput, error) {
	log := logging.FromContext(ctx)

	sessions, err := uc.sessionRepo.GetRecent(ctx, maxSessionsToCheck)
	if err != nil {
		return nil, err
	}

	for _, session := range sessions {
		// Only consider browser sessions
		if session.Type != entity.SessionTypeBrowser {
			continue
		}

		// Skip current session
		if session.ID == input.ExcludeSessionID {
			continue
		}

		// Check if session is actively running
		// A session is "active" if ended_at IS NULL AND it has a lock file
		if session.IsActive() && isSessionLocked(uc.lockDir, session.ID) {
			log.Debug().
				Str("session_id", string(session.ID)).
				Msg("auto-restore: skipping active session")
			continue
		}

		// Try to get snapshot
		state, err := uc.stateRepo.GetSnapshot(ctx, session.ID)
		if err != nil {
			log.Debug().
				Err(err).
				Str("session_id", string(session.ID)).
				Msg("auto-restore: failed to get snapshot")
			continue
		}
		if state == nil {
			continue
		}

		// Skip sessions with no tabs
		if len(state.Tabs) == 0 {
			log.Debug().
				Str("session_id", string(session.ID)).
				Msg("auto-restore: skipping session with no tabs")
			continue
		}

		// Found a valid restorable session
		log.Info().
			Str("session_id", string(session.ID)).
			Int("tabs", len(state.Tabs)).
			Int("panes", state.CountPanes()).
			Msg("auto-restore: found restorable session")

		return &GetLastRestorableSessionOutput{
			SessionID: session.ID,
			State:     state,
		}, nil
	}

	// No restorable session found
	log.Debug().Msg("auto-restore: no restorable session found")
	return &GetLastRestorableSessionOutput{}, nil
}
