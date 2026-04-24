package usecase_test

import (
	"errors"
	"testing"
	"time"

	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/domain/entity"
	repomocks "github.com/bnema/dumber/internal/domain/repository/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestSearchHistoryUseCase_GetRecentSince_ReturnsEntries(t *testing.T) {
	ctx := testContext()

	historyRepo := repomocks.NewMockHistoryRepository(t)

	entries := []*entity.HistoryEntry{
		{ID: 1, URL: "https://example.com", Title: "Example", LastVisited: time.Now()},
		{ID: 2, URL: "https://github.com", Title: "GitHub", LastVisited: time.Now().Add(-24 * time.Hour)},
	}

	historyRepo.EXPECT().GetRecentSince(mock.Anything, 30).Return(entries, nil)

	uc := usecase.NewSearchHistoryUseCase(historyRepo)

	result, err := uc.GetRecentSince(ctx, 30)
	require.NoError(t, err)
	require.Len(t, result, 2)
	assert.Equal(t, "https://example.com", result[0].URL)
	assert.Equal(t, "https://github.com", result[1].URL)
}

func TestSearchHistoryUseCase_GetRecentSince_ZeroMeansAll(t *testing.T) {
	ctx := testContext()

	historyRepo := repomocks.NewMockHistoryRepository(t)

	entries := []*entity.HistoryEntry{
		{ID: 1, URL: "https://example.com", Title: "Example"},
	}

	// When days == 0, should call GetAllRecentHistory
	historyRepo.EXPECT().GetAllRecentHistory(mock.Anything).Return(entries, nil)

	uc := usecase.NewSearchHistoryUseCase(historyRepo)

	result, err := uc.GetRecentSince(ctx, 0)
	require.NoError(t, err)
	require.Len(t, result, 1)
}

func TestSearchHistoryUseCase_GetRecentSince_NegativeDaysDefaultsTo30(t *testing.T) {
	ctx := testContext()

	historyRepo := repomocks.NewMockHistoryRepository(t)

	entries := []*entity.HistoryEntry{}

	historyRepo.EXPECT().GetRecentSince(mock.Anything, 30).Return(entries, nil)

	uc := usecase.NewSearchHistoryUseCase(historyRepo)

	result, err := uc.GetRecentSince(ctx, -5)
	require.NoError(t, err)
	require.Empty(t, result)
}

func TestSearchHistoryUseCase_GetRecentSince_ReturnsErrorOnRepoFailure(t *testing.T) {
	ctx := testContext()

	historyRepo := repomocks.NewMockHistoryRepository(t)

	historyRepo.EXPECT().GetRecentSince(mock.Anything, 7).Return(nil, errors.New("db connection failed"))

	uc := usecase.NewSearchHistoryUseCase(historyRepo)

	result, err := uc.GetRecentSince(ctx, 7)
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "failed to get recent history")
}

func TestSearchHistoryUseCase_GetMostVisited_ReturnsEntries(t *testing.T) {
	ctx := testContext()

	historyRepo := repomocks.NewMockHistoryRepository(t)

	entries := []*entity.HistoryEntry{
		{ID: 1, URL: "https://github.com", Title: "GitHub", VisitCount: 100},
		{ID: 2, URL: "https://example.com", Title: "Example", VisitCount: 50},
	}

	historyRepo.EXPECT().GetMostVisited(mock.Anything, 30).Return(entries, nil)

	uc := usecase.NewSearchHistoryUseCase(historyRepo)

	result, err := uc.GetMostVisited(ctx, 30)
	require.NoError(t, err)
	require.Len(t, result, 2)
	assert.Equal(t, "https://github.com", result[0].URL)
	assert.Equal(t, int64(100), result[0].VisitCount)
}

func TestSearchHistoryUseCase_GetMostVisited_ZeroMeansAll(t *testing.T) {
	ctx := testContext()

	historyRepo := repomocks.NewMockHistoryRepository(t)

	entries := []*entity.HistoryEntry{}

	// When days == 0, should call GetAllMostVisited
	historyRepo.EXPECT().GetAllMostVisited(mock.Anything).Return(entries, nil)

	uc := usecase.NewSearchHistoryUseCase(historyRepo)

	result, err := uc.GetMostVisited(ctx, 0)
	require.NoError(t, err)
	require.Empty(t, result)
}

