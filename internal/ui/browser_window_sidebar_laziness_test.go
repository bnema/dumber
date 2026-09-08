package ui

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/bnema/dumber/internal/application/dto"
	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/ui/component"
	"github.com/bnema/dumber/internal/ui/window"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWindowCreation_DoesNotConstructHiddenSidebars(t *testing.T) {
	ctx := context.Background()
	app := &App{
		deps: &Dependencies{
			HistoryUC:   usecase.NewSearchHistoryUseCase(nil),
			FavoritesUC: usecase.NewManageFavoritesUseCase(nil, nil),
		},
		browserWindows: map[string]*browserWindow{},
	}
	app.browserWindowFactory = func(ctx context.Context, url string) (*browserWindow, error) {
		return &browserWindow{id: "w-1", initialURL: url, mainWindow: &window.MainWindow{}}, nil
	}

	created, err := app.createBrowserWindow(ctx, "https://example.com")
	require.NoError(t, err)
	require.NotNil(t, created)
	assert.Nil(t, created.historySidebar, "window creation must not construct the hidden history panel")
	assert.Nil(t, created.favoritesSidebar, "window creation must not construct the hidden favorites panel")
	assert.False(t, created.sidebarVisible)
	assert.Equal(t, nativeSidebarNone, created.activeSidebarKind)
}

func TestEnsureSidebars_ConstructOnceAndReuse(t *testing.T) {
	ctx := context.Background()
	var historyConstructions atomic.Int64
	var favoritesConstructions atomic.Int64
	releaseHistorySeam := stubHistorySidebarConstructor(&historyConstructions)
	defer releaseHistorySeam()
	releaseFavoritesSeam := stubFavoritesSidebarConstructor(&favoritesConstructions)
	defer releaseFavoritesSeam()

	app := &App{
		deps: &Dependencies{
			HistoryUC:   usecase.NewSearchHistoryUseCase(nil),
			FavoritesUC: usecase.NewManageFavoritesUseCase(nil, nil),
		},
		browserWindows: map[string]*browserWindow{},
	}
	bw := &browserWindow{id: "w-1", mainWindow: &window.MainWindow{}}
	app.browserWindows[bw.id] = bw

	require.True(t, bw.ensureHistorySidebar(ctx, app))
	require.True(t, bw.ensureFavoritesSidebar(ctx, app))
	assert.EqualValues(t, 1, historyConstructions.Load())
	assert.EqualValues(t, 1, favoritesConstructions.Load())

	// Reopening resolves the same instance without reconstructing.
	firstHistory := bw.historySidebar
	firstFavorites := bw.favoritesSidebar
	require.True(t, bw.ensureHistorySidebar(ctx, app))
	require.True(t, bw.ensureFavoritesSidebar(ctx, app))
	assert.Same(t, firstHistory, bw.historySidebar)
	assert.Same(t, firstFavorites, bw.favoritesSidebar)
	assert.EqualValues(t, 1, historyConstructions.Load())
	assert.EqualValues(t, 1, favoritesConstructions.Load())
}

func TestEnsureSidebars_UnavailableWithoutDeps(t *testing.T) {
	ctx := context.Background()
	bw := &browserWindow{id: "w-1", mainWindow: &window.MainWindow{}}
	assert.False(t, bw.ensureHistorySidebar(ctx, &App{}))
	assert.False(t, bw.ensureFavoritesSidebar(ctx, &App{}))
	assert.False(t, (&browserWindow{}).ensureHistorySidebar(ctx, &App{deps: &Dependencies{}}))
	assert.False(t, (&browserWindow{}).ensureFavoritesSidebar(ctx, &App{deps: &Dependencies{}}))
	assert.False(t, (*browserWindow)(nil).ensureHistorySidebar(ctx, &App{}))
	assert.False(t, (*browserWindow)(nil).ensureFavoritesSidebar(ctx, &App{}))
}

