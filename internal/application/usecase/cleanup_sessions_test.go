package usecase_test

import (
	"errors"
	"testing"
	"time"

	"github.com/bnema/dumber/internal/application/usecase"
	repomocks "github.com/bnema/dumber/internal/domain/repository/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

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

	uc := usecase.NewCleanupSessionsUseCase(sessionRepo)

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
	uc := usecase.NewCleanupSessionsUseCase(sessionRepo)

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

	uc := usecase.NewCleanupSessionsUseCase(sessionRepo)

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

	uc := usecase.NewCleanupSessionsUseCase(sessionRepo)

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

	uc := usecase.NewCleanupSessionsUseCase(sessionRepo)

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

	uc := usecase.NewCleanupSessionsUseCase(sessionRepo)

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

	uc := usecase.NewCleanupSessionsUseCase(sessionRepo)

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

	uc := usecase.NewCleanupSessionsUseCase(sessionRepo)

	output, err := uc.Execute(ctx, usecase.CleanupSessionsInput{
		MaxExitedSessions:       0, // Keep 0 = delete all
		MaxExitedSessionAgeDays: 0, // Disabled
	})

	require.NoError(t, err)
	assert.Equal(t, int64(0), output.DeletedByAge)
	assert.Equal(t, int64(10), output.DeletedByCount)
	assert.Equal(t, int64(10), output.TotalDeleted)
}
