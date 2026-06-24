package usecase

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/bnema/dumber/internal/application/dto"
	"github.com/bnema/dumber/internal/domain/entity"
	repomocks "github.com/bnema/dumber/internal/domain/repository/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type recordingHistoryChangeSink struct {
	mu      sync.Mutex
	changes []dto.HistoryChange
}

func (s *recordingHistoryChangeSink) OnHistoryChanged(_ context.Context, change dto.HistoryChange) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.changes = append(s.changes, change)
}

func (s *recordingHistoryChangeSink) snapshot() []dto.HistoryChange {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]dto.HistoryChange, len(s.changes))
	copy(out, s.changes)
	return out
}

func TestNormalizeHistoryChangeSink_TreatsTypedNilAsNoop(t *testing.T) {
	var sink *recordingHistoryChangeSink

	normalized := normalizeHistoryChangeSink(sink)

	require.NotNil(t, normalized)
	require.NotPanics(t, func() {
		normalized.OnHistoryChanged(context.Background(), dto.HistoryChange{Reasons: []dto.HistoryChangeReason{dto.HistoryChangeReasonVisit}})
	})
}

type metadataUpdatingHistoryRepository struct {
	*repomocks.MockHistoryRepository
	mu    sync.Mutex
	entry *entity.HistoryEntry
}

func (r *metadataUpdatingHistoryRepository) UpdateMetadata(_ context.Context, entry *entity.HistoryEntry) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	clone := *entry
	r.entry = &clone
	return nil
}

func (r *metadataUpdatingHistoryRepository) snapshotEntry() *entity.HistoryEntry {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.entry == nil {
		return nil
	}
	clone := *r.entry
	return &clone
}

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

func TestCanonicalizeURLForHistory_RemovesUserinfo(t *testing.T) {
	got := canonicalizeURLForHistory(
		"https://user:password@example.com/path#section-1",
		historyCanonicalizationOptions{StripTrackingParams: true},
	)

	require.Equal(t, "https://example.com/path", got)
}

func TestCanonicalizeURLForHistory_RemovesSchemeDefaultPorts(t *testing.T) {
	require.Equal(t, "http://example.com/path", canonicalizeURLForHistory(
		"http://Example.com:80/path",
		historyCanonicalizationOptions{StripTrackingParams: true},
	))
	require.Equal(t, "https://example.com/path", canonicalizeURLForHistory(
		"https://Example.com:443/path",
		historyCanonicalizationOptions{StripTrackingParams: true},
	))
	require.Equal(t, "https://example.com:8443/path", canonicalizeURLForHistory(
		"https://Example.com:8443/path",
		historyCanonicalizationOptions{StripTrackingParams: true},
	))
}

func TestHistoryRecorder_RecordHistory_IgnoresNonWebSchemes(t *testing.T) {
	ctx := context.Background()
	sink := &recordingHistoryChangeSink{}
	repo := repomocks.NewMockHistoryRepository(t)

	uc := NewHistoryRecorderUseCase(repo, sink)
	uc.RecordHistory(ctx, "pane-1", "file:///private/history.txt")
	uc.RecordHistory(ctx, "pane-1", "data:text/plain,secret")
	uc.RecordHistory(ctx, "pane-1", "dumb://history")
	uc.RecordHistory(ctx, "pane-1", "about:blank")
	uc.Close()

	require.Empty(t, sink.snapshot())
}

func TestHistoryRecorder_UpdateHistoryTitle_IgnoresNonWebSchemes(t *testing.T) {
	ctx := context.Background()
	sink := &recordingHistoryChangeSink{}
	repo := repomocks.NewMockHistoryRepository(t)

	uc := NewHistoryRecorderUseCase(repo, sink)
	uc.UpdateHistoryTitle(ctx, "file:///private/history.txt", "Secret")
	uc.UpdateHistoryTitle(ctx, "dumb://history", "History")
	uc.Close()

	require.Empty(t, sink.snapshot())
}

