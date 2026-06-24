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

func TestSearchHistoryUseCase_GetRecent_ZeroLimitMeansAll(t *testing.T) {
	ctx := testContext()
	historyRepo := repomocks.NewMockHistoryRepository(t)
	entries := []*entity.HistoryEntry{{ID: 1, URL: "https://example.com", Title: "Example"}}
	historyRepo.EXPECT().GetRecent(mock.Anything, 0, 0).Return(entries, nil).Once()

	uc := usecase.NewSearchHistoryUseCase(historyRepo)
	result, err := uc.GetRecent(ctx, 0, 0)

	require.NoError(t, err)
	require.Len(t, result, 1)
}

func TestSearchHistoryUseCase_GetRecent_NegativeLimitDefaultsToPageSize(t *testing.T) {
	ctx := testContext()
	historyRepo := repomocks.NewMockHistoryRepository(t)
	historyRepo.EXPECT().GetRecent(mock.Anything, 50, 0).Return([]*entity.HistoryEntry{}, nil).Once()

	uc := usecase.NewSearchHistoryUseCase(historyRepo)
	result, err := uc.GetRecent(ctx, -1, 0)

	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestSearchHistoryUseCase_GetRecent_ClampsOversizedLimit(t *testing.T) {
	ctx := testContext()
	historyRepo := repomocks.NewMockHistoryRepository(t)
	historyRepo.EXPECT().GetRecent(mock.Anything, 500, 10).Return([]*entity.HistoryEntry{}, nil).Once()

	uc := usecase.NewSearchHistoryUseCase(historyRepo)
	result, err := uc.GetRecent(ctx, 10_000, 10)

	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestSearchHistoryUseCase_Search_ClampsOversizedLimit(t *testing.T) {
	ctx := testContext()
	historyRepo := repomocks.NewMockHistoryRepository(t)
	historyRepo.EXPECT().Search(mock.Anything, "dumber", 100).Return([]entity.HistoryMatch{}, nil).Once()

	uc := usecase.NewSearchHistoryUseCase(historyRepo)
	result, err := uc.Search(ctx, usecase.SearchInput{Query: "dumber", Limit: 10_000})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Empty(t, result.Matches)
}

func TestSearchHistoryUseCase_GetRecentWindow_InvalidDomainReturnsValidationError(t *testing.T) {
	ctx := testContext()
	historyRepo := repomocks.NewMockHistoryRepository(t)
	uc := usecase.NewSearchHistoryUseCase(historyRepo)

	result, err := uc.GetRecentWindow(ctx, time.Time{}, 0, "///")

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "domain is required")
}

func TestSearchHistoryUseCase_GetRecentWindow_RejectsInvalidCursorPairs(t *testing.T) {
	ctx := testContext()
	historyRepo := repomocks.NewMockHistoryRepository(t)
	uc := usecase.NewSearchHistoryUseCase(historyRepo)

	result, err := uc.GetRecentWindow(ctx, time.Time{}, 42, "")
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "before and beforeID")

	result, err = uc.GetRecentWindow(ctx, time.Now(), 0, "")
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "before and beforeID")

	result, err = uc.GetRecentWindow(ctx, time.Now(), -1, "")
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "beforeID must be non-negative")
}

func TestSearchHistoryUseCase_GetRecentWindow_UsesLimitPlusOneForHasMoreAndCursor(t *testing.T) {
	ctx := testContext()
	before := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)
	entries := make([]*entity.HistoryEntry, 101)
	for i := range entries {
		entries[i] = &entity.HistoryEntry{ID: int64(200 - i), URL: "https://example.com", LastVisited: before.Add(-time.Duration(i) * time.Minute)}
	}
	historyRepo := repomocks.NewMockHistoryRepository(t)
	historyRepo.EXPECT().GetRecentWindow(mock.Anything, before, int64(999), 101).Return(entries, nil).Once()
	uc := usecase.NewSearchHistoryUseCase(historyRepo)

	result, err := uc.GetRecentWindow(ctx, before, 999, "")

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Len(t, result.Entries, 100)
	assert.True(t, result.HasMore)
	assert.Equal(t, entries[99].LastVisited.UTC(), result.CursorLastVisited)
	assert.Equal(t, entries[99].ID, result.CursorID)
	assert.Equal(t, before.Add(-24*time.Hour), result.After)
}

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

func TestSearchHistoryUseCase_GetDomainStats_ClampsOversizedLimit(t *testing.T) {
	ctx := testContext()
	historyRepo := repomocks.NewMockHistoryRepository(t)
	historyRepo.EXPECT().GetDomainStats(mock.Anything, 100).Return([]*entity.DomainStat{}, nil).Once()

	uc := usecase.NewSearchHistoryUseCase(historyRepo)
	result, err := uc.GetDomainStats(ctx, 10_000)

	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestSearchHistoryUseCase_ClearRange_RecentRangeDeletesSinceCutoff(t *testing.T) {
	ctx := testContext()
	historyRepo := repomocks.NewMockHistoryRepository(t)
	now := time.Now()
	expectedCutoff := now.Add(-time.Hour)
	historyRepo.EXPECT().DeleteSince(mock.Anything, mock.MatchedBy(func(since time.Time) bool {
		return since.After(expectedCutoff.Add(-time.Minute)) && since.Before(expectedCutoff.Add(time.Minute))
	})).Return(nil).Once()

	uc := usecase.NewSearchHistoryUseCase(historyRepo)

	err := uc.ClearRange(ctx, "hour")
	require.NoError(t, err)
}

func TestSearchHistoryUseCase_ClearRange_TodayDeletesSinceLocalMidnight(t *testing.T) {
	ctx := testContext()
	historyRepo := repomocks.NewMockHistoryRepository(t)
	now := time.Now()
	expectedCutoff := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	historyRepo.EXPECT().DeleteSince(mock.Anything, mock.MatchedBy(func(since time.Time) bool {
		return since.Equal(expectedCutoff)
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

func TestSearchHistoryUseCase_GetRecentByDomain_ClampsOversizedLimit(t *testing.T) {
	ctx := testContext()
	historyRepo := repomocks.NewMockHistoryRepository(t)
	historyRepo.EXPECT().GetRecentByDomain(mock.Anything, "example.com", 500, 25).Return([]*entity.HistoryEntry{}, nil).Once()
	uc := usecase.NewSearchHistoryUseCase(historyRepo)

	result, err := uc.GetRecentByDomain(ctx, "example.com", 10_000, 25)

	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestSearchHistoryUseCase_DeleteByDomainAllowsUnderscoreDomains(t *testing.T) {
	ctx := testContext()
	historyRepo := repomocks.NewMockHistoryRepository(t)
	historyRepo.EXPECT().DeleteByDomain(mock.Anything, "example_.com").Return(nil).Once()
	uc := usecase.NewSearchHistoryUseCase(historyRepo)

	err := uc.DeleteByDomain(ctx, "example_.com")

	require.NoError(t, err)
}
