package usecase_test

import (
	"errors"
	"testing"
	"time"

	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/domain/entity"
	repomocks "github.com/bnema/dumber/internal/domain/repository/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRestoreSessionUseCase_Execute_ReturnsState(t *testing.T) {
	ctx := testContext()

	stateRepo := repomocks.NewMockSessionStateRepository(t)
	sessionRepo := repomocks.NewMockSessionRepository(t)

	sessionID := entity.SessionID("20251224_120000_restore")

	expectedState := &entity.SessionState{
		Version:   entity.SessionStateVersion,
		SessionID: sessionID,
		Tabs: []entity.TabSnapshot{
			{
				ID:   "tab-1",
				Name: "Test Tab",
				Workspace: entity.WorkspaceSnapshot{
					Root: &entity.PaneNodeSnapshot{
						Pane: &entity.PaneSnapshot{
							ID:    "pane-1",
							URI:   "https://example.com",
							Title: "Example",
						},
					},
				},
			},
		},
		SavedAt: time.Now(),
	}

	stateRepo.EXPECT().GetSnapshot(ctx, sessionID).Return(expectedState, nil)

	uc := usecase.NewRestoreSessionUseCase(stateRepo, sessionRepo)

	output, err := uc.Execute(ctx, usecase.RestoreInput{SessionID: sessionID})
	require.NoError(t, err)
	require.NotNil(t, output)
	assert.Equal(t, expectedState, output.State)
}

func TestRestoreSessionUseCase_Execute_RequiresSessionID(t *testing.T) {
	ctx := testContext()

	stateRepo := repomocks.NewMockSessionStateRepository(t)
	sessionRepo := repomocks.NewMockSessionRepository(t)

	uc := usecase.NewRestoreSessionUseCase(stateRepo, sessionRepo)

	_, err := uc.Execute(ctx, usecase.RestoreInput{SessionID: ""})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "session id required")
}

func TestRestoreSessionUseCase_Execute_SessionNotFound(t *testing.T) {
	ctx := testContext()

	stateRepo := repomocks.NewMockSessionStateRepository(t)
	sessionRepo := repomocks.NewMockSessionRepository(t)

	sessionID := entity.SessionID("20251224_120000_notfound")

	stateRepo.EXPECT().GetSnapshot(ctx, sessionID).Return(nil, nil)

	uc := usecase.NewRestoreSessionUseCase(stateRepo, sessionRepo)

	_, err := uc.Execute(ctx, usecase.RestoreInput{SessionID: sessionID})
	require.Error(t, err)
	assert.ErrorIs(t, err, usecase.ErrSessionNotFound)
}

func TestRestoreSessionUseCase_Execute_VersionMismatch(t *testing.T) {
	ctx := testContext()

	stateRepo := repomocks.NewMockSessionStateRepository(t)
	sessionRepo := repomocks.NewMockSessionRepository(t)

	sessionID := entity.SessionID("20251224_120000_version")

	// Return a state with a future version
	futureState := &entity.SessionState{
		Version:   entity.SessionStateVersion + 1,
		SessionID: sessionID,
		Tabs:      []entity.TabSnapshot{},
		SavedAt:   time.Now(),
	}

	stateRepo.EXPECT().GetSnapshot(ctx, sessionID).Return(futureState, nil)

	uc := usecase.NewRestoreSessionUseCase(stateRepo, sessionRepo)

	_, err := uc.Execute(ctx, usecase.RestoreInput{SessionID: sessionID})
	require.Error(t, err)
	assert.ErrorIs(t, err, usecase.ErrVersionMismatch)
}

func TestRestoreSessionUseCase_Execute_GetSnapshotError(t *testing.T) {
	ctx := testContext()

	stateRepo := repomocks.NewMockSessionStateRepository(t)
	sessionRepo := repomocks.NewMockSessionRepository(t)

	sessionID := entity.SessionID("20251224_120000_error")

	stateRepo.EXPECT().GetSnapshot(ctx, sessionID).Return(nil, errors.New("db error"))

	uc := usecase.NewRestoreSessionUseCase(stateRepo, sessionRepo)

	_, err := uc.Execute(ctx, usecase.RestoreInput{SessionID: sessionID})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "get session snapshot")
}

func TestRestoreSessionUseCase_DeleteSnapshot(t *testing.T) {
	ctx := testContext()

	stateRepo := repomocks.NewMockSessionStateRepository(t)
	sessionRepo := repomocks.NewMockSessionRepository(t)

	sessionID := entity.SessionID("20251224_120000_delete")

	stateRepo.EXPECT().DeleteSnapshot(ctx, sessionID).Return(nil)

	uc := usecase.NewRestoreSessionUseCase(stateRepo, sessionRepo)

	err := uc.DeleteSnapshot(ctx, sessionID)
	require.NoError(t, err)
}
