package usecase_test

import (
	"errors"
	"testing"
	"time"

	portmocks "github.com/bnema/dumber/internal/application/port/mocks"
	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/domain/entity"
	repomocks "github.com/bnema/dumber/internal/domain/repository/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestCleanupSessionsUseCase_ReconcileActiveBrowserSessions_EndsOnlyProvenDeadSessions(t *testing.T) {
	ctx := testContext()

	sessionRepo := repomocks.NewMockSessionRepository(t)
	processProbe := portmocks.NewMockSessionProcessProbe(t)

	now := time.Date(2026, 6, 18, 20, 0, 0, 0, time.UTC)
	livePID := 111
	deadPID := 222
	current := &entity.Session{ID: "current", Type: entity.SessionTypeBrowser, StartedAt: now}
	live := &entity.Session{ID: "live", Type: entity.SessionTypeBrowser, StartedAt: now.Add(-time.Minute), ProcessID: &livePID}
	dead := &entity.Session{ID: "dead", Type: entity.SessionTypeBrowser, StartedAt: now.Add(-2 * time.Minute), ProcessID: &deadPID}
	unknown := &entity.Session{ID: "unknown", Type: entity.SessionTypeBrowser, StartedAt: now.Add(-3 * time.Minute)}

	sessionRepo.EXPECT().
		GetRecent(ctx, 25).
		Return([]*entity.Session{current, live, dead, unknown}, nil)
	processProbe.EXPECT().
		IsProcessAlive(ctx, livePID).
		Return(true, nil)
	processProbe.EXPECT().
		IsProcessAlive(ctx, deadPID).
		Return(false, nil)
	sessionRepo.EXPECT().
		MarkEnded(ctx, dead.ID, now).
		Return(nil)

	uc := usecase.NewCleanupSessionsUseCase(sessionRepo, processProbe)

	output, err := uc.ReconcileActiveBrowserSessions(ctx, usecase.ReconcileActiveBrowserSessionsInput{
		CurrentSessionID: current.ID,
		RecentLimit:      25,
		EndedAt:          now,
	})

	require.NoError(t, err)
	assert.Equal(t, int64(1), output.EndedDeadSessions)
}

func TestCleanupSessionsUseCase_ReconcileActiveBrowserSessions_SkipsWhenProbeErrors(t *testing.T) {
	ctx := testContext()

	sessionRepo := repomocks.NewMockSessionRepository(t)
	processProbe := portmocks.NewMockSessionProcessProbe(t)

	now := time.Date(2026, 6, 18, 20, 30, 0, 0, time.UTC)
	pid := 333
	active := &entity.Session{ID: "active", Type: entity.SessionTypeBrowser, StartedAt: now.Add(-time.Minute), ProcessID: &pid}

	sessionRepo.EXPECT().
		GetRecent(ctx, 20).
		Return([]*entity.Session{active}, nil)
	processProbe.EXPECT().
		IsProcessAlive(ctx, pid).
		Return(false, errors.New("probe failed"))

	uc := usecase.NewCleanupSessionsUseCase(sessionRepo, processProbe)

	output, err := uc.ReconcileActiveBrowserSessions(ctx, usecase.ReconcileActiveBrowserSessionsInput{
		RecentLimit: 20,
		EndedAt:     now,
	})

	require.NoError(t, err)
	assert.Equal(t, int64(0), output.EndedDeadSessions)
}

func TestCleanupSessionsUseCase_Execute_AgeAndCountCleanup(t *testing.T) {
	ctx := testContext()

	sessionRepo := repomocks.NewMockSessionRepository(t)

	// Expect age-based deletion
	sessionRepo.EXPECT().
		DeleteExitedBefore(ctx, mock.MatchedBy(func(cutoff time.Time) bool {
			// Should be approximately 7 days ago
			expected := time.Now().AddDate(0, 0, -7)
			diff := expected.Sub(cutoff)
			return diff > -time.Minute && diff < time.Minute
		})).
		Return(int64(3), nil)

	// Expect count-based deletion
	sessionRepo.EXPECT().
		DeleteOldestExited(ctx, 50).
		Return(int64(2), nil)

	uc := usecase.NewCleanupSessionsUseCase(sessionRepo, nil)

	output, err := uc.Execute(ctx, usecase.CleanupSessionsInput{
		MaxExitedSessions:       50,
		MaxExitedSessionAgeDays: 7,
	})

	require.NoError(t, err)
	assert.Equal(t, int64(3), output.DeletedByAge)
	assert.Equal(t, int64(2), output.DeletedByCount)
	assert.Equal(t, int64(5), output.TotalDeleted)
}

func TestCleanupSessionsUseCase_Execute_AgeOnlyCleanup(t *testing.T) {
	ctx := testContext()

	sessionRepo := repomocks.NewMockSessionRepository(t)

	// Expect age-based deletion
	sessionRepo.EXPECT().
		DeleteExitedBefore(ctx, mock.Anything).
		Return(int64(5), nil)

	// Count-based with -1 should be skipped (disabled)
	// MaxExitedSessions >= 0 triggers count cleanup, so use -1 to disable
	uc := usecase.NewCleanupSessionsUseCase(sessionRepo, nil)

	output, err := uc.Execute(ctx, usecase.CleanupSessionsInput{
		MaxExitedSessions:       -1, // Disabled
		MaxExitedSessionAgeDays: 30,
	})

	require.NoError(t, err)
	assert.Equal(t, int64(5), output.DeletedByAge)
	assert.Equal(t, int64(0), output.DeletedByCount)
	assert.Equal(t, int64(5), output.TotalDeleted)
}