func TestIsHashOnlyTransition_UsesCanonicalComparison(t *testing.T) {
	tests := []struct {
		name     string
		previous string
		current  string
	}{
		{
			name:     "query order and tracking params",
			previous: "https://example.com/docs?utm_source=newsletter&b=2&a=1#intro",
			current:  "https://example.com/docs/?a=1&b=2#api",
		},
		{
			name:     "default port",
			previous: "https://example.com:443/docs#intro",
			current:  "https://example.com/docs#api",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.True(t, isHashOnlyTransition(tt.previous, tt.current))
		})
	}
}

func TestHistoryRecorder_RecordHistory_CoalescesVisitsAndPublishesOneChange(t *testing.T) {
	ctx := context.Background()
	sink := &recordingHistoryChangeSink{}
	repo := repomocks.NewMockHistoryRepository(t)

	repo.EXPECT().FindByURL(mock.Anything, "https://example.com/article").Return(nil, nil).Once()
	repo.EXPECT().Save(mock.Anything, mock.MatchedBy(func(entry *entity.HistoryEntry) bool {
		return entry.URL == "https://example.com/article" && entry.VisitCount == 2
	})).Return(nil).Once()

	uc := NewHistoryRecorderUseCase(repo, sink)
	uc.RecordHistory(ctx, "pane-1", "https://example.com/article?utm_source=feed")
	uc.RecordHistory(ctx, "pane-2", "https://example.com/article")
	uc.Close()

	changes := sink.snapshot()
	require.Len(t, changes, 1)
	require.Equal(t, []dto.HistoryChangeReason{dto.HistoryChangeReasonVisit}, changes[0].Reasons)
	require.Equal(t, 2, changes[0].VisitCount)
	require.Zero(t, changes[0].TitleCount)
}

func TestHistoryRecorder_CloseDrainsPendingAndPublishes(t *testing.T) {
	ctx := context.Background()
	sink := &recordingHistoryChangeSink{}
	repo := repomocks.NewMockHistoryRepository(t)

	repo.EXPECT().FindByURL(mock.Anything, "https://example.com/close").Return(nil, nil).Once()
	repo.EXPECT().Save(mock.Anything, mock.MatchedBy(func(entry *entity.HistoryEntry) bool {
		return entry.URL == "https://example.com/close" && entry.VisitCount == 1
	})).Return(nil).Once()

	uc := NewHistoryRecorderUseCase(repo, sink)
	uc.RecordHistory(ctx, "pane-1", "https://example.com/close")
	uc.Close()

	changes := sink.snapshot()
	require.Len(t, changes, 1)
	require.Equal(t, 1, changes[0].VisitCount)
}

func TestHistoryRecorder_FailedPersistencePublishesNoChange(t *testing.T) {
	ctx := context.Background()
	sink := &recordingHistoryChangeSink{}
	repo := repomocks.NewMockHistoryRepository(t)

	repo.EXPECT().FindByURL(mock.Anything, "https://example.com/fail").Return(nil, errors.New("db down")).Times(3)

	uc := NewHistoryRecorderUseCase(repo, sink)
	uc.RecordHistory(ctx, "pane-1", "https://example.com/fail")
	uc.Close()

	require.Empty(t, sink.snapshot())
}

func TestHistoryRecorder_RecordHistory_DoesNotUpdateDedupeStateWhenEnqueueFails(t *testing.T) {
	ctx := context.Background()
	uc := &HistoryRecorderUseCase{
		recentVisits: make(map[string]paneHistoryState),
		historyQueue: make(chan historyRecord, 1),
	}
	uc.historyQueue <- historyRecord{url: "https://queued.example", visits: 1}

	uc.RecordHistory(ctx, "pane-1", "https://example.com/full")

	uc.recentMu.Lock()
	_, ok := uc.recentVisits["pane-1"]
	uc.recentMu.Unlock()
	require.False(t, ok, "failed enqueue must not suppress a later retry through dedupe state")
}

