package usecase

import (
	"context"
	"fmt"

	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/domain/repository"
	"github.com/bnema/dumber/internal/logging"
)

// SnapshotSessionUseCase handles saving session state snapshots.
type SnapshotSessionUseCase struct {
	stateRepo repository.SessionStateRepository
}

// NewSnapshotSessionUseCase creates a new SnapshotSessionUseCase.
func NewSnapshotSessionUseCase(stateRepo repository.SessionStateRepository) *SnapshotSessionUseCase {
	return &SnapshotSessionUseCase{stateRepo: stateRepo}
}

// SnapshotInput contains the parameters for creating a session snapshot.
type SnapshotInput struct {
	SessionID entity.SessionID
	TabList   *entity.TabList
}

// Execute creates a snapshot of the current session state and saves it.
func (uc *SnapshotSessionUseCase) Execute(ctx context.Context, input SnapshotInput) error {
	log := logging.FromContext(ctx)

	if input.SessionID == "" {
		return fmt.Errorf("session id required")
	}

	state := entity.SnapshotFromTabList(input.SessionID, input.TabList)

	log.Debug().
		Str("session_id", string(input.SessionID)).
		Int("tab_count", len(state.Tabs)).
		Int("pane_count", state.CountPanes()).
		Msg("creating session snapshot")

	if err := uc.stateRepo.SaveSnapshot(ctx, state); err != nil {
		return fmt.Errorf("save session snapshot: %w", err)
	}

	return nil
}