func TestSearchHistoryUseCase_GetMostVisited_ReturnsErrorOnRepoFailure(t *testing.T) {
	ctx := testContext()

	historyRepo := repomocks.NewMockHistoryRepository(t)

	historyRepo.EXPECT().GetMostVisited(mock.Anything, 14).Return(nil, errors.New("query failed"))

	uc := usecase.NewSearchHistoryUseCase(historyRepo)

	result, err := uc.GetMostVisited(ctx, 14)
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "failed to get most visited history")
}

func TestSearchHistoryUseCase_GetMostVisited_ReturnsEmptyWhenNoHistory(t *testing.T) {
	ctx := testContext()

	historyRepo := repomocks.NewMockHistoryRepository(t)

	historyRepo.EXPECT().GetMostVisited(mock.Anything, 30).Return([]*entity.HistoryEntry{}, nil)

	uc := usecase.NewSearchHistoryUseCase(historyRepo)

	result, err := uc.GetMostVisited(ctx, 30)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Empty(t, result)
}

func TestSearchHistoryUseCase_ClearRange_RecentRangeDeletesSinceCutoff(t *testing.T) {
	ctx := testContext()
	historyRepo := repomocks.NewMockHistoryRepository(t)
	historyRepo.EXPECT().DeleteSince(mock.Anything, mock.MatchedBy(func(since time.Time) bool {
		return !since.IsZero() && time.Since(since) <= time.Hour+time.Minute
	})).Return(nil).Once()

	uc := usecase.NewSearchHistoryUseCase(historyRepo)

	err := uc.ClearRange(ctx, "hour")
	require.NoError(t, err)
}

func TestSearchHistoryUseCase_ClearRange_TodayDeletesSinceLocalMidnight(t *testing.T) {
	ctx := testContext()
	historyRepo := repomocks.NewMockHistoryRepository(t)
	today := time.Now()
	historyRepo.EXPECT().DeleteSince(mock.Anything, mock.MatchedBy(func(since time.Time) bool {
		return since.Year() == today.Year() &&
			since.Month() == today.Month() &&
			since.Day() == today.Day() &&
			since.Hour() == 0 &&
			since.Minute() == 0 &&
			since.Second() == 0 &&
			since.Nanosecond() == 0
	})).Return(nil).Once()

	uc := usecase.NewSearchHistoryUseCase(historyRepo)

	err := uc.ClearRange(ctx, "day")
	require.NoError(t, err)
}

func TestSearchHistoryUseCase_ClearRange_AllUsesClearAll(t *testing.T) {
	ctx := testContext()

	historyRepo := repomocks.NewMockHistoryRepository(t)

	historyRepo.EXPECT().DeleteAll(mock.Anything).Return(nil)

	uc := usecase.NewSearchHistoryUseCase(historyRepo)

	err := uc.ClearRange(ctx, "all")
	require.NoError(t, err)
}

func TestSearchHistoryUseCase_ClearRange_UnknownRangeDoesNotDelete(t *testing.T) {
	ctx := testContext()

	historyRepo := repomocks.NewMockHistoryRepository(t)

	uc := usecase.NewSearchHistoryUseCase(historyRepo)

	err := uc.ClearRange(ctx, "bogus")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown history delete range")
}

func TestSearchHistoryUseCase_GetRecentByDomainAllowsUnderscoreDomains(t *testing.T) {
	ctx := testContext()
	historyRepo := repomocks.NewMockHistoryRepository(t)
	want := []*entity.HistoryEntry{{URL: "https://example_.com"}}
	historyRepo.EXPECT().GetRecentByDomain(mock.Anything, "example_.com", 20, 0).Return(want, nil).Once()
	uc := usecase.NewSearchHistoryUseCase(historyRepo)

	result, err := uc.GetRecentByDomain(ctx, "example_.com", 20, 0)

	require.NoError(t, err)
	assert.Equal(t, want, result)
}

func TestSearchHistoryUseCase_DeleteByDomainAllowsUnderscoreDomains(t *testing.T) {
	ctx := testContext()
	historyRepo := repomocks.NewMockHistoryRepository(t)
	historyRepo.EXPECT().DeleteByDomain(mock.Anything, "example_.com").Return(nil).Once()
	uc := usecase.NewSearchHistoryUseCase(historyRepo)

	err := uc.DeleteByDomain(ctx, "example_.com")

	require.NoError(t, err)
}