func TestHistoryRecorder_UpdateHistoryTitle_PublishesTitleOnlyWithoutIncrementingVisits(t *testing.T) {
	ctx := context.Background()
	sink := &recordingHistoryChangeSink{}
	repo := &metadataUpdatingHistoryRepository{
		MockHistoryRepository: repomocks.NewMockHistoryRepository(t),
		entry: &entity.HistoryEntry{
			URL:        "https://example.com/article",
			Title:      "Old",
			VisitCount: 7,
		},
	}
	repo.EXPECT().FindByURL(mock.Anything, "https://example.com/article").RunAndReturn(
		func(context.Context, string) (*entity.HistoryEntry, error) {
			return repo.snapshotEntry(), nil
		},
	).Once()

	uc := NewHistoryRecorderUseCase(repo, sink)
	uc.UpdateHistoryTitle(ctx, "https://example.com/article", "New")
	uc.Close()

	after := repo.snapshotEntry()
	require.NotNil(t, after)
	require.Equal(t, "New", after.Title)
	require.Equal(t, int64(7), after.VisitCount)

	changes := sink.snapshot()
	require.Len(t, changes, 1)
	require.Equal(t, []dto.HistoryChangeReason{dto.HistoryChangeReasonTitle}, changes[0].Reasons)
	require.Zero(t, changes[0].VisitCount)
	require.Equal(t, 1, changes[0].TitleCount)
}

func TestHistoryRecorder_FlushPendingHistoryDiscardsTitleWhenHistoryRowMissing(t *testing.T) {
	ctx := context.Background()
	sink := &recordingHistoryChangeSink{}
	repo := repomocks.NewMockHistoryRepository(t)
	const historyURL = "https://example.com/missing-title"

	repo.EXPECT().FindByURL(mock.Anything, historyURL).Return(nil, nil).Once()

	uc := &HistoryRecorderUseCase{historyRepo: repo, changeSink: sink, ctx: ctx}
	pending := newPendingHistoryRecords()
	pending.add(historyRecord{url: historyURL, title: "Missing"})

	err := uc.flushPendingHistory(ctx, pending)

	require.NoError(t, err)
	require.Empty(t, pending.visits)
	require.Empty(t, pending.titles)
	require.Empty(t, sink.snapshot())
}

func TestHistoryRecorder_RecordHistory_IgnoresHashOnlyTransitionsAndPublishesNoSecondChange(t *testing.T) {
	ctx := context.Background()
	sink := &recordingHistoryChangeSink{}
	repo := repomocks.NewMockHistoryRepository(t)

	repo.EXPECT().FindByURL(mock.Anything, "https://example.com/docs").Return(nil, nil).Once()
	repo.EXPECT().Save(mock.Anything, mock.MatchedBy(func(entry *entity.HistoryEntry) bool {
		return entry.URL == "https://example.com/docs" && entry.VisitCount == 1
	})).Return(nil).Once()

	uc := NewHistoryRecorderUseCase(repo, sink)
	uc.RecordHistory(ctx, "pane-1", "https://example.com/docs#intro")
	uc.RecordHistory(ctx, "pane-1", "https://example.com/docs#api")
	uc.Close()

	changes := sink.snapshot()
	require.Len(t, changes, 1)
	require.Equal(t, 1, changes[0].VisitCount)
}

func TestHistoryRecorder_RecordHistory_DedupIsPerPane(t *testing.T) {
	ctx := context.Background()
	sink := &recordingHistoryChangeSink{}
	repo := repomocks.NewMockHistoryRepository(t)

	repo.EXPECT().FindByURL(mock.Anything, "https://example.com/dedupe").Return(nil, nil).Once()
	repo.EXPECT().Save(mock.Anything, mock.MatchedBy(func(entry *entity.HistoryEntry) bool {
		return entry.URL == "https://example.com/dedupe" && entry.VisitCount == 2
	})).Return(nil).Once()

	uc := NewHistoryRecorderUseCase(repo, sink)
	uc.RecordHistory(ctx, "pane-1", "https://example.com/dedupe")
	uc.RecordHistory(ctx, "pane-1", "https://example.com/dedupe")
	uc.RecordHistory(ctx, "pane-2", "https://example.com/dedupe")
	uc.Close()

	changes := sink.snapshot()
	require.Len(t, changes, 1)
	require.Equal(t, 2, changes[0].VisitCount)
}

