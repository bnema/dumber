package systemviews

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"

	"github.com/bnema/dumber/internal/application/dto"
	"github.com/bnema/dumber/internal/application/port/mocks"
	"github.com/bnema/dumber/internal/domain/entity"
)

// historyStore is the stored history behind a mocked history service. Reads
// (TimelineWindow, Search, Stats, DomainStats) are served from it and recorded.
// Deletes need expectDeletes; Timeline, TimelineByDomain and Analytics have no
// expectation, so any call to them fails the test.
type historyStore struct {
	service *mocks.MockSystemviewHistoryService

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

	deletedEntryID int64
	deleteErr      error
	deletedRangeID string
	deletedDomain  string

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

// expectDeletes lets the service accept deletes and records them.
func (s *historyStore) expectDeletes() {
	s.service.EXPECT().DeleteEntry(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, id int64) error {
		s.deletedEntryID = id
		return s.deleteErr
	}).Maybe()
	s.service.EXPECT().DeleteRange(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, id string) error {
		s.deletedRangeID = id
		return nil
	}).Maybe()
	s.service.EXPECT().DeleteDomain(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, domain string) error {
		s.deletedDomain = domain
		return nil
	}).Maybe()
}

// favoritesStore is the stored favorites behind a mocked favorites service.
// List and ListTags serve it; writes need expectWrites and are recorded.
type favoritesStore struct {
	service *mocks.MockSystemviewFavoritesService

	calledList bool
	calledTags bool
	favorites  []*entity.Favorite
	tags       []*entity.Tag

	createdFavorite dto.FavoriteCreateInput
	updatedFavorite dto.FavoriteUpdateInput
	deletedFavorite int64
	createdTag      string
	updatedTagID    int64
	deletedTagID    int64
	assigned        [][2]int64
	removed         [][2]int64
	shortcuts       map[int64]*int
	failDelete      error
	failAssign      error
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

// expectWrites lets the service accept favorite and tag writes and records
// them. failDelete and failAssign make those calls fail without recording.
func (s *favoritesStore) expectWrites() {
	svc := s.service
	svc.EXPECT().CreateFavorite(mock.Anything, mock.Anything).RunAndReturn(
		func(_ context.Context, input dto.FavoriteCreateInput) (*entity.Favorite, error) {
			s.createdFavorite = input
			return &entity.Favorite{ID: 99, URL: input.URL, Title: input.Title}, nil
		}).Maybe()
	svc.EXPECT().UpdateFavorite(mock.Anything, mock.Anything).RunAndReturn(
		func(_ context.Context, input dto.FavoriteUpdateInput) (*entity.Favorite, error) {
			s.updatedFavorite = input
			return &entity.Favorite{ID: input.ID, URL: "https://example.com", Title: input.Title, FaviconURL: input.FaviconURL, ShortcutKey: input.ShortcutKey}, nil
		}).Maybe()
	svc.EXPECT().DeleteFavorite(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, id int64) error {
		if s.failDelete != nil {
			return s.failDelete
		}
		s.deletedFavorite = id
		return nil
	}).Maybe()
	svc.EXPECT().SetShortcut(mock.Anything, mock.Anything, mock.Anything).RunAndReturn(
		func(_ context.Context, id int64, key *int) error {
			if s.shortcuts == nil {
				s.shortcuts = map[int64]*int{}
			}
			s.shortcuts[id] = key
			return nil
		}).Maybe()
	svc.EXPECT().CreateTag(mock.Anything, mock.Anything, mock.Anything).RunAndReturn(
		func(_ context.Context, name, color string) (*entity.Tag, error) {
			s.createdTag = name
			return &entity.Tag{ID: 55, Name: name, Color: color}, nil
		}).Maybe()
	svc.EXPECT().UpdateTag(mock.Anything, mock.Anything, mock.Anything, mock.Anything).RunAndReturn(
		func(_ context.Context, id int64, _, _ string) error {
			s.updatedTagID = id
			return nil
		}).Maybe()
	svc.EXPECT().DeleteTag(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, id int64) error {
		s.deletedTagID = id
		return nil
	}).Maybe()
	svc.EXPECT().AssignTag(mock.Anything, mock.Anything, mock.Anything).RunAndReturn(
		func(_ context.Context, favoriteID, tagID int64) error {
			if s.failAssign != nil {
				return s.failAssign
			}
			s.assigned = append(s.assigned, [2]int64{favoriteID, tagID})
			return nil
		}).Maybe()
	svc.EXPECT().RemoveTag(mock.Anything, mock.Anything, mock.Anything).RunAndReturn(
		func(_ context.Context, favoriteID, tagID int64) error {
			s.removed = append(s.removed, [2]int64{favoriteID, tagID})
			return nil
		}).Maybe()
}
