package usecase_test

import (
	"testing"
	"time"

	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/domain/entity"
	repomocks "github.com/bnema/dumber/internal/domain/repository/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListSessionsUseCase_Execute_ReturnsSessionsWithState(t *testing.T) {
	ctx := testContext()

	sessionRepo := repomocks.NewMockSessionRepository(t)
	stateRepo := repomocks.NewMockSessionStateRepository(t)

	endedAt := time.Now().Add(-time.Hour)
	sessions := []*entity.Session{
		{
			ID:        "20251224_120000_abc1",
			Type:      entity.SessionTypeBrowser,
			StartedAt: time.Now().Add(-2 * time.Hour),
			EndedAt:   &endedAt,
		},
		{
			ID:        "20251224_130000_def2",
			Type:      entity.SessionTypeBrowser,
			StartedAt: time.Now().Add(-time.Hour),
			EndedAt:   nil, // Active
		},
	}

	states := []*entity.SessionState{
		{
			SessionID: "20251224_120000_abc1",
			SavedAt:   time.Now().Add(-time.Hour),
			Tabs: []entity.TabSnapshot{
				{ID: "tab1"},
				{ID: "tab2"},
			},
		},
	}

	sessionRepo.EXPECT().GetRecent(ctx, 50).Return(sessions, nil)
	stateRepo.EXPECT().GetAllSnapshots(ctx).Return(states, nil)

	uc := usecase.NewListSessionsUseCase(sessionRepo, stateRepo, "")

	output, err := uc.Execute(ctx, "", 50)
	require.NoError(t, err)
	require.NotNil(t, output)
	require.Len(t, output.Sessions, 2)

	// Active session should be first (or current, but we have no current)
	// The sorting puts active sessions before inactive
	activeSession := output.Sessions[0]
	assert.True(t, activeSession.IsActive)

	// Check session with state has tab count
	var sessionWithState *entity.SessionInfo
	for i := range output.Sessions {
		if output.Sessions[i].Session.ID == "20251224_120000_abc1" {
			sessionWithState = &output.Sessions[i]
			break
		}
	}
	require.NotNil(t, sessionWithState)
	assert.Equal(t, 2, sessionWithState.TabCount)
}

func TestListSessionsUseCase_GetPurgeableSessions_ReturnsOnlyInactive(t *testing.T) {
	ctx := testContext()

	sessionRepo := repomocks.NewMockSessionRepository(t)
	stateRepo := repomocks.NewMockSessionStateRepository(t)

	endedAt := time.Now().Add(-time.Hour)
	sessions := []*entity.Session{
		{
			ID:        "20251224_120000_inactive1",
			Type:      entity.SessionTypeBrowser,
			StartedAt: time.Now().Add(-3 * time.Hour),
			EndedAt:   &endedAt, // Ended - purgeable
		},
		{
			ID:        "20251224_130000_active1",
			Type:      entity.SessionTypeBrowser,
			StartedAt: time.Now().Add(-time.Hour),
			EndedAt:   nil, // Active - NOT purgeable
		},
		{
			ID:        "20251224_140000_inactive2",
			Type:      entity.SessionTypeBrowser,
			StartedAt: time.Now().Add(-2 * time.Hour),
			EndedAt:   &endedAt, // Ended - purgeable
		},
	}

	sessionRepo.EXPECT().GetRecent(ctx, 1000).Return(sessions, nil)
	stateRepo.EXPECT().GetAllSnapshots(ctx).Return(nil, nil)
	stateRepo.EXPECT().GetTotalSnapshotsSize(ctx).Return(int64(12345), nil)

	uc := usecase.NewListSessionsUseCase(sessionRepo, stateRepo, "")

	output, err := uc.GetPurgeableSessions(ctx)
	require.NoError(t, err)
	require.NotNil(t, output)

	// Should only have 2 inactive sessions, not the active one
	require.Len(t, output.Sessions, 2)
	assert.Equal(t, int64(12345), output.TotalSize)

	// Verify all returned sessions are inactive
	for _, s := range output.Sessions {
		assert.False(t, s.Info.IsActive, "session %s should not be active", s.Info.Session.ID)
		assert.True(t, s.Selected, "sessions should be selected by default")
	}
}

func TestListSessionsUseCase_GetPurgeableSessions_EmptyWhenAllActive(t *testing.T) {
	ctx := testContext()

	sessionRepo := repomocks.NewMockSessionRepository(t)
	stateRepo := repomocks.NewMockSessionStateRepository(t)

	sessions := []*entity.Session{
		{
			ID:        "20251224_130000_active1",
			Type:      entity.SessionTypeBrowser,
			StartedAt: time.Now().Add(-time.Hour),
			EndedAt:   nil, // Active
		},
	}

	sessionRepo.EXPECT().GetRecent(ctx, 1000).Return(sessions, nil)
	stateRepo.EXPECT().GetAllSnapshots(ctx).Return(nil, nil)
	stateRepo.EXPECT().GetTotalSnapshotsSize(ctx).Return(int64(0), nil)

	uc := usecase.NewListSessionsUseCase(sessionRepo, stateRepo, "")

	output, err := uc.GetPurgeableSessions(ctx)
	require.NoError(t, err)
	require.NotNil(t, output)
	require.Empty(t, output.Sessions)
}

func TestListSessionsUseCase_GetPurgeableSessions_HandlesGetSizeError(t *testing.T) {
	ctx := testContext()

	sessionRepo := repomocks.NewMockSessionRepository(t)
	stateRepo := repomocks.NewMockSessionStateRepository(t)

	endedAt := time.Now().Add(-time.Hour)
	sessions := []*entity.Session{
		{
			ID:        "20251224_120000_inactive1",
			Type:      entity.SessionTypeBrowser,
			StartedAt: time.Now().Add(-2 * time.Hour),
			EndedAt:   &endedAt,
		},
	}

	sessionRepo.EXPECT().GetRecent(ctx, 1000).Return(sessions, nil)
	stateRepo.EXPECT().GetAllSnapshots(ctx).Return(nil, nil)
	stateRepo.EXPECT().GetTotalSnapshotsSize(ctx).Return(int64(0), assert.AnError)

	uc := usecase.NewListSessionsUseCase(sessionRepo, stateRepo, "")

	output, err := uc.GetPurgeableSessions(ctx)
	// Should succeed even if size query fails
	require.NoError(t, err)
	require.NotNil(t, output)
	require.Len(t, output.Sessions, 1)
	assert.Equal(t, int64(0), output.TotalSize) // Size defaults to 0 on error
}