func TestHistoryRecorder_FlushPendingHistoryTracksPartialFallbackVisitWrites(t *testing.T) {
	ctx := context.Background()
	sink := &recordingHistoryChangeSink{}
	repo := repomocks.NewMockHistoryRepository(t)
	const historyURL = "https://example.com/partial"
	increments := 0

	repo.EXPECT().FindByURL(mock.Anything, historyURL).Return(&entity.HistoryEntry{URL: historyURL}, nil).Once()
	repo.EXPECT().IncrementVisitCount(mock.Anything, historyURL).RunAndReturn(func(context.Context, string) error {
		increments++
		if increments == 3 {
			return errors.New("increment failed")
		}
		return nil
	}).Times(3)

	uc := &HistoryRecorderUseCase{historyRepo: repo, changeSink: sink, ctx: ctx}
	pending := newPendingHistoryRecords()
	pending.add(historyRecord{url: historyURL, visits: 3})

	err := uc.flushPendingHistory(ctx, pending)

	require.ErrorIs(t, err, errHistoryPersistenceIncomplete)
	require.Equal(t, map[string]int{historyURL: 1}, pending.visits)
	require.Empty(t, pending.titles)
	changes := sink.snapshot()
	require.Len(t, changes, 1)
	require.Equal(t, 2, changes[0].VisitCount)
}

func TestHistoryRecorder_HandleHistoryControlKeepsDedupeWhenFlushFails(t *testing.T) {
	ctx := context.Background()
	repo := repomocks.NewMockHistoryRepository(t)
	const historyURL = "https://example.com/fail-reset"

	repo.EXPECT().FindByURL(mock.Anything, historyURL).Return(nil, errors.New("db down")).Once()

	uc := &HistoryRecorderUseCase{
		historyRepo: repo,
		changeSink:  &recordingHistoryChangeSink{},
		recentVisits: map[string]paneHistoryState{
			"pane-1": {lastCanonicalURL: historyURL},
		},
		ctx: ctx,
	}
	pending := newPendingHistoryRecords()
	pending.add(historyRecord{url: historyURL, visits: 1})
	req := historyControlRequest{kind: historyControlFlushAndReset, ctx: ctx, done: make(chan error, 1)}

	uc.handleHistoryControl(pending, req)

	require.ErrorIs(t, <-req.done, errHistoryPersistenceIncomplete)
	require.Contains(t, uc.recentVisits, "pane-1")
}

func TestHistoryRecorder_BeginHistoryMutationFlushesPendingAndResetsDedupe(t *testing.T) {
	ctx := context.Background()
	sink := &recordingHistoryChangeSink{}
	repo := repomocks.NewMockHistoryRepository(t)

	repo.EXPECT().FindByURL(mock.Anything, "https://example.com/reset").Return(nil, nil).Twice()
	repo.EXPECT().Save(mock.Anything, mock.MatchedBy(func(entry *entity.HistoryEntry) bool {
		return entry.URL == "https://example.com/reset" && entry.VisitCount == 1
	})).Return(nil).Twice()

	uc := NewHistoryRecorderUseCase(repo, sink)
	uc.RecordHistory(ctx, "pane-1", "https://example.com/reset")

	release, err := uc.BeginHistoryMutation(ctx)
	require.NoError(t, err)
	release()

	uc.RecordHistory(ctx, "pane-1", "https://example.com/reset")
	uc.Close()

	changes := sink.snapshot()
	require.Len(t, changes, 2)
	require.Equal(t, 1, changes[0].VisitCount)
	require.Equal(t, 1, changes[1].VisitCount)
}

