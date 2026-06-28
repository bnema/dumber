package component

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/bnema/dumber/internal/application/dto"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/puregotk/v4/gdk"
	"github.com/bnema/puregotk/v4/glib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeFavoritesSidebarUC struct {
	favorites []*entity.Favorite
	tags      []*entity.Tag
}

func (f *fakeFavoritesSidebarUC) GetAll(context.Context) ([]*entity.Favorite, error) {
	return f.favorites, nil
}
func (f *fakeFavoritesSidebarUC) GetAllTags(context.Context) ([]*entity.Tag, error) {
	return f.tags, nil
}
func (f *fakeFavoritesSidebarUC) AddFavorite(context.Context, dto.FavoriteCreateInput) (*entity.Favorite, error) {
	return nil, nil
}
func (f *fakeFavoritesSidebarUC) UpdateFavorite(context.Context, dto.FavoriteUpdateInput) (*entity.Favorite, error) {
	return nil, nil
}
func (f *fakeFavoritesSidebarUC) DeleteFavorite(context.Context, entity.FavoriteID) error { return nil }
func (f *fakeFavoritesSidebarUC) SetShortcut(context.Context, entity.FavoriteID, *int) error {
	return nil
}
func (f *fakeFavoritesSidebarUC) TagFavorite(context.Context, entity.FavoriteID, entity.TagID) error {
	return nil
}
func (f *fakeFavoritesSidebarUC) UntagFavorite(context.Context, entity.FavoriteID, entity.TagID) error {
	return nil
}

func newFavoritesSidebarHarness(favorites []*entity.Favorite, tags []*entity.Tag) *FavoritesSidebar {
	fs := &FavoritesSidebar{
		favoritesUC:    &fakeFavoritesSidebarUC{favorites: favorites, tags: tags},
		selectedTagIDs: make(map[entity.TagID]struct{}),
		ctx:            context.Background(),
		idleScheduler: func(cb glib.SourceFunc) {
			cb(0)
		},
	}
	return fs
}

func TestFavoritesSidebarInitialLoadRenderingModelBehavior(t *testing.T) {
	fav := &entity.Favorite{ID: 1, URL: "https://go.dev", Title: "Go", Position: 2}
	fs := newFavoritesSidebarHarness([]*entity.Favorite{fav}, []*entity.Tag{{ID: 10, Name: "dev"}})
	fs.loadGen = 1

	applied := fs.applyLoadedData(fs.favoritesUC.(*fakeFavoritesSidebarUC).favorites, fs.favoritesUC.(*fakeFavoritesSidebarUC).tags, 1, nil)

	require.True(t, applied)
	assert.Len(t, fs.allFavorites, 1)
	assert.Len(t, fs.allTags, 1)
	require.Len(t, fs.displayRows, 1)
	assert.Equal(t, entity.FavoriteID(1), fs.displayRows[0].FavoriteID)
	assert.Equal(t, "https://go.dev", fs.displayRows[0].URL)
}

func TestFavoritesSidebarSearchFilteringByTitleURLAndTag(t *testing.T) {
	devTag := entity.Tag{ID: 1, Name: "Dev"}
	newsTag := entity.Tag{ID: 2, Name: "News"}
	fs := newFavoritesSidebarHarness([]*entity.Favorite{
		{ID: 1, URL: "https://go.dev/doc", Title: "Golang Docs", Tags: []entity.Tag{devTag}, Position: 1},
		{ID: 2, URL: "https://example.com/news", Title: "Daily", Tags: []entity.Tag{newsTag}, Position: 2},
	}, nil)
	fs.allFavorites = fs.favoritesUC.(*fakeFavoritesSidebarUC).favorites

	fs.currentQuery = "golang"
	fs.rebuildDisplayRowsLocked()
	require.Len(t, fs.displayRows, 1)
	assert.Equal(t, entity.FavoriteID(1), fs.displayRows[0].FavoriteID)

	fs.currentQuery = "example.com"
	fs.rebuildDisplayRowsLocked()
	require.Len(t, fs.displayRows, 1)
	assert.Equal(t, entity.FavoriteID(2), fs.displayRows[0].FavoriteID)

	fs.currentQuery = "dev"
	fs.rebuildDisplayRowsLocked()
	require.Len(t, fs.displayRows, 1)
	assert.Equal(t, entity.FavoriteID(1), fs.displayRows[0].FavoriteID)
}

func TestFavoritesSidebarTagFilteringMatchAny(t *testing.T) {
	fs := newFavoritesSidebarHarness([]*entity.Favorite{
		{ID: 1, URL: "https://a.test", Title: "A", Tags: []entity.Tag{{ID: 1, Name: "one"}}},
		{ID: 2, URL: "https://b.test", Title: "B", Tags: []entity.Tag{{ID: 2, Name: "two"}}},
		{ID: 3, URL: "https://c.test", Title: "C", Tags: []entity.Tag{{ID: 3, Name: "three"}}},
	}, nil)
	fs.allFavorites = fs.favoritesUC.(*fakeFavoritesSidebarUC).favorites
	fs.selectedTagIDs[1] = struct{}{}
	fs.selectedTagIDs[2] = struct{}{}

	fs.rebuildDisplayRowsLocked()

	require.Len(t, fs.displayRows, 2)
	assert.Equal(t, entity.FavoriteID(1), fs.displayRows[0].FavoriteID)
	assert.Equal(t, entity.FavoriteID(2), fs.displayRows[1].FavoriteID)
}

