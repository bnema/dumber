package usecase

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/bnema/dumber/internal/domain/entity"
	repomocks "github.com/bnema/dumber/internal/domain/repository/mocks"
	"github.com/bnema/dumber/internal/infrastructure/persistence/sqlite"
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

func TestUpdateHistoryTitle_UsesMetadataUpdateWithoutIncrementingVisits(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "dumber.db")
	db, err := sqlite.NewConnection(ctx, dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	repo := sqlite.NewHistoryRepository(db)
	require.NoError(t, repo.Save(ctx, &entity.HistoryEntry{
		URL:   "https://example.com/article",
		Title: "Old",
	}))

	before, err := repo.FindByURL(ctx, "https://example.com/article")
	require.NoError(t, err)
	require.NotNil(t, before)

	uc := NewNavigateUseCase(repo, nil, entity.ZoomDefault)
	uc.UpdateHistoryTitle(ctx, "https://example.com/article", "New")
	uc.Close()

	after, err := repo.FindByURL(ctx, "https://example.com/article")
	require.NoError(t, err)
	require.NotNil(t, after)
	require.Equal(t, "New", after.Title)
	require.Equal(t, before.VisitCount, after.VisitCount)
}

func TestHistoryWorker_ReenqueueDuringFlushIsPersistedOnShutdown(t *testing.T) {
	ctx := context.Background()

	// Stateful store — Save writes here, FindByURL reads from here.
	var mu sync.Mutex
	store := make(map[string]*entity.HistoryEntry)

	// Fires once during the first Save to re-enqueue a title update while
	// the flush is in progress. This is the exact race that caused the
	// concurrent map crash (fixed by swapping maps in flushPending).
	var saveOnce sync.Once
	var uc *NavigateUseCase

	repo := repomocks.NewMockHistoryRepository(t)

	repo.EXPECT().FindByURL(mock.Anything, mock.Anything).RunAndReturn(
		func(_ context.Context, url string) (*entity.HistoryEntry, error) {
			mu.Lock()
			defer mu.Unlock()
			e := store[url]
			if e == nil {
				return nil, nil
			}
			clone := *e
			return &clone, nil
		},
	).Maybe()

	repo.EXPECT().Save(mock.Anything, mock.Anything).RunAndReturn(
		func(_ context.Context, entry *entity.HistoryEntry) error {
			clone := *entry
			mu.Lock()
			store[entry.URL] = &clone
			mu.Unlock()

			// Re-enqueue a title update during the first Save.
			saveOnce.Do(func() {
				if entry.URL == "https://example.com/article" {
					uc.UpdateHistoryTitle(ctx, entry.URL, "Queued title")
				}
			})
			return nil
		},
	).Maybe()

	uc = NewNavigateUseCase(repo, nil, entity.ZoomDefault)
	uc.RecordHistory(ctx, "pane-1", "https://example.com/article")

	// The history worker flushes on a timer; there is no exported sync
	// mechanism, so sleep 3× the flush interval before shutting down.
	time.Sleep(historyWorkerFlushInterval * 3)
	uc.Close()

	mu.Lock()
	entry := store["https://example.com/article"]
	mu.Unlock()
	require.NotNil(t, entry)
	require.Equal(t, int64(1), entry.VisitCount)
	require.Equal(t, "Queued title", entry.Title)
}
