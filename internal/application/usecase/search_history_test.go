package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/bnema/dumber/internal/application/dto"
	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/domain/entity"
	repomocks "github.com/bnema/dumber/internal/domain/repository/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type recordingHistoryChangeSink struct {
	changes  []dto.HistoryChange
	onChange func(dto.HistoryChange)
}

func (s *recordingHistoryChangeSink) OnHistoryChanged(_ context.Context, change dto.HistoryChange) {
	if s.onChange != nil {
		s.onChange(change)
	}
	s.changes = append(s.changes, change)
}

type recordingHistoryMutationCoordinator struct {
	beginCount   int
	releaseCount int
	err          error
}

func (c *recordingHistoryMutationCoordinator) BeginHistoryMutation(context.Context) (func(), error) {
	c.beginCount++
	if c.err != nil {
		return nil, c.err
	}
	return func() { c.releaseCount++ }, nil
}

type nilPanicHistoryMutationCoordinator struct {
	called bool
}

func (c *nilPanicHistoryMutationCoordinator) BeginHistoryMutation(context.Context) (func(), error) {
	c.called = true
	return func() {}, nil
}

func requirePublishedHistoryChange(t *testing.T, sink *recordingHistoryChangeSink, reason dto.HistoryChangeReason) dto.HistoryChange {
	t.Helper()
	require.Len(t, sink.changes, 1)
	change := sink.changes[0]
	assert.Equal(t, []dto.HistoryChangeReason{reason}, change.Reasons)
	assert.False(t, change.ChangedAt.IsZero())
	return change
}

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

func TestSearchHistoryUseCase_Delete_PublishesDeleteChangeAfterSuccess(t *testing.T) {
	ctx := testContext()
	historyRepo := repomocks.NewMockHistoryRepository(t)
	historyRepo.EXPECT().Delete(mock.Anything, int64(42)).Return(nil).Once()
	sink := &recordingHistoryChangeSink{}
	uc := usecase.NewSearchHistoryUseCase(historyRepo, sink)

	err := uc.Delete(ctx, 42)

	require.NoError(t, err)
	change := requirePublishedHistoryChange(t, sink, dto.HistoryChangeReasonDelete)
	assert.Equal(t, 1, change.DeleteCount)
	assert.True(t, change.DeleteCountKnown)
}

func TestSearchHistoryUseCase_Delete_DoesNotPublishOnRepositoryError(t *testing.T) {
	ctx := testContext()
	historyRepo := repomocks.NewMockHistoryRepository(t)
	historyRepo.EXPECT().Delete(mock.Anything, int64(42)).Return(errors.New("delete failed")).Once()
	sink := &recordingHistoryChangeSink{}
	uc := usecase.NewSearchHistoryUseCase(historyRepo, sink)

	err := uc.Delete(ctx, 42)

	require.Error(t, err)
	assert.Empty(t, sink.changes)
}

func TestSearchHistoryUseCase_DeleteByDomain_PublishesDeleteChangeAfterSuccess(t *testing.T) {
	ctx := testContext()
	historyRepo := repomocks.NewMockHistoryRepository(t)
	historyRepo.EXPECT().DeleteByDomain(mock.Anything, "example.com").Return(nil).Once()
	sink := &recordingHistoryChangeSink{}
	uc := usecase.NewSearchHistoryUseCase(historyRepo, sink)

	err := uc.DeleteByDomain(ctx, "https://example.com/path")

	require.NoError(t, err)
	change := requirePublishedHistoryChange(t, sink, dto.HistoryChangeReasonDelete)
	assert.Zero(t, change.DeleteCount, "domain delete count is unknown for aggregate deletes")
	assert.False(t, change.DeleteCountKnown)
}

func TestSearchHistoryUseCase_DeleteByDomain_DoesNotPublishOnRepositoryError(t *testing.T) {
	ctx := testContext()
	historyRepo := repomocks.NewMockHistoryRepository(t)
	historyRepo.EXPECT().DeleteByDomain(mock.Anything, "example.com").Return(errors.New("delete domain failed")).Once()
	sink := &recordingHistoryChangeSink{}
	uc := usecase.NewSearchHistoryUseCase(historyRepo, sink)

	err := uc.DeleteByDomain(ctx, "example.com")

	require.Error(t, err)
	assert.Empty(t, sink.changes)
}

