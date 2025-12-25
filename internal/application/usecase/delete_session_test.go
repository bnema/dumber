package usecase_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/domain/entity"
	repomocks "github.com/bnema/dumber/internal/domain/repository/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeleteSessionUseCase_Execute_Success(t *testing.T) {
	ctx := testContext()

	stateRepo := repomocks.NewMockSessionStateRepository(t)
	sessionRepo := repomocks.NewMockSessionRepository(t)

	sessionID := entity.SessionID("20251224_120000_delete")

	endedAt := time.Now().Add(-time.Hour)
	session := &entity.Session{
		ID:        sessionID,
		Type:      entity.SessionTypeBrowser,
		StartedAt: time.Now().Add(-2 * time.Hour),
		EndedAt:   &endedAt,
	}

	sessionRepo.EXPECT().FindByID(ctx, sessionID).Return(session, nil)
	stateRepo.EXPECT().DeleteSnapshot(ctx, sessionID).Return(nil)
	sessionRepo.EXPECT().Delete(ctx, sessionID).Return(nil)

	uc := usecase.NewDeleteSessionUseCase(stateRepo, sessionRepo, "")

	err := uc.Execute(ctx, usecase.DeleteSessionInput{
		SessionID:        sessionID,
		CurrentSessionID: "other-session",
	})
	require.NoError(t, err)
}

