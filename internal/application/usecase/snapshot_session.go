package usecase

import (
	"context"
	"fmt"
	"time"

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

	// v2: window-scoped snapshot. A non-nil slice, including an empty one,
	// requests the v2 path; leave nil when using the legacy TabList path.
	Windows           []entity.WindowTabListState
	ActiveWindowIndex int
}

// Execute creates a snapshot of the current session state and saves it.
// When Windows is non-nil, uses the v2 window-scoped path, including explicit
// zero-window snapshots. Otherwise falls back to the legacy flat TabList path.
func (uc *SnapshotSessionUseCase) Execute(ctx context.Context, input SnapshotInput) error {
	log := logging.FromContext(ctx)

	if input.SessionID == "" {
		return fmt.Errorf("session id required")
	}
	if input.Windows != nil && input.TabList != nil {
		return fmt.Errorf("windows and tab list are mutually exclusive")
	}

	var state *entity.SessionState

	if input.Windows != nil {
		state = entity.SnapshotFromWindowTabLists(input.SessionID, input.Windows, input.ActiveWindowIndex, time.Now())
		log.Debug().
			Str("session_id", string(input.SessionID)).
			Int("window_count", len(state.Windows)).
			Int("active_window", state.ActiveWindowIndex).
			Int("pane_count", state.CountPanes()).
			Msg("creating window-scoped session snapshot")
	} else {
		state = entity.SnapshotFromTabList(input.SessionID, input.TabList)
		log.Debug().
			Str("session_id", string(input.SessionID)).
			Int("tab_count", len(state.Tabs)).
			Int("pane_count", state.CountPanes()).
			Msg("creating session snapshot")
	}

	if err := uc.stateRepo.SaveSnapshot(ctx, state); err != nil {
		return fmt.Errorf("save session snapshot: %w", err)
	}

	return nil
}