func TestSearchHistoryUseCase_SetHistoryMutationCoordinator_TypedNilClearsCoordinator(t *testing.T) {
	ctx := testContext()
	historyRepo := repomocks.NewMockHistoryRepository(t)
	historyRepo.EXPECT().DeleteAll(mock.Anything).Return(nil).Once()
	previous := &recordingHistoryMutationCoordinator{}
	var coordinator *nilPanicHistoryMutationCoordinator
	sink := &recordingHistoryChangeSink{}
	uc := usecase.NewSearchHistoryUseCase(historyRepo, sink)
	uc.SetHistoryMutationCoordinator(previous)
	uc.SetHistoryMutationCoordinator(coordinator)

	require.NotPanics(t, func() {
		require.NoError(t, uc.ClearAll(ctx))
	})
	require.Zero(t, previous.beginCount)
	requirePublishedHistoryChange(t, sink, dto.HistoryChangeReasonClear)
}

func TestSearchHistoryUseCase_ClearAll_PublishesClearChangeAfterSuccess(t *testing.T) {
	ctx := testContext()
	historyRepo := repomocks.NewMockHistoryRepository(t)
	historyRepo.EXPECT().DeleteAll(mock.Anything).Return(nil).Once()
	sink := &recordingHistoryChangeSink{}
	uc := usecase.NewSearchHistoryUseCase(historyRepo, sink)

	err := uc.ClearAll(ctx)

	require.NoError(t, err)
	change := requirePublishedHistoryChange(t, sink, dto.HistoryChangeReasonClear)
	assert.Zero(t, change.DeleteCount)
	assert.False(t, change.DeleteCountKnown)
}

func TestSearchHistoryUseCase_ClearAll_CoordinatesPendingRecorderWritesBeforeDelete(t *testing.T) {
	ctx := testContext()
	historyRepo := repomocks.NewMockHistoryRepository(t)
	coordinator := &recordingHistoryMutationCoordinator{}
	historyRepo.EXPECT().DeleteAll(mock.Anything).RunAndReturn(func(context.Context) error {
		require.Equal(t, 1, coordinator.beginCount)
		require.Zero(t, coordinator.releaseCount)
		return nil
	}).Once()
	sink := &recordingHistoryChangeSink{onChange: func(dto.HistoryChange) {
		require.Equal(t, 1, coordinator.releaseCount, "history change should publish after mutation coordinator release")
	}}
	uc := usecase.NewSearchHistoryUseCase(historyRepo, sink)
	uc.SetHistoryMutationCoordinator(coordinator)

	err := uc.ClearAll(ctx)

	require.NoError(t, err)
	require.Equal(t, 1, coordinator.beginCount)
	require.Equal(t, 1, coordinator.releaseCount)
	requirePublishedHistoryChange(t, sink, dto.HistoryChangeReasonClear)
}