func TestToggleActions_EnsureOnDemandAndHideWithoutConstruction(t *testing.T) {
	ctx := context.Background()
	var historyConstructions atomic.Int64
	releaseHistorySeam := stubHistorySidebarConstructor(&historyConstructions)
	defer releaseHistorySeam()

	// Close-before-open does nothing and constructs nothing.
	quiet := &browserWindow{id: "quiet", mainWindow: &window.MainWindow{}}
	quiet.hideHistorySidebar()
	quiet.hideFavoritesSidebar()
	quiet.toggleHistorySidebar()
	quiet.toggleFavoritesSidebar()
	assert.False(t, quiet.sidebarVisible)
	assert.Zero(t, historyConstructions.Load())

	// Keyboard shortcut constructs on first open.
	app := &App{
		deps:                &Dependencies{HistoryUC: usecase.NewSearchHistoryUseCase(nil)},
		browserWindows:      map[string]*browserWindow{quiet.id: quiet},
		lastFocusedWindowID: quiet.id,
	}
	require.NoError(t, app.toggleHistorySidebarAction(ctx))
	assert.EqualValues(t, 1, historyConstructions.Load())
	assert.True(t, quiet.sidebarVisible)
	assert.Equal(t, nativeSidebarHistory, quiet.activeSidebarKind)

	// Closing the visible panel hides without further construction.
	require.NoError(t, app.toggleHistorySidebarAction(ctx))
	assert.False(t, quiet.sidebarVisible)
	assert.Equal(t, nativeSidebarNone, quiet.activeSidebarKind)
	assert.EqualValues(t, 1, historyConstructions.Load())

	// Reopening reuses the retained instance.
	require.NoError(t, app.toggleHistorySidebarAction(ctx))
	assert.True(t, quiet.sidebarVisible)
	assert.EqualValues(t, 1, historyConstructions.Load())
}

func TestToggleActions_ErrorWhenUnavailable(t *testing.T) {
	ctx := context.Background()
	app := &App{browserWindows: map[string]*browserWindow{"w": {id: "w"}}, lastFocusedWindowID: "w"}
	require.ErrorContains(t, app.toggleHistorySidebarAction(ctx), "history sidebar unavailable")
	require.ErrorContains(t, app.toggleFavoritesSidebarAction(ctx), "favorites sidebar unavailable")

	empty := &App{}
	require.ErrorContains(t, empty.toggleHistorySidebarAction(ctx), "no focused browser window")
	require.ErrorContains(t, empty.toggleFavoritesSidebarAction(ctx), "no focused browser window")
}

func TestSidebarConfigReload_DoesNotConstructPanels(t *testing.T) {
	ctx := context.Background()
	bw := &browserWindow{id: "w-1", mainWindow: &window.MainWindow{}}
	app := &App{
		deps: &Dependencies{
			HistoryUC:   usecase.NewSearchHistoryUseCase(nil),
			FavoritesUC: usecase.NewManageFavoritesUseCase(nil, nil),
		},
		browserWindows:      map[string]*browserWindow{bw.id: bw},
		lastFocusedWindowID: bw.id,
	}
	app.runtimeConfig = newRuntimeConfigState(nil)

	app.applyRuntimeConfigChange(ctx, app.runtimeConfigSnapshot())
	assert.Nil(t, bw.historySidebar)
	assert.Nil(t, bw.favoritesSidebar)

	app.reloadVisibleFavoritesSidebars("config-reload")
	app.refreshVisibleHistorySidebars(dto.HistoryChange{})
	assert.Nil(t, bw.historySidebar)
	assert.Nil(t, bw.favoritesSidebar)
}

func TestEnsureSidebars_MultipleWindowsIndependent(t *testing.T) {
	ctx := context.Background()
	var historyConstructions atomic.Int64
	releaseHistorySeam := stubHistorySidebarConstructor(&historyConstructions)
	defer releaseHistorySeam()

	app := &App{
		deps:           &Dependencies{HistoryUC: usecase.NewSearchHistoryUseCase(nil)},
		browserWindows: map[string]*browserWindow{},
	}
	first := &browserWindow{id: "w-1", mainWindow: &window.MainWindow{}}
	second := &browserWindow{id: "w-2", mainWindow: &window.MainWindow{}}
	app.browserWindows[first.id] = first
	app.browserWindows[second.id] = second

	require.True(t, first.ensureHistorySidebar(ctx, app))
	assert.NotNil(t, first.historySidebar)
	assert.Nil(t, second.historySidebar)
	assert.EqualValues(t, 1, historyConstructions.Load())
}

func stubHistorySidebarConstructor(calls *atomic.Int64) func() {
	previous := newHistorySidebarComponent
	newHistorySidebarComponent = func(ctx context.Context, cfg component.HistorySidebarConfig) *component.HistorySidebar {
		calls.Add(1)
		return &component.HistorySidebar{}
	}
	return func() { newHistorySidebarComponent = previous }
}

func stubFavoritesSidebarConstructor(calls *atomic.Int64) func() {
	previous := newFavoritesSidebarComponent
	newFavoritesSidebarComponent = func(ctx context.Context, cfg component.FavoritesSidebarConfig) *component.FavoritesSidebar {
		calls.Add(1)
		return &component.FavoritesSidebar{}
	}
	return func() { newFavoritesSidebarComponent = previous }
}
