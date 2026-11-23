package systemviews

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"

	"bnema.dev/dumber/internal/application/dto"
	"bnema.dev/dumber/internal/application/port/mocks"
	"bnema.dev/dumber/internal/domain/entity"
)

// historyStore is the stored history behind a mocked history service. Reads
// (TimelineWindow, Search, Stats, DomainStats) are served from it and recorded.
// Deletes need expectDelete* per call; Timeline, TimelineByDomain and Analytics have no
// expectation, so any call to them fails the test.
type historyStore struct {
	service *mocks.MockSystemviewHistoryService
	mu      sync.Mutex // guards delete records; deletes run concurrently

	called         bool // TimelineWindow
	windowBefore   time.Time
	windowBeforeID int64
	windowDomain   string
	searchCalled   bool
	query          string
	searchLimit    int
	statsCalled    bool

	entries       []*entity.HistoryEntry
	searchEntries []*entity.HistoryEntry
	window        *entity.HistoryWindow
	err           error
	stats         *entity.HistoryStats
	domainStats   []*entity.DomainStat

	deletedEntries []int64
	deleteErr      error

	// When set, TimelineWindow closes timelineStarted and blocks until
	// releaseTimeline is closed.
	timelineStarted chan struct{}
	releaseTimeline chan struct{}
	startOnce       sync.Once
}

func newHistoryStore(t *testing.T, s *historyStore) *historyStore {
	t.Helper()
	svc := mocks.NewMockSystemviewHistoryService(t)
	s.service = svc
	svc.EXPECT().TimelineWindow(mock.Anything, mock.Anything, mock.Anything, mock.Anything).RunAndReturn(
		func(_ context.Context, before time.Time, beforeID int64, domain string) (*entity.HistoryWindow, error) {
			s.called = true
			s.windowBefore = before
			s.windowBeforeID = beforeID
			s.windowDomain = domain
			if s.timelineStarted != nil {
				s.startOnce.Do(func() { close(s.timelineStarted) })
			}
			if s.releaseTimeline != nil {
				<-s.releaseTimeline
			}
			if s.err != nil {
				return nil, s.err
			}
			if s.window != nil {
				return s.window, nil
			}
			return &entity.HistoryWindow{Entries: s.entries, Before: before, After: before.Add(-24 * time.Hour)}, nil
		}).Maybe()
	svc.EXPECT().Search(mock.Anything, mock.Anything, mock.Anything).RunAndReturn(
		func(_ context.Context, query string, limit int) ([]*entity.HistoryEntry, error) {
			s.searchCalled = true
			s.query = query
			s.searchLimit = limit
			if s.searchEntries != nil {
				return s.searchEntries, nil
			}
			return s.entries, nil
		}).Maybe()
	svc.EXPECT().Stats(mock.Anything).RunAndReturn(func(context.Context) (*entity.HistoryStats, error) {
		s.statsCalled = true
		return s.stats, nil
	}).Maybe()
	svc.EXPECT().DomainStats(mock.Anything, mock.Anything).RunAndReturn(
		func(context.Context, int) ([]*entity.DomainStat, error) { return s.domainStats, nil }).Maybe()
	return s
}

// expectDeleteEntry expects one DeleteEntry per id and records it. Each
// call returns deleteErr.
func (s *historyStore) expectDeleteEntry(ids ...int64) {
	for _, id := range ids {
		s.service.EXPECT().DeleteEntry(mock.Anything, id).RunAndReturn(func(_ context.Context, id int64) error {
			s.mu.Lock()
			defer s.mu.Unlock()
			s.deletedEntries = append(s.deletedEntries, id)
			return s.deleteErr
		}).Once()
	}
}

func (s *historyStore) deletedEntryIDs() []int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.deletedEntries)
}

func (s *historyStore) expectDeleteRange(rangeID string) {
	s.service.EXPECT().DeleteRange(mock.Anything, rangeID).Return(nil).Once()
}

func (s *historyStore) expectDeleteDomain(domain string) {
	s.service.EXPECT().DeleteDomain(mock.Anything, domain).Return(nil).Once()
}

// favoritesStore is the stored favorites behind a mocked favorites service.
// List and ListTags serve it; each expected write needs its expect* helper.
type favoritesStore struct {
	service *mocks.MockSystemviewFavoritesService
	mu      sync.Mutex // guards delete records; deletes run concurrently

	calledList bool
	calledTags bool
	favorites  []*entity.Favorite
	tags       []*entity.Tag

	createdFavorite  dto.FavoriteCreateInput
	updatedFavorite  dto.FavoriteUpdateInput
	deletedFavorites []int64
	deletedTags      []int64
	assigned         [][2]int64
	removed          [][2]int64
	shortcuts        map[int64]*int
	failDelete       error
	failAssign       error
}