func TestFavoritesSidebarActivationCallbacks(t *testing.T) {
	fs := newFavoritesSidebarHarness(nil, nil)
	fs.ctx = context.Background()
	fs.idleScheduler = func(cb glib.SourceFunc) { cb(0) }
	var mu sync.Mutex
	called := []string{}
	fs.onNavigate = func(_ context.Context, url string) error {
		mu.Lock()
		defer mu.Unlock()
		called = append(called, "nav:"+url)
		return nil
	}
	fs.onNavigateKeepOpen = func(_ context.Context, url string) error {
		mu.Lock()
		defer mu.Unlock()
		called = append(called, "keep:"+url)
		return nil
	}
	fs.onOpenInNewPane = func(_ context.Context, url string) error {
		mu.Lock()
		defer mu.Unlock()
		called = append(called, "pane:"+url)
		return nil
	}

	fs.navigateToURL("https://a.test")
	fs.navigateWithoutClosing("https://b.test")
	fs.navigateToNewPane("https://c.test")

	assert.Equal(t, []string{"nav:https://a.test", "keep:https://b.test", "pane:https://c.test"}, called)
}

func TestFavoritesSidebarStaleLoadRejected(t *testing.T) {
	fs := newFavoritesSidebarHarness(nil, nil)
	fs.loadGen = 2

	applied := fs.applyLoadedData([]*entity.Favorite{{ID: 1, URL: "https://stale.test"}}, nil, 1, nil)

	assert.False(t, applied)
	assert.Empty(t, fs.allFavorites)
	assert.Empty(t, fs.displayRows)
}

func TestFavoritesSidebarDestroyedCallbacksNoOp(t *testing.T) {
	fs := newFavoritesSidebarHarness(nil, nil)
	fs.idleScheduler = func(cb glib.SourceFunc) { cb(0) }
	called := false
	fs.onNavigate = func(context.Context, string) error {
		called = true
		return nil
	}
	fs.destroyed = true

	fs.navigateToURL("https://example.test")
	fs.toggleTag(1)
	fs.onSearchChanged()

	assert.False(t, called)
	assert.Empty(t, fs.selectedTagIDs)
}

func TestFavoritesSidebarNavigationErrorSetsNotice(t *testing.T) {
	fs := newFavoritesSidebarHarness(nil, nil)
	fs.idleScheduler = func(cb glib.SourceFunc) { cb(0) }
	fs.onNavigate = func(context.Context, string) error { return errors.New("navigation failed") }

	fs.navigateToURL("https://example.test")

	assert.Equal(t, "navigation failed", fs.notice)
}

func TestFavoritesSidebarRenderingModelTagCallbackRetentionAndListRows(t *testing.T) {
	fs := newFavoritesSidebarHarness(nil, nil)
	fs.allTags = []*entity.Tag{{ID: 1, Name: "dev"}, {ID: 2, Name: "docs"}}
	fs.displayRows = buildFavoriteSidebarDisplayRows([]*entity.Favorite{
		{ID: 1, URL: "https://go.dev", Title: "Go"},
		{ID: 2, URL: "", Title: "Missing URL"},
	})

	assert.Equal(t, 2, len(fs.allTags))
	require.Len(t, fs.displayRows, 2)
	assert.True(t, fs.displayRows[0].Selectable)
	assert.False(t, fs.displayRows[1].Selectable)
	fs.tagCallbacks = []interface{}{func() {}, func() {}}
	fs.tagCallbacks = nil
	assert.Empty(t, fs.tagCallbacks)
}

func TestFavoritesSidebarSlashFocusBehavior(t *testing.T) {
	assert.True(t, shouldFocusSearchForSlash(false))
	assert.False(t, shouldFocusSearchForSlash(true))
}

func TestFavoritesSidebarHandleEnterKeyDispatchesModifiers(t *testing.T) {
	fs := newFavoritesSidebarHarness(nil, nil)
	fs.idleScheduler = func(cb glib.SourceFunc) { cb(0) }
	fs.displayRows = []favoriteSidebarDisplayRow{{FavoriteID: 1, URL: "https://go.dev", Selectable: true}}
	var called []string
	fs.onNavigate = func(_ context.Context, url string) error {
		called = append(called, "nav:"+url)
		return nil
	}
	fs.onNavigateKeepOpen = func(_ context.Context, url string) error {
		called = append(called, "keep:"+url)
		return nil
	}
	fs.onOpenInNewPane = func(_ context.Context, url string) error {
		called = append(called, "pane:"+url)
		return nil
	}

	assert.True(t, fs.handleEnterKey(0))
	assert.True(t, fs.handleEnterKey(gdk.ControlMaskValue))
	assert.True(t, fs.handleEnterKey(gdk.ShiftMaskValue))

	assert.Equal(t, []string{"nav:https://go.dev", "keep:https://go.dev", "pane:https://go.dev"}, called)
}