func TestSearchHistoryUseCase_OtherDestructiveMutationsCoordinateBeforeDeleteAndPublish(t *testing.T) {
	ctx := testContext()
	before := time.Now().Add(-7 * 24 * time.Hour)

	tests := []struct {
		name             string
		expectRepo       func(*repomocks.MockHistoryRepository, *recordingHistoryMutationCoordinator)
		run              func(*usecase.SearchHistoryUseCase) error
		reason           dto.HistoryChangeReason
		deleteCount      int
		deleteCountKnown bool
	}{
		{
			name: "delete",
			expectRepo: func(repo *repomocks.MockHistoryRepository, coordinator *recordingHistoryMutationCoordinator) {
				repo.EXPECT().Delete(mock.Anything, int64(42)).RunAndReturn(func(context.Context, int64) error {
					require.Equal(t, 1, coordinator.beginCount)
					require.Zero(t, coordinator.releaseCount)
					return nil
				}).Once()
			},
			run: func(uc *usecase.SearchHistoryUseCase) error {
				return uc.Delete(ctx, 42)
			},
			reason:           dto.HistoryChangeReasonDelete,
			deleteCount:      1,
			deleteCountKnown: true,
		},
		{
			name: "delete by domain",
			expectRepo: func(repo *repomocks.MockHistoryRepository, coordinator *recordingHistoryMutationCoordinator) {
				repo.EXPECT().DeleteByDomain(mock.Anything, "example.com").RunAndReturn(func(context.Context, string) error {
					require.Equal(t, 1, coordinator.beginCount)
					require.Zero(t, coordinator.releaseCount)
					return nil
				}).Once()
			},
			run: func(uc *usecase.SearchHistoryUseCase) error {
				return uc.DeleteByDomain(ctx, "https://example.com/path")
			},
			reason: dto.HistoryChangeReasonDelete,
		},
		{
			name: "clear older than",
			expectRepo: func(repo *repomocks.MockHistoryRepository, coordinator *recordingHistoryMutationCoordinator) {
				repo.EXPECT().DeleteOlderThan(mock.Anything, before).RunAndReturn(func(context.Context, time.Time) error {
					require.Equal(t, 1, coordinator.beginCount)
					require.Zero(t, coordinator.releaseCount)
					return nil
				}).Once()
			},
			run: func(uc *usecase.SearchHistoryUseCase) error {
				return uc.ClearOlderThan(ctx, before)
			},
			reason: dto.HistoryChangeReasonClear,
		},
		{
			name: "clear range",
			expectRepo: func(repo *repomocks.MockHistoryRepository, coordinator *recordingHistoryMutationCoordinator) {
				repo.EXPECT().DeleteSince(mock.Anything, mock.AnythingOfType("time.Time")).RunAndReturn(func(context.Context, time.Time) error {
					require.Equal(t, 1, coordinator.beginCount)
					require.Zero(t, coordinator.releaseCount)
					return nil
				}).Once()
			},
			run: func(uc *usecase.SearchHistoryUseCase) error {
				return uc.ClearRange(ctx, "hour")
			},
			reason: dto.HistoryChangeReasonClear,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			historyRepo := repomocks.NewMockHistoryRepository(t)
			coordinator := &recordingHistoryMutationCoordinator{}
			tt.expectRepo(historyRepo, coordinator)
			sink := &recordingHistoryChangeSink{onChange: func(dto.HistoryChange) {
				require.Equal(t, 1, coordinator.releaseCount, "history change should publish after mutation coordinator release")
			}}
			uc := usecase.NewSearchHistoryUseCase(historyRepo, sink)
			uc.SetHistoryMutationCoordinator(coordinator)

			err := tt.run(uc)

			require.NoError(t, err)
			require.Equal(t, 1, coordinator.beginCount)
			require.Equal(t, 1, coordinator.releaseCount)
			change := requirePublishedHistoryChange(t, sink, tt.reason)
			require.Equal(t, tt.deleteCount, change.DeleteCount)
			require.Equal(t, tt.deleteCountKnown, change.DeleteCountKnown)
		})
	}
}

func TestSearchHistoryUseCase_DestructiveMutationsReleaseCoordinatorOnRepositoryError(t *testing.T) {
	ctx := testContext()
	before := time.Now().Add(-7 * 24 * time.Hour)
	repoErr := errors.New("repository failed")

	tests := []struct {
		name       string
		expectRepo func(*repomocks.MockHistoryRepository, *recordingHistoryMutationCoordinator)
		run        func(*usecase.SearchHistoryUseCase) error
	}{
		{
			name: "delete",
			expectRepo: func(repo *repomocks.MockHistoryRepository, coordinator *recordingHistoryMutationCoordinator) {
				repo.EXPECT().Delete(mock.Anything, int64(42)).RunAndReturn(func(context.Context, int64) error {
					require.Equal(t, 1, coordinator.beginCount)
					require.Zero(t, coordinator.releaseCount)
					return repoErr
				}).Once()
			},
			run: func(uc *usecase.SearchHistoryUseCase) error { return uc.Delete(ctx, 42) },
		},
		{
			name: "delete by domain",
			expectRepo: func(repo *repomocks.MockHistoryRepository, coordinator *recordingHistoryMutationCoordinator) {
				repo.EXPECT().DeleteByDomain(mock.Anything, "example.com").RunAndReturn(func(context.Context, string) error {
					require.Equal(t, 1, coordinator.beginCount)
					require.Zero(t, coordinator.releaseCount)
					return repoErr
				}).Once()
			},
			run: func(uc *usecase.SearchHistoryUseCase) error { return uc.DeleteByDomain(ctx, "example.com") },
		},
		{
			name: "clear older than",
			expectRepo: func(repo *repomocks.MockHistoryRepository, coordinator *recordingHistoryMutationCoordinator) {
				repo.EXPECT().DeleteOlderThan(mock.Anything, before).RunAndReturn(func(context.Context, time.Time) error {
					require.Equal(t, 1, coordinator.beginCount)
					require.Zero(t, coordinator.releaseCount)
					return repoErr
				}).Once()
			},
			run: func(uc *usecase.SearchHistoryUseCase) error { return uc.ClearOlderThan(ctx, before) },
		},
		{
			name: "clear range",
			expectRepo: func(repo *repomocks.MockHistoryRepository, coordinator *recordingHistoryMutationCoordinator) {
				repo.EXPECT().DeleteSince(mock.Anything, mock.AnythingOfType("time.Time")).RunAndReturn(func(context.Context, time.Time) error {
					require.Equal(t, 1, coordinator.beginCount)
					require.Zero(t, coordinator.releaseCount)
					return repoErr
				}).Once()
			},
			run: func(uc *usecase.SearchHistoryUseCase) error { return uc.ClearRange(ctx, "hour") },
		},
		{
			name: "clear all",
			expectRepo: func(repo *repomocks.MockHistoryRepository, coordinator *recordingHistoryMutationCoordinator) {
				repo.EXPECT().DeleteAll(mock.Anything).RunAndReturn(func(context.Context) error {
					require.Equal(t, 1, coordinator.beginCount)
					require.Zero(t, coordinator.releaseCount)
					return repoErr
				}).Once()
			},
			run: func(uc *usecase.SearchHistoryUseCase) error { return uc.ClearAll(ctx) },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			historyRepo := repomocks.NewMockHistoryRepository(t)
			coordinator := &recordingHistoryMutationCoordinator{}
			tt.expectRepo(historyRepo, coordinator)
			sink := &recordingHistoryChangeSink{}
			uc := usecase.NewSearchHistoryUseCase(historyRepo, sink)
			uc.SetHistoryMutationCoordinator(coordinator)

			err := tt.run(uc)

			require.Error(t, err)
			require.Equal(t, 1, coordinator.beginCount)
			require.Equal(t, 1, coordinator.releaseCount)
			assert.Empty(t, sink.changes)
		})
	}
}