func TestDeleteSessionUseCase_Execute_CannotDeleteCurrentSession(t *testing.T) {
	ctx := testContext()

	stateRepo := repomocks.NewMockSessionStateRepository(t)
	sessionRepo := repomocks.NewMockSessionRepository(t)

	sessionID := entity.SessionID("20251224_120000_current")

	uc := usecase.NewDeleteSessionUseCase(stateRepo, sessionRepo, "")

	err := uc.Execute(ctx, usecase.DeleteSessionInput{
		SessionID:        sessionID,
		CurrentSessionID: sessionID, // Same as current
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, usecase.ErrCannotDeleteCurrentSession)
}

func TestDeleteSessionUseCase_Execute_SessionNotFound(t *testing.T) {
	ctx := testContext()

	stateRepo := repomocks.NewMockSessionStateRepository(t)
	sessionRepo := repomocks.NewMockSessionRepository(t)

	sessionID := entity.SessionID("20251224_120000_notfound")

	sessionRepo.EXPECT().FindByID(ctx, sessionID).Return(nil, nil)

	uc := usecase.NewDeleteSessionUseCase(stateRepo, sessionRepo, "")

	err := uc.Execute(ctx, usecase.DeleteSessionInput{
		SessionID:        sessionID,
		CurrentSessionID: "other-session",
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, usecase.ErrSessionNotFound)
}

func TestDeleteSessionUseCase_Execute_CannotDeleteActiveSession(t *testing.T) {
	ctx := testContext()

	stateRepo := repomocks.NewMockSessionStateRepository(t)
	sessionRepo := repomocks.NewMockSessionRepository(t)

	sessionID := entity.SessionID("20251224_120000_active")

	// Session with nil EndedAt (still active)
	session := &entity.Session{
		ID:        sessionID,
		Type:      entity.SessionTypeBrowser,
		StartedAt: time.Now().Add(-time.Hour),
		EndedAt:   nil, // Active
	}

	sessionRepo.EXPECT().FindByID(ctx, sessionID).Return(session, nil)

	uc := usecase.NewDeleteSessionUseCase(stateRepo, sessionRepo, "")

	err := uc.Execute(ctx, usecase.DeleteSessionInput{
		SessionID:        sessionID,
		CurrentSessionID: "other-session",
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, usecase.ErrCannotDeleteActiveSession)
}

func TestDeleteSessionUseCase_Execute_CannotDeleteLockedSession(t *testing.T) {
	ctx := testContext()

	stateRepo := repomocks.NewMockSessionStateRepository(t)
	sessionRepo := repomocks.NewMockSessionRepository(t)

	// Create a temporary lock directory and lock file
	lockDir := t.TempDir()
	sessionID := entity.SessionID("20251224_120000_locked")
	lockPath := filepath.Join(lockDir, "session_"+string(sessionID)+".lock")
	f, err := os.Create(lockPath)
	require.NoError(t, err)
	f.Close()

	endedAt := time.Now().Add(-time.Hour)
	session := &entity.Session{
		ID:        sessionID,
		Type:      entity.SessionTypeBrowser,
		StartedAt: time.Now().Add(-2 * time.Hour),
		EndedAt:   &endedAt, // Ended, but has lock file
	}

	sessionRepo.EXPECT().FindByID(ctx, sessionID).Return(session, nil)

	uc := usecase.NewDeleteSessionUseCase(stateRepo, sessionRepo, lockDir)

	err = uc.Execute(ctx, usecase.DeleteSessionInput{
		SessionID:        sessionID,
		CurrentSessionID: "other-session",
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, usecase.ErrCannotDeleteActiveSession)
}

func TestDeleteSessionUseCase_Execute_FindByIDError(t *testing.T) {
	ctx := testContext()

	stateRepo := repomocks.NewMockSessionStateRepository(t)
	sessionRepo := repomocks.NewMockSessionRepository(t)

	sessionID := entity.SessionID("20251224_120000_dberror")

	sessionRepo.EXPECT().FindByID(ctx, sessionID).Return(nil, errors.New("db error"))

	uc := usecase.NewDeleteSessionUseCase(stateRepo, sessionRepo, "")

	err := uc.Execute(ctx, usecase.DeleteSessionInput{
		SessionID:        sessionID,
		CurrentSessionID: "other-session",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "db error")
}

func TestDeleteSessionUseCase_Execute_DeleteSnapshotErrorIgnored(t *testing.T) {
	ctx := testContext()

	stateRepo := repomocks.NewMockSessionStateRepository(t)
	sessionRepo := repomocks.NewMockSessionRepository(t)

	sessionID := entity.SessionID("20251224_120000_nosnapshot")

	endedAt := time.Now().Add(-time.Hour)
	session := &entity.Session{
		ID:        sessionID,
		Type:      entity.SessionTypeBrowser,
		StartedAt: time.Now().Add(-2 * time.Hour),
		EndedAt:   &endedAt,
	}

	sessionRepo.EXPECT().FindByID(ctx, sessionID).Return(session, nil)
	// Snapshot deletion fails (e.g., no snapshot exists) - should be ignored
	stateRepo.EXPECT().DeleteSnapshot(ctx, sessionID).Return(errors.New("no rows"))
	sessionRepo.EXPECT().Delete(ctx, sessionID).Return(nil)

	uc := usecase.NewDeleteSessionUseCase(stateRepo, sessionRepo, "")

	err := uc.Execute(ctx, usecase.DeleteSessionInput{
		SessionID:        sessionID,
		CurrentSessionID: "other-session",
	})
	// Should succeed even though DeleteSnapshot failed
	require.NoError(t, err)
}

func TestDeleteSessionUseCase_Execute_DeleteSessionError(t *testing.T) {
	ctx := testContext()

	stateRepo := repomocks.NewMockSessionStateRepository(t)
	sessionRepo := repomocks.NewMockSessionRepository(t)

	sessionID := entity.SessionID("20251224_120000_deleteerror")

	endedAt := time.Now().Add(-time.Hour)
	session := &entity.Session{
		ID:        sessionID,
		Type:      entity.SessionTypeBrowser,
		StartedAt: time.Now().Add(-2 * time.Hour),
		EndedAt:   &endedAt,
	}

	sessionRepo.EXPECT().FindByID(ctx, sessionID).Return(session, nil)
	stateRepo.EXPECT().DeleteSnapshot(ctx, sessionID).Return(nil)
	sessionRepo.EXPECT().Delete(ctx, sessionID).Return(errors.New("delete failed"))

	uc := usecase.NewDeleteSessionUseCase(stateRepo, sessionRepo, "")

	err := uc.Execute(ctx, usecase.DeleteSessionInput{
		SessionID:        sessionID,
		CurrentSessionID: "other-session",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "delete failed")
}

func TestDeleteSessionUseCase_Execute_EmptyCurrentSessionAllowed(t *testing.T) {
	ctx := testContext()

	stateRepo := repomocks.NewMockSessionStateRepository(t)
	sessionRepo := repomocks.NewMockSessionRepository(t)

	sessionID := entity.SessionID("20251224_120000_nocurrent")

	endedAt := time.Now().Add(-time.Hour)
	session := &entity.Session{
		ID:        sessionID,
		Type:      entity.SessionTypeBrowser,
		StartedAt: time.Now().Add(-2 * time.Hour),
		EndedAt:   &endedAt,
	}

	sessionRepo.EXPECT().FindByID(ctx, sessionID).Return(session, nil)
	stateRepo.EXPECT().DeleteSnapshot(ctx, sessionID).Return(nil)
	sessionRepo.EXPECT().Delete(ctx, sessionID).Return(nil)

	uc := usecase.NewDeleteSessionUseCase(stateRepo, sessionRepo, "")

	// CurrentSessionID is empty (e.g., CLI context)
	err := uc.Execute(ctx, usecase.DeleteSessionInput{
		SessionID:        sessionID,
		CurrentSessionID: "",
	})
	require.NoError(t, err)
}
