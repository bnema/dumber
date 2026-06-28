package ui

import (
	"context"
	"errors"
	"testing"

	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/ui/component"
	contentcoord "github.com/bnema/dumber/internal/ui/coordinator/content"
	"github.com/bnema/dumber/internal/ui/window"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBrowserWindow_FavoritesSidebarToggle_CoexistsWithHistorySidebar(t *testing.T) {
	bw := &browserWindow{
		id:               "test-window",
		mainWindow:       &window.MainWindow{},
		historySidebar:   &component.HistorySidebar{},
		favoritesSidebar: &component.FavoritesSidebar{},
	}

	bw.showHistorySidebar()
	assert.True(t, bw.sidebarVisible)
	assert.Equal(t, nativeSidebarHistory, bw.activeSidebarKind)

	bw.toggleFavoritesSidebar()
	assert.True(t, bw.sidebarVisible)
	assert.Equal(t, nativeSidebarFavorites, bw.activeSidebarKind)

	bw.toggleHistorySidebar()
	assert.True(t, bw.sidebarVisible)
	assert.Equal(t, nativeSidebarHistory, bw.activeSidebarKind)

	bw.toggleHistorySidebar()
	assert.False(t, bw.sidebarVisible)
	assert.Equal(t, nativeSidebarNone, bw.activeSidebarKind)
}

func TestApp_ToggleFavoritesSidebarActionHidesActiveFavoritesSidebar(t *testing.T) {
	bw := &browserWindow{
		id:                "w",
		mainWindow:        &window.MainWindow{},
		favoritesSidebar:  &component.FavoritesSidebar{},
		sidebarVisible:    true,
		activeSidebarKind: nativeSidebarFavorites,
	}
	app := &App{browserWindows: map[string]*browserWindow{bw.id: bw}, lastFocusedWindowID: bw.id}

	require.NoError(t, app.toggleFavoritesSidebarAction(context.Background()))

	assert.False(t, bw.sidebarVisible)
	assert.Equal(t, nativeSidebarNone, bw.activeSidebarKind)
}

func TestFavoritesSidebarConfigOnCloseHidesFavoritesSidebar(t *testing.T) {
	bw := &browserWindow{
		id:                "w",
		mainWindow:        &window.MainWindow{},
		favoritesSidebar:  &component.FavoritesSidebar{},
		sidebarVisible:    true,
		activeSidebarKind: nativeSidebarFavorites,
	}
	app := &App{browserWindows: map[string]*browserWindow{bw.id: bw}, lastFocusedWindowID: bw.id}

	cfg := app.buildFavoritesSidebarConfig(bw)
	require.NotNil(t, cfg.OnClose)
	cfg.OnClose()

	assert.False(t, bw.sidebarVisible)
	assert.Equal(t, nativeSidebarNone, bw.activeSidebarKind)
}

func TestApp_ToggleFavoritesSidebarActionErrorsWhenUnavailable(t *testing.T) {
	app := &App{browserWindows: map[string]*browserWindow{"w": {id: "w"}}, lastFocusedWindowID: "w"}
	err := app.toggleFavoritesSidebarAction(context.Background())
	require.Error(t, err)
	assert.ErrorContains(t, err, "favorites sidebar unavailable")
}

func TestApp_ToggleCurrentPageFavoriteTogglesActiveWebViewURI(t *testing.T) {
	ctx := context.Background()
	tab := entity.NewTab(entity.TabID("tab-1"), entity.WorkspaceID("ws-1"), entity.NewPane(entity.PaneID("pane-1")))
	tabs := entity.NewTabList()
	tabs.Add(tab)
	tabs.SetActive(tab.ID)
	bw := &browserWindow{id: "w", tabs: tabs}

	contentCoord := contentcoord.NewCoordinator(ctx, nil, nil, nil, nil, nil, nil, nil)
	wv := &recordingWebView{id: 1, loadURILastURI: "https://example.com/page"}
	contentCoord.RegisterPopupWebView(entity.PaneID("pane-1"), wv)

	favRepo := newMemoryFavoriteRepo()
	app := &App{
		deps:                &Dependencies{FavoritesUC: usecase.NewManageFavoritesUseCase(favRepo, &memoryTagRepo{})},
		browserWindows:      map[string]*browserWindow{bw.id: bw},
		lastFocusedWindowID: bw.id,
		contentCoord:        contentCoord,
		workspaceViews:      map[entity.TabID]*component.WorkspaceView{tab.ID: {}},
	}

	require.NoError(t, app.toggleCurrentPageFavoriteAction(ctx))
	assert.Len(t, favRepo.byURL, 1)
	_, ok := favRepo.byURL["https://example.com/page"]
	assert.True(t, ok)

	require.NoError(t, app.toggleCurrentPageFavoriteAction(ctx))
	assert.Empty(t, favRepo.byURL)
}

func TestApp_ToggleCurrentPageFavoriteErrorCases(t *testing.T) {
	ctx := context.Background()
	assert.ErrorContains(t, (&App{}).toggleCurrentPageFavoriteAction(ctx), "usecase not configured")

	app := &App{deps: &Dependencies{FavoritesUC: usecase.NewManageFavoritesUseCase(newMemoryFavoriteRepo(), &memoryTagRepo{})}}
	assert.ErrorContains(t, app.toggleCurrentPageFavoriteAction(ctx), "no active webview")

	tab := entity.NewTab(entity.TabID("tab-1"), entity.WorkspaceID("ws-1"), entity.NewPane(entity.PaneID("pane-1")))
	tabs := entity.NewTabList()
	tabs.Add(tab)
	tabs.SetActive(tab.ID)
	bw := &browserWindow{id: "w", tabs: tabs}
	contentCoord := contentcoord.NewCoordinator(ctx, nil, nil, nil, nil, nil, nil, nil)
	contentCoord.RegisterPopupWebView(entity.PaneID("pane-1"), &recordingWebView{id: 2})
	app = &App{
		deps:                &Dependencies{FavoritesUC: usecase.NewManageFavoritesUseCase(newMemoryFavoriteRepo(), &memoryTagRepo{})},
		browserWindows:      map[string]*browserWindow{bw.id: bw},
		lastFocusedWindowID: bw.id,
		contentCoord:        contentCoord,
		workspaceViews:      map[entity.TabID]*component.WorkspaceView{tab.ID: {}},
	}
	assert.ErrorContains(t, app.toggleCurrentPageFavoriteAction(ctx), "active page has no URI")
}

type memoryFavoriteRepo struct {
	nextID entity.FavoriteID
	byID   map[entity.FavoriteID]*entity.Favorite
	byURL  map[string]*entity.Favorite
	err    error
}

func newMemoryFavoriteRepo() *memoryFavoriteRepo {
	return &memoryFavoriteRepo{nextID: 1, byID: map[entity.FavoriteID]*entity.Favorite{}, byURL: map[string]*entity.Favorite{}}
}

func (r *memoryFavoriteRepo) Save(_ context.Context, fav *entity.Favorite) error {
	if r.err != nil {
		return r.err
	}
	if fav.ID == 0 {
		fav.ID = r.nextID
		r.nextID++
	}
	cp := *fav
	r.byID[fav.ID] = &cp
	r.byURL[fav.URL] = &cp
	return nil
}
func (r *memoryFavoriteRepo) FindByID(context.Context, entity.FavoriteID) (*entity.Favorite, error) {
	return nil, r.err
}
func (r *memoryFavoriteRepo) FindByURL(_ context.Context, url string) (*entity.Favorite, error) {
	return r.byURL[url], r.err
}
func (r *memoryFavoriteRepo) GetAll(context.Context) ([]*entity.Favorite, error) { return nil, r.err }
func (r *memoryFavoriteRepo) GetByTag(context.Context, entity.TagID) ([]*entity.Favorite, error) {
	return nil, r.err
}
func (r *memoryFavoriteRepo) GetByShortcut(context.Context, int) (*entity.Favorite, error) {
	return nil, r.err
}
func (r *memoryFavoriteRepo) UpdatePosition(context.Context, entity.FavoriteID, int) error {
	return r.err
}
func (r *memoryFavoriteRepo) SetShortcut(context.Context, entity.FavoriteID, *int) error {
	return r.err
}
func (r *memoryFavoriteRepo) Delete(_ context.Context, id entity.FavoriteID) error {
	if r.err != nil {
		return r.err
	}
	fav := r.byID[id]
	if fav == nil {
		return errors.New("favorite not found")
	}
	delete(r.byID, id)
	delete(r.byURL, fav.URL)
	return nil
}

type memoryTagRepo struct{}

func (*memoryTagRepo) Save(context.Context, *entity.Tag) error                     { return nil }
func (*memoryTagRepo) FindByID(context.Context, entity.TagID) (*entity.Tag, error) { return nil, nil }
func (*memoryTagRepo) FindByName(context.Context, string) (*entity.Tag, error)     { return nil, nil }
func (*memoryTagRepo) GetAll(context.Context) ([]*entity.Tag, error)               { return nil, nil }
func (*memoryTagRepo) AssignToFavorite(context.Context, entity.TagID, entity.FavoriteID) error {
	return nil
}
func (*memoryTagRepo) RemoveFromFavorite(context.Context, entity.TagID, entity.FavoriteID) error {
	return nil
}
func (*memoryTagRepo) GetForFavorite(context.Context, entity.FavoriteID) ([]*entity.Tag, error) {
	return nil, nil
}
func (*memoryTagRepo) Delete(context.Context, entity.TagID) error { return nil }