func TestSearchHistoryUseCase_OtherDestructiveMutationsReturnCoordinatorErrorBeforeDeleting(t *testing.T) {
	ctx := testContext()
	before := time.Now().Add(-7 * 24 * time.Hour)
	tests := []struct {
		name string
		run  func(*usecase.SearchHistoryUseCase) error
	}{
		{name: "delete", run: func(uc *usecase.SearchHistoryUseCase) error { return uc.Delete(ctx, 42) }},
		{name: "delete by domain", run: func(uc *usecase.SearchHistoryUseCase) error { return uc.DeleteByDomain(ctx, "example.com") }},
		{name: "clear older than", run: func(uc *usecase.SearchHistoryUseCase) error { return uc.ClearOlderThan(ctx, before) }},
		{name: "clear range", run: func(uc *usecase.SearchHistoryUseCase) error { return uc.ClearRange(ctx, "hour") }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			historyRepo := repomocks.NewMockHistoryRepository(t)
			coordinator := &recordingHistoryMutationCoordinator{err: errors.New("flush failed")}
			sink := &recordingHistoryChangeSink{}
			uc := usecase.NewSearchHistoryUseCase(historyRepo, sink)
			uc.SetHistoryMutationCoordinator(coordinator)

			err := tt.run(uc)

			require.Error(t, err)
			require.Equal(t, 1, coordinator.beginCount)
			require.Zero(t, coordinator.releaseCount)
			assert.Empty(t, sink.changes)
		})
	}
}

func TestSearchHistoryUseCase_ClearAll_ReturnsCoordinatorErrorBeforeDeleting(t *testing.T) {
	ctx := testContext()
	historyRepo := repomocks.NewMockHistoryRepository(t)
	coordinator := &recordingHistoryMutationCoordinator{err: errors.New("flush failed")}
	sink := &recordingHistoryChangeSink{}
	uc := usecase.NewSearchHistoryUseCase(historyRepo, sink)
	uc.SetHistoryMutationCoordinator(coordinator)

	err := uc.ClearAll(ctx)

	require.Error(t, err)
	require.Equal(t, 1, coordinator.beginCount)
	require.Zero(t, coordinator.releaseCount)
	assert.Empty(t, sink.changes)
}

func TestSearchHistoryUseCase_ClearAll_DoesNotPublishOnRepositoryError(t *testing.T) {
	ctx := testContext()
	historyRepo := repomocks.NewMockHistoryRepository(t)
	historyRepo.EXPECT().DeleteAll(mock.Anything).Return(errors.New("clear all failed")).Once()
	sink := &recordingHistoryChangeSink{}
	uc := usecase.NewSearchHistoryUseCase(historyRepo, sink)

	err := uc.ClearAll(ctx)

	require.Error(t, err)
	assert.Empty(t, sink.changes)
}