func TestCleanupSessionsUseCase_Execute_CountOnlyCleanup(t *testing.T) {
	ctx := testContext()

	sessionRepo := repomocks.NewMockSessionRepository(t)

	// Count-based deletion only (age disabled with 0)
	sessionRepo.EXPECT().
		DeleteOldestExited(ctx, 10).
		Return(int64(4), nil)

	uc := usecase.NewCleanupSessionsUseCase(sessionRepo, nil)

	output, err := uc.Execute(ctx, usecase.CleanupSessionsInput{
		MaxExitedSessions:       10,
		MaxExitedSessionAgeDays: 0, // Disabled
	})

	require.NoError(t, err)
	assert.Equal(t, int64(0), output.DeletedByAge)
	assert.Equal(t, int64(4), output.DeletedByCount)
	assert.Equal(t, int64(4), output.TotalDeleted)
}

func TestCleanupSessionsUseCase_Execute_NoCleanupNeeded(t *testing.T) {
	ctx := testContext()

	sessionRepo := repomocks.NewMockSessionRepository(t)

	// Nothing to delete
	sessionRepo.EXPECT().
		DeleteExitedBefore(ctx, mock.Anything).
		Return(int64(0), nil)

	sessionRepo.EXPECT().
		DeleteOldestExited(ctx, 50).
		Return(int64(0), nil)

	uc := usecase.NewCleanupSessionsUseCase(sessionRepo, nil)

	output, err := uc.Execute(ctx, usecase.CleanupSessionsInput{
		MaxExitedSessions:       50,
		MaxExitedSessionAgeDays: 7,
	})

	require.NoError(t, err)
	assert.Equal(t, int64(0), output.DeletedByAge)
	assert.Equal(t, int64(0), output.DeletedByCount)
	assert.Equal(t, int64(0), output.TotalDeleted)
}

func TestCleanupSessionsUseCase_Execute_AgeDeleteError_ContinuesToCount(t *testing.T) {
	ctx := testContext()

	sessionRepo := repomocks.NewMockSessionRepository(t)

	// Age-based deletion fails
	sessionRepo.EXPECT().
		DeleteExitedBefore(ctx, mock.Anything).
		Return(int64(0), errors.New("db error"))

	// Should still attempt count-based deletion
	sessionRepo.EXPECT().
		DeleteOldestExited(ctx, 50).
		Return(int64(3), nil)

	uc := usecase.NewCleanupSessionsUseCase(sessionRepo, nil)

	output, err := uc.Execute(ctx, usecase.CleanupSessionsInput{
		MaxExitedSessions:       50,
		MaxExitedSessionAgeDays: 7,
	})

	// Should not return error, just log warning
	require.NoError(t, err)
	assert.Equal(t, int64(0), output.DeletedByAge)
	assert.Equal(t, int64(3), output.DeletedByCount)
	assert.Equal(t, int64(3), output.TotalDeleted)
}

func TestCleanupSessionsUseCase_Execute_CountDeleteError(t *testing.T) {
	ctx := testContext()

	sessionRepo := repomocks.NewMockSessionRepository(t)

	// Age-based deletion succeeds
	sessionRepo.EXPECT().
		DeleteExitedBefore(ctx, mock.Anything).
		Return(int64(2), nil)

	// Count-based deletion fails
	sessionRepo.EXPECT().
		DeleteOldestExited(ctx, 50).
		Return(int64(0), errors.New("db error"))

	uc := usecase.NewCleanupSessionsUseCase(sessionRepo, nil)

	output, err := uc.Execute(ctx, usecase.CleanupSessionsInput{
		MaxExitedSessions:       50,
		MaxExitedSessionAgeDays: 7,
	})

	// Should not return error, just log warning
	require.NoError(t, err)
	assert.Equal(t, int64(2), output.DeletedByAge)
	assert.Equal(t, int64(0), output.DeletedByCount)
	assert.Equal(t, int64(2), output.TotalDeleted)
}

func TestCleanupSessionsUseCase_Execute_BothDisabled(t *testing.T) {
	ctx := testContext()

	sessionRepo := repomocks.NewMockSessionRepository(t)

	// No expectations - nothing should be called
	// Actually, count with 0 means "keep 0" which will trigger cleanup
	// So we need -1 for count and 0 for age to fully disable

	uc := usecase.NewCleanupSessionsUseCase(sessionRepo, nil)

	output, err := uc.Execute(ctx, usecase.CleanupSessionsInput{
		MaxExitedSessions:       -1, // Disabled
		MaxExitedSessionAgeDays: 0,  // Disabled
	})

	require.NoError(t, err)
	assert.Equal(t, int64(0), output.DeletedByAge)
	assert.Equal(t, int64(0), output.DeletedByCount)
	assert.Equal(t, int64(0), output.TotalDeleted)
}

func TestCleanupSessionsUseCase_Execute_ZeroMaxSessions(t *testing.T) {
	ctx := testContext()

	sessionRepo := repomocks.NewMockSessionRepository(t)

	// MaxExitedSessions = 0 means keep 0 sessions (delete all)
	sessionRepo.EXPECT().
		DeleteOldestExited(ctx, 0).
		Return(int64(10), nil)

	uc := usecase.NewCleanupSessionsUseCase(sessionRepo, nil)

	output, err := uc.Execute(ctx, usecase.CleanupSessionsInput{
		MaxExitedSessions:       0, // Keep 0 = delete all
		MaxExitedSessionAgeDays: 0, // Disabled
	})

	require.NoError(t, err)
	assert.Equal(t, int64(0), output.DeletedByAge)
	assert.Equal(t, int64(10), output.DeletedByCount)
	assert.Equal(t, int64(10), output.TotalDeleted)
}