func TestHistoryRecorder_BeginHistoryMutationUsesCallerContextForFlush(t *testing.T) {
	type contextKey string
	ctx := context.WithValue(context.Background(), contextKey("mutation"), "flush")
	sink := &recordingHistoryChangeSink{}
	repo := repomocks.NewMockHistoryRepository(t)

	repo.EXPECT().FindByURL(mock.Anything, "https://example.com/context").RunAndReturn(
		func(ctx context.Context, _ string) (*entity.HistoryEntry, error) {
			require.Equal(t, "flush", ctx.Value(contextKey("mutation")))
			return nil, nil
		},
	).Once()
	repo.EXPECT().Save(mock.Anything, mock.MatchedBy(func(entry *entity.HistoryEntry) bool {
		return entry.URL == "https://example.com/context" && entry.VisitCount == 1
	})).RunAndReturn(func(ctx context.Context, _ *entity.HistoryEntry) error {
		require.Equal(t, "flush", ctx.Value(contextKey("mutation")))
		return nil
	}).Once()

	uc := NewHistoryRecorderUseCase(repo, sink)
	uc.RecordHistory(context.Background(), "pane-1", "https://example.com/context")

	release, err := uc.BeginHistoryMutation(ctx)
	require.NoError(t, err)
	release()
	uc.Close()
}

func TestHistoryRecorder_BeginHistoryMutationWaitsForInFlightShutdownDrain(t *testing.T) {
	ctx := context.Background()
	saveStarted := make(chan struct{})
	allowSave := make(chan struct{})
	repo := repomocks.NewMockHistoryRepository(t)

	repo.EXPECT().FindByURL(mock.Anything, "https://example.com/shutdown").Return(nil, nil).Once()
	repo.EXPECT().Save(mock.Anything, mock.MatchedBy(func(entry *entity.HistoryEntry) bool {
		return entry.URL == "https://example.com/shutdown" && entry.VisitCount == 1
	})).RunAndReturn(func(context.Context, *entity.HistoryEntry) error {
		close(saveStarted)
		<-allowSave
		return nil
	}).Once()

	uc := NewHistoryRecorderUseCase(repo, nil)
	uc.RecordHistory(ctx, "pane-1", "https://example.com/shutdown")

	closeDone := make(chan struct{})
	go func() {
		uc.Close()
		close(closeDone)
	}()
	select {
	case <-saveStarted:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for history save to start")
	}

	mutationDone := make(chan error, 1)
	beginStarted := make(chan struct{})
	go func() {
		close(beginStarted)
		release, err := uc.BeginHistoryMutation(ctx)
		if release != nil {
			release()
		}
		mutationDone <- err
	}()
	select {
	case <-beginStarted:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for history mutation to start")
	}

	select {
	case err := <-mutationDone:
		t.Fatalf("history mutation returned before shutdown drain finished: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	close(allowSave)
	select {
	case err := <-mutationDone:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for history mutation to finish")
	}
	select {
	case <-closeDone:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for history recorder shutdown")
	}
}

func TestHistoryWorker_ReenqueueDuringFinalShutdownFlushIsRejected(t *testing.T) {
	assertReenqueueDuringFlush(t, false, "")
}

func TestHistoryWorker_ReenqueueDuringPeriodicFlushIsPersisted(t *testing.T) {
	assertReenqueueDuringFlush(t, true, "Queued title")
}

func assertReenqueueDuringFlush(t *testing.T, waitForPeriodicFlush bool, expectedTitle string) {
	t.Helper()
	ctx := context.Background()

	var mu sync.Mutex
	store := make(map[string]*entity.HistoryEntry)
	saved := make(chan string, 4)
	var saveOnce sync.Once
	var uc *HistoryRecorderUseCase

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

			saveOnce.Do(func() {
				if entry.URL == "https://example.com/article" {
					uc.UpdateHistoryTitle(ctx, entry.URL, "Queued title")
				}
			})
			select {
			case saved <- entry.URL:
			default:
			}
			return nil
		},
	).Maybe()

	uc = NewHistoryRecorderUseCase(repo, nil)
	uc.RecordHistory(ctx, "pane-1", "https://example.com/article")
	if waitForPeriodicFlush {
		require.Eventually(t, func() bool {
			select {
			case url := <-saved:
				return url == "https://example.com/article"
			default:
				return false
			}
		}, time.Second, 10*time.Millisecond, "periodic flush should save the initial visit before Close")
	}
	uc.Close()

	mu.Lock()
	entry := store["https://example.com/article"]
	mu.Unlock()
	require.NotNil(t, entry)
	require.Equal(t, int64(1), entry.VisitCount)
	require.Equal(t, expectedTitle, entry.Title)
}