func TestSearchHistoryUseCase_ClearOlderThan_PublishesClearChangeAfterSuccess(t *testing.T) {
	ctx := testContext()
	historyRepo := repomocks.NewMockHistoryRepository(t)
	before := time.Now().Add(-7 * 24 * time.Hour)
	historyRepo.EXPECT().DeleteOlderThan(mock.Anything, before).Return(nil).Once()
	sink := &recordingHistoryChangeSink{}
	uc := usecase.NewSearchHistoryUseCase(historyRepo, sink)

	err := uc.ClearOlderThan(ctx, before)

	require.NoError(t, err)
	requirePublishedHistoryChange(t, sink, dto.HistoryChangeReasonClear)
}

func TestSearchHistoryUseCase_ClearOlderThan_DoesNotPublishOnRepositoryError(t *testing.T) {
	ctx := testContext()
	historyRepo := repomocks.NewMockHistoryRepository(t)
	before := time.Now().Add(-7 * 24 * time.Hour)
	historyRepo.EXPECT().DeleteOlderThan(mock.Anything, before).Return(errors.New("clear old failed")).Once()
	sink := &recordingHistoryChangeSink{}
	uc := usecase.NewSearchHistoryUseCase(historyRepo, sink)

	err := uc.ClearOlderThan(ctx, before)

	require.Error(t, err)
	assert.Empty(t, sink.changes)
}

func TestSearchHistoryUseCase_ClearRange_RecentRangeDeletesSinceCutoffAndPublishesClearChange(t *testing.T) {
	ctx := testContext()
	historyRepo := repomocks.NewMockHistoryRepository(t)
	now := time.Now()
	expectedCutoff := now.Add(-time.Hour)
	historyRepo.EXPECT().DeleteSince(mock.Anything, mock.MatchedBy(func(since time.Time) bool {
		return since.After(expectedCutoff.Add(-time.Minute)) && since.Before(expectedCutoff.Add(time.Minute))
	})).Return(nil).Once()
	sink := &recordingHistoryChangeSink{}
	uc := usecase.NewSearchHistoryUseCase(historyRepo, sink)

	err := uc.ClearRange(ctx, "hour")

	require.NoError(t, err)
	requirePublishedHistoryChange(t, sink, dto.HistoryChangeReasonClear)
}

func TestSearchHistoryUseCase_ClearRange_RecentRangeDoesNotPublishOnRepositoryError(t *testing.T) {
	ctx := testContext()
	historyRepo := repomocks.NewMockHistoryRepository(t)
	historyRepo.EXPECT().DeleteSince(mock.Anything, mock.AnythingOfType("time.Time")).Return(errors.New("delete since failed")).Once()
	sink := &recordingHistoryChangeSink{}
	uc := usecase.NewSearchHistoryUseCase(historyRepo, sink)

	err := uc.ClearRange(ctx, "hour")

	require.Error(t, err)
	assert.Empty(t, sink.changes)
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

func TestSearchHistoryUseCase_ClearRange_AllUsesClearAllAndPublishesOnce(t *testing.T) {
	ctx := testContext()

	historyRepo := repomocks.NewMockHistoryRepository(t)

	historyRepo.EXPECT().DeleteAll(mock.Anything).Return(nil).Once()
	sink := &recordingHistoryChangeSink{}
	uc := usecase.NewSearchHistoryUseCase(historyRepo, sink)

	err := uc.ClearRange(ctx, "all")
	require.NoError(t, err)
	requirePublishedHistoryChange(t, sink, dto.HistoryChangeReasonClear)
}

func TestSearchHistoryUseCase_ClearRange_AllDoesNotPublishOnRepositoryError(t *testing.T) {
	ctx := testContext()

	historyRepo := repomocks.NewMockHistoryRepository(t)

	historyRepo.EXPECT().DeleteAll(mock.Anything).Return(errors.New("clear all failed")).Once()
	sink := &recordingHistoryChangeSink{}
	uc := usecase.NewSearchHistoryUseCase(historyRepo, sink)

	err := uc.ClearRange(ctx, "all")
	require.Error(t, err)
	assert.Empty(t, sink.changes)
}

func TestSearchHistoryUseCase_ClearRange_UnknownRangeDoesNotDeleteOrPublish(t *testing.T) {
	ctx := testContext()

	historyRepo := repomocks.NewMockHistoryRepository(t)
	sink := &recordingHistoryChangeSink{}
	uc := usecase.NewSearchHistoryUseCase(historyRepo, sink)

	err := uc.ClearRange(ctx, "bogus")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown history delete range")
	assert.Empty(t, sink.changes)
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
