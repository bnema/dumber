package usecase

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/domain/repository"
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
	repo := newHistoryWorkerRegressionRepo()

	var uc *NavigateUseCase
	repo.onSave = func(entry *entity.HistoryEntry) {
		if entry.URL != "https://example.com/article" {
			return
		}
		uc.UpdateHistoryTitle(ctx, entry.URL, "Queued title")
	}

	uc = NewNavigateUseCase(repo, nil, entity.ZoomDefault)
	uc.RecordHistory(ctx, "pane-1", "https://example.com/article")

	// The history worker flushes on a timer; there is no exported sync/flush
	// mechanism we can hook into, so we sleep for 3× the flush interval to
	// give it a comfortable margin before shutting down.
	time.Sleep(historyWorkerFlushInterval * 3)
	uc.Close()

	entry, err := repo.FindByURL(ctx, "https://example.com/article")
	require.NoError(t, err)
	require.NotNil(t, entry)
	require.Equal(t, int64(1), entry.VisitCount)
	require.Equal(t, "Queued title", entry.Title)
}

// historyWorkerRegressionRepo is a hand-rolled fake instead of a mockery
// mock because mockery cannot express the re-enqueue-during-Save callback
// pattern needed for this regression test (onSave fires a second write
// back into the use-case while the first Save is still in progress).
type historyWorkerRegressionRepo struct {
	mu     sync.Mutex
	byURL  map[string]*entity.HistoryEntry
	saveMu sync.Once
	onSave func(*entity.HistoryEntry)
}

func newHistoryWorkerRegressionRepo() *historyWorkerRegressionRepo {
	return &historyWorkerRegressionRepo{
		byURL: make(map[string]*entity.HistoryEntry),
	}
}

func (r *historyWorkerRegressionRepo) Save(_ context.Context, entry *entity.HistoryEntry) error {
	clone := *entry

	r.mu.Lock()
	r.byURL[entry.URL] = &clone
	r.mu.Unlock()

	if r.onSave != nil {
		r.saveMu.Do(func() {
			r.onSave(&clone)
		})
	}
	return nil
}

func (r *historyWorkerRegressionRepo) FindByURL(_ context.Context, url string) (*entity.HistoryEntry, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	entry := r.byURL[url]
	if entry == nil {
		return nil, nil
	}
	clone := *entry
	return &clone, nil
}

func (*historyWorkerRegressionRepo) Search(context.Context, string, int) ([]entity.HistoryMatch, error) {
	return nil, nil
}

func (*historyWorkerRegressionRepo) GetRecent(context.Context, int, int) ([]*entity.HistoryEntry, error) {
	return nil, nil
}

func (*historyWorkerRegressionRepo) GetRecentSince(context.Context, int) ([]*entity.HistoryEntry, error) {
	return nil, nil
}

func (*historyWorkerRegressionRepo) GetMostVisited(context.Context, int) ([]*entity.HistoryEntry, error) {
	return nil, nil
}

func (*historyWorkerRegressionRepo) GetAllRecentHistory(context.Context) ([]*entity.HistoryEntry, error) {
	return nil, nil
}

func (*historyWorkerRegressionRepo) GetAllMostVisited(context.Context) ([]*entity.HistoryEntry, error) {
	return nil, nil
}

func (r *historyWorkerRegressionRepo) IncrementVisitCount(_ context.Context, url string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if entry := r.byURL[url]; entry != nil {
		entry.VisitCount++
	}
	return nil
}

func (*historyWorkerRegressionRepo) Delete(context.Context, int64) error {
	return nil
}

func (*historyWorkerRegressionRepo) DeleteOlderThan(context.Context, time.Time) error {
	return nil
}

func (*historyWorkerRegressionRepo) DeleteAll(context.Context) error {
	return nil
}

func (*historyWorkerRegressionRepo) DeleteByDomain(context.Context, string) error {
	return nil
}

func (*historyWorkerRegressionRepo) GetStats(context.Context) (*entity.HistoryStats, error) {
	return nil, nil
}

func (*historyWorkerRegressionRepo) GetDomainStats(context.Context, int) ([]*entity.DomainStat, error) {
	return nil, nil
}

func (*historyWorkerRegressionRepo) GetHourlyDistribution(context.Context) ([]*entity.HourlyDistribution, error) {
	return nil, nil
}

func (*historyWorkerRegressionRepo) GetDailyVisitCount(context.Context, string) ([]*entity.DailyVisitCount, error) {
	return nil, nil
}

var _ repository.HistoryRepository = (*historyWorkerRegressionRepo)(nil)
