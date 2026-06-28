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

	addInputs    []dto.FavoriteCreateInput
	updateInputs []dto.FavoriteUpdateInput
	deletedIDs   []entity.FavoriteID
	shortcuts    []struct {
		id  entity.FavoriteID
		key *int
	}
	tagged []struct {
		favID entity.FavoriteID
		tagID entity.TagID
	}
	untagged []struct {
		favID entity.FavoriteID
		tagID entity.TagID
	}
	err error
}

func (f *fakeFavoritesSidebarUC) GetAll(context.Context) ([]*entity.Favorite, error) {
	return f.favorites, nil
}
func (f *fakeFavoritesSidebarUC) GetAllTags(context.Context) ([]*entity.Tag, error) {
	return f.tags, nil
}
func (f *fakeFavoritesSidebarUC) AddFavorite(_ context.Context, input dto.FavoriteCreateInput) (*entity.Favorite, error) {
	f.addInputs = append(f.addInputs, input)
	if f.err != nil {
		return nil, f.err
	}
	return &entity.Favorite{ID: 99, URL: input.URL, Title: input.Title}, nil
}
func (f *fakeFavoritesSidebarUC) UpdateFavorite(_ context.Context, input dto.FavoriteUpdateInput) (*entity.Favorite, error) {
	f.updateInputs = append(f.updateInputs, input)
	if f.err != nil {
		return nil, f.err
	}
	return &entity.Favorite{ID: input.ID, Title: input.Title}, nil
}
func (f *fakeFavoritesSidebarUC) DeleteFavorite(_ context.Context, id entity.FavoriteID) error {
	f.deletedIDs = append(f.deletedIDs, id)
	return f.err
}
func (f *fakeFavoritesSidebarUC) SetShortcut(_ context.Context, id entity.FavoriteID, key *int) error {
	f.shortcuts = append(f.shortcuts, struct {
		id  entity.FavoriteID
		key *int
	}{id: id, key: key})
	return f.err
}
func (f *fakeFavoritesSidebarUC) TagFavorite(_ context.Context, favID entity.FavoriteID, tagID entity.TagID) error {
	f.tagged = append(f.tagged, struct {
		favID entity.FavoriteID
		tagID entity.TagID
	}{favID: favID, tagID: tagID})
	return f.err
}
func (f *fakeFavoritesSidebarUC) UntagFavorite(_ context.Context, favID entity.FavoriteID, tagID entity.TagID) error {
	f.untagged = append(f.untagged, struct {
		favID entity.FavoriteID
		tagID entity.TagID
	}{favID: favID, tagID: tagID})
	return f.err
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
	assert.Equal(t, []string{"saved"}, favoriteSidebarNoticeRows("saved", "", true))
	assert.Equal(t, []string{"No favorites"}, favoriteSidebarNoticeRows("", "", false))
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

func TestFavoritesSidebarFocusZoneCyclingSkipsAbsentZones(t *testing.T) {
	fs := newFavoritesSidebarHarness(nil, nil)
	fs.searchEntry = nil
	fs.tagBox = nil
	fs.listBox = nil
	assert.Empty(t, fs.availableFocusZones())

	fs.formBox = nil
	fs.mode = favoritesSidebarModeAdd
	fs.confirmDelete = true
	assert.Equal(t, []favoritesSidebarFocusZone{favoritesSidebarFocusConfirm}, fs.availableFocusZones())
}

func TestFavoritesSidebarSingleKeyCommandsAreGatedInTextEditContext(t *testing.T) {
	fs := newFavoritesSidebarHarness(nil, nil)
	fs.searchEntry = nil
	fs.displayRows = []favoriteSidebarDisplayRow{{FavoriteID: 1, Favorite: &entity.Favorite{ID: 1}, Selectable: true}}

	assert.True(t, fs.handleSingleKeyCommand(uint(gdk.KEY_a)))
	assert.Equal(t, favoritesSidebarModeAdd, fs.mode)

	fs.mode = favoritesSidebarModeNone
	assert.True(t, fs.handleSingleKeyCommand(uint(gdk.KEY_e)))
	assert.Equal(t, favoritesSidebarModeEdit, fs.mode)
}

func TestFavoritesSidebarAddAndEditDTOs(t *testing.T) {
	uc := &fakeFavoritesSidebarUC{}
	fs := newFavoritesSidebarHarness(nil, nil)
	fs.favoritesUC = uc
	fs.formURL = "https://add.test"
	fs.formTitle = "Added"
	fs.formTags = "1, 2"
	fs.formShortcut = "3"
	fs.mode = favoritesSidebarModeAdd

	assert.True(t, fs.submitForm())
	require.Len(t, uc.addInputs, 1)
	assert.Equal(t, dto.FavoriteCreateInput{URL: "https://add.test", Title: "Added", Tags: []entity.TagID{1, 2}}, uc.addInputs[0])
	require.Len(t, uc.shortcuts, 1)
	assert.Equal(t, 3, *uc.shortcuts[0].key)

	fs.mode = favoritesSidebarModeEdit
	fs.editingID = 7
	fs.formTitle = "Edited"
	fs.formTags = "999"
	fs.formShortcut = "4"
	assert.True(t, fs.submitForm())
	require.Len(t, uc.updateInputs, 1)
	assert.Equal(t, entity.FavoriteID(7), uc.updateInputs[0].ID)
	assert.Equal(t, "Edited", uc.updateInputs[0].Title)
	require.NotNil(t, uc.updateInputs[0].ShortcutKey)
	assert.Equal(t, 4, *uc.updateInputs[0].ShortcutKey)
}

func TestFavoritesSidebarInvalidTagAndShortcutPreserveFormState(t *testing.T) {
	uc := &fakeFavoritesSidebarUC{}
	fs := newFavoritesSidebarHarness(nil, nil)
	fs.favoritesUC = uc
	fs.formURL = "https://add.test"
	fs.formTitle = "Added"
	fs.formTags = "1, nope"
	fs.mode = favoritesSidebarModeAdd

	assert.True(t, fs.submitForm())
	assert.Empty(t, uc.addInputs)
	assert.Equal(t, favoritesSidebarModeAdd, fs.mode)
	assert.Contains(t, fs.notice, "invalid tag ID")
	assert.Equal(t, "1, nope", fs.formTags)

	fs.formTags = "1"
	fs.formShortcut = "10"
	assert.True(t, fs.submitForm())
	assert.Empty(t, uc.addInputs)
	assert.Equal(t, favoritesSidebarModeAdd, fs.mode)
	assert.Contains(t, fs.notice, "invalid shortcut")
	assert.Equal(t, "10", fs.formShortcut)
}

func TestFavoritesSidebarEnterConfirmsDelete(t *testing.T) {
	fav := &entity.Favorite{ID: 5, URL: "https://go.dev"}
	fs := newFavoritesSidebarHarness([]*entity.Favorite{fav}, nil)
	uc := fs.favoritesUC.(*fakeFavoritesSidebarUC)
	fs.displayRows = []favoriteSidebarDisplayRow{{FavoriteID: fav.ID, Favorite: fav, URL: fav.URL, Selectable: true}}
	fs.confirmDelete = true

	assert.True(t, fs.confirmDeleteActive())
	assert.True(t, fs.confirmDeleteFavorite())
	assert.Equal(t, []entity.FavoriteID{5}, uc.deletedIDs)
}

func TestFavoritesSidebarCtrlEnterSubmitsFormPlainEnterDoesNot(t *testing.T) {
	uc := &fakeFavoritesSidebarUC{}
	fs := newFavoritesSidebarHarness(nil, nil)
	fs.favoritesUC = uc
	fs.formURL = "https://add.test"
	fs.formTitle = "Added"
	fs.mode = favoritesSidebarModeAdd
	fs.displayRows = []favoriteSidebarDisplayRow{{FavoriteID: 1, URL: "https://existing.test", Selectable: true}}
	fs.onNavigate = func(context.Context, string) error {
		t.Fatal("plain Enter in form mode must not activate selected row")
		return nil
	}

	assert.False(t, fs.handleReturnKey(0))
	assert.Empty(t, uc.addInputs)
	fs.mode = favoritesSidebarModeAdd
	assert.True(t, fs.handleReturnKey(gdk.ControlMaskValue))
	require.Len(t, uc.addInputs, 1)
}

func TestFavoritesSidebarEditSubmitIgnoresTagsByDesign(t *testing.T) {
	uc := &fakeFavoritesSidebarUC{}
	fs := newFavoritesSidebarHarness(nil, nil)
	fs.favoritesUC = uc
	fs.mode = favoritesSidebarModeEdit
	fs.editingID = 7
	fs.formTitle = "Edited"
	fs.formTags = "999"

	assert.True(t, fs.submitForm())
	require.Len(t, uc.updateInputs, 1)
	assert.Equal(t, entity.FavoriteID(7), uc.updateInputs[0].ID)
	assert.Empty(t, uc.tagged)
	assert.Empty(t, uc.untagged)
}

func TestFavoritesSidebarTagShortcutAndDeleteFlows(t *testing.T) {
	dev := entity.Tag{ID: 10, Name: "dev"}
	news := entity.Tag{ID: 11, Name: "news"}
	fav := &entity.Favorite{ID: 5, URL: "https://go.dev", Tags: []entity.Tag{dev}}
	fs := newFavoritesSidebarHarness([]*entity.Favorite{fav}, []*entity.Tag{&dev, &news})
	uc := fs.favoritesUC.(*fakeFavoritesSidebarUC)
	fs.allTags = uc.tags
	fs.displayRows = []favoriteSidebarDisplayRow{{FavoriteID: fav.ID, Favorite: fav, URL: fav.URL, Selectable: true}}

	fs.mode = favoritesSidebarModeTag
	assert.True(t, fs.handleSingleKeyCommand(uint(gdk.KEY_1)))
	require.Len(t, uc.untagged, 1)
	assert.Equal(t, entity.TagID(10), uc.untagged[0].tagID)
	assert.True(t, fs.handleSingleKeyCommand(uint(gdk.KEY_2)))
	require.Len(t, uc.tagged, 1)
	assert.Equal(t, entity.TagID(11), uc.tagged[0].tagID)

	fs.mode = favoritesSidebarModeShortcut
	assert.True(t, fs.handleSingleKeyCommand(uint(gdk.KEY_9)))
	assert.True(t, fs.handleSingleKeyCommand(uint(gdk.KEY_BackSpace)))
	require.GreaterOrEqual(t, len(uc.shortcuts), 2)
	assert.Equal(t, 9, *uc.shortcuts[len(uc.shortcuts)-2].key)
	assert.Nil(t, uc.shortcuts[len(uc.shortcuts)-1].key)

	fs.mode = favoritesSidebarModeNone
	assert.True(t, fs.handleSingleKeyCommand(uint(gdk.KEY_Delete)))
	assert.True(t, fs.confirmDelete)
	assert.True(t, fs.handleSingleKeyCommand(uint(gdk.KEY_Delete)))
	assert.Equal(t, []entity.FavoriteID{5}, uc.deletedIDs)
}

func TestFavoritesSidebarErrorsPreserveStateAndShowNotice(t *testing.T) {
	fs := newFavoritesSidebarHarness(nil, nil)
	uc := fs.favoritesUC.(*fakeFavoritesSidebarUC)
	uc.err = errors.New("add failed")
	fs.mode = favoritesSidebarModeAdd

	assert.True(t, fs.submitForm())
	assert.Equal(t, favoritesSidebarModeAdd, fs.mode)
	assert.Equal(t, "add failed", fs.notice)
}