func newFavoritesStore(t *testing.T, s *favoritesStore) *favoritesStore {
	t.Helper()
	svc := mocks.NewMockSystemviewFavoritesService(t)
	s.service = svc
	svc.EXPECT().List(mock.Anything).RunAndReturn(func(context.Context) ([]*entity.Favorite, error) {
		s.calledList = true
		return s.favorites, nil
	}).Maybe()
	svc.EXPECT().ListTags(mock.Anything).RunAndReturn(func(context.Context) ([]*entity.Tag, error) {
		s.calledTags = true
		return s.tags, nil
	}).Maybe()
	return s
}

// expectCreateFavorite expects one CreateFavorite for url, records the
// input and returns favorite 99.
func (s *favoritesStore) expectCreateFavorite(url string) {
	s.service.EXPECT().CreateFavorite(mock.Anything, mock.MatchedBy(func(in dto.FavoriteCreateInput) bool { return in.URL == url })).
		RunAndReturn(func(_ context.Context, input dto.FavoriteCreateInput) (*entity.Favorite, error) {
			s.createdFavorite = input
			return &entity.Favorite{ID: 99, URL: input.URL, Title: input.Title}, nil
		}).Once()
}

// expectUpdateFavorite expects one UpdateFavorite for id and echoes the input.
func (s *favoritesStore) expectUpdateFavorite(id int64) {
	s.service.EXPECT().UpdateFavorite(mock.Anything, mock.MatchedBy(func(in dto.FavoriteUpdateInput) bool { return int64(in.ID) == id })).
		RunAndReturn(func(_ context.Context, input dto.FavoriteUpdateInput) (*entity.Favorite, error) {
			s.updatedFavorite = input
			return &entity.Favorite{ID: input.ID, URL: "https://example.com", Title: input.Title, FaviconURL: input.FaviconURL, ShortcutKey: input.ShortcutKey}, nil
		}).Once()
}

// expectDeleteFavorite expects one DeleteFavorite per id. A call returns
// failDelete when set and records the id otherwise.
func (s *favoritesStore) expectDeleteFavorite(ids ...int64) {
	for _, id := range ids {
		s.service.EXPECT().DeleteFavorite(mock.Anything, id).RunAndReturn(func(_ context.Context, id int64) error {
			s.mu.Lock()
			defer s.mu.Unlock()
			if s.failDelete != nil {
				return s.failDelete
			}
			s.deletedFavorites = append(s.deletedFavorites, id)
			return nil
		}).Once()
	}
}

// expectDeleteTag expects one DeleteTag per id and records it.
func (s *favoritesStore) expectDeleteTag(ids ...int64) {
	for _, id := range ids {
		s.service.EXPECT().DeleteTag(mock.Anything, id).RunAndReturn(func(_ context.Context, id int64) error {
			s.mu.Lock()
			defer s.mu.Unlock()
			s.deletedTags = append(s.deletedTags, id)
			return nil
		}).Once()
	}
}

func (s *favoritesStore) deletedFavoriteIDs() []int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.deletedFavorites)
}

func (s *favoritesStore) deletedTagIDs() []int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.deletedTags)
}

// expectSetShortcut expects one SetShortcut for id and records the key.
func (s *favoritesStore) expectSetShortcut(id int64) {
	s.service.EXPECT().SetShortcut(mock.Anything, id, mock.Anything).RunAndReturn(
		func(_ context.Context, id int64, key *int) error {
			if s.shortcuts == nil {
				s.shortcuts = map[int64]*int{}
			}
			s.shortcuts[id] = key
			return nil
		}).Once()
}

// expectAssignTag expects one AssignTag; it returns failAssign when set.
func (s *favoritesStore) expectAssignTag(favoriteID, tagID int64) {
	s.service.EXPECT().AssignTag(mock.Anything, favoriteID, tagID).RunAndReturn(
		func(_ context.Context, favoriteID, tagID int64) error {
			if s.failAssign != nil {
				return s.failAssign
			}
			s.assigned = append(s.assigned, [2]int64{favoriteID, tagID})
			return nil
		}).Once()
}

// expectRemoveTag expects one RemoveTag and records it.
func (s *favoritesStore) expectRemoveTag(favoriteID, tagID int64) {
	s.service.EXPECT().RemoveTag(mock.Anything, favoriteID, tagID).RunAndReturn(
		func(_ context.Context, favoriteID, tagID int64) error {
			s.removed = append(s.removed, [2]int64{favoriteID, tagID})
			return nil
		}).Once()
}
