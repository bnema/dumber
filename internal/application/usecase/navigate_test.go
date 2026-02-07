package usecase

import (
	"context"
	"testing"

	"github.com/bnema/dumber/internal/domain/entity"
	repomocks "github.com/bnema/dumber/internal/domain/repository/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestCanonicalizeURLForHistory_StripsHashTrackingAndTrailingSlash(t *testing.T) {
	got := canonicalizeURLForHistory(
		"https://Example.com/path/?utm_source=newsletter&utm_campaign=launch&a=2&b=1#section-1",
		historyCanonicalizationOptions{StripTrackingParams: true},
	)

	require.Equal(t, "https://example.com/path?a=2&b=1", got)
}

func TestCanonicalizeURLForHistory_OptionallyKeepsTrackingParams(t *testing.T) {
	got := canonicalizeURLForHistory(
		"https://example.com/path/?utm_source=newsletter&b=1#section-1",
		historyCanonicalizationOptions{StripTrackingParams: false},
	)

	require.Equal(t, "https://example.com/path?b=1&utm_source=newsletter", got)
}

func TestRecordHistory_IgnoresHashOnlyTransitions(t *testing.T) {
	ctx := context.Background()
	repo := repomocks.NewMockHistoryRepository(t)

	repo.EXPECT().FindByURL(mock.Anything, "https://example.com/docs").Return(nil, nil).Once()
	repo.EXPECT().Save(mock.Anything, mock.MatchedBy(func(entry *entity.HistoryEntry) bool {
		return entry.URL == "https://example.com/docs" && entry.VisitCount == 1
	})).Return(nil).Once()

	uc := NewNavigateUseCase(repo, nil, entity.ZoomDefault)
	uc.RecordHistory(ctx, "pane-1", "https://example.com/docs#intro")
	uc.RecordHistory(ctx, "pane-1", "https://example.com/docs#api")
	uc.Close()
}

func TestRecordHistory_DedupIsPerPaneAndCoalescesVisits(t *testing.T) {
	ctx := context.Background()
	repo := repomocks.NewMockHistoryRepository(t)

	repo.EXPECT().FindByURL(mock.Anything, "https://example.com/article").Return(nil, nil).Once()
	repo.EXPECT().Save(mock.Anything, mock.MatchedBy(func(entry *entity.HistoryEntry) bool {
		return entry.URL == "https://example.com/article" && entry.VisitCount == 2
	})).Return(nil).Once()

	uc := NewNavigateUseCase(repo, nil, entity.ZoomDefault)
	uc.RecordHistory(ctx, "pane-1", "https://example.com/article?utm_source=feed")
	uc.RecordHistory(ctx, "pane-1", "https://example.com/article?utm_source=feed")
	uc.RecordHistory(ctx, "pane-2", "https://example.com/article")
	uc.Close()
}
