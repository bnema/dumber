package systemviews

import (
	"context"
	"errors"
	"testing"

	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func favoritesFixture(t *testing.T, dom DOM) (*App, *favoritesStore) {
	t.Helper()
	three := 3
	favorites := newFavoritesStore(t, &favoritesStore{
		favorites: []*entity.Favorite{
			{ID: 1, URL: "https://go.dev", Title: "Go", ShortcutKey: &three, FaviconURL: "https://go.dev/icon.png",
				Tags: []entity.Tag{{ID: 7, Name: "dev"}, {ID: 8, Name: "news"}}},
			{ID: 2, URL: "https://news.example", Title: "News", Tags: []entity.Tag{{ID: 8, Name: "news"}}},
			{ID: 3, URL: "https://plain.example", Title: "Plain"},
		},
		tags: []*entity.Tag{{ID: 7, Name: "dev", Color: "#3b82f6"}, {ID: 8, Name: "news", Color: "#ef4444"}},
	})
	favorites.expectWrites()
	app := NewApp(Dependencies{DOM: dom, Favorites: favorites.service, LocationURI: "dumb://favorites"})
	require.NoError(t, app.LoadInitial(context.Background()))
	return app, favorites
}

func regionHTML(t *testing.T, regions []DOMRegion, name string) string {
	t.Helper()
	for _, region := range regions {
		if region.Name == name {
			return region.HTML
		}
	}
	t.Fatalf("region %s missing", name)
	return ""
}

func TestFavoritesRenderDataCountsAndHidesPending(t *testing.T) {
	t.Parallel()
	app, favorites := favoritesFixture(t, &recordingDOM{})
	data := app.favoritesData
	assert.Equal(t, 3, data.TotalCount)
	assert.Equal(t, 1, data.UntaggedCount)
	assert.Equal(t, 1, data.TagCounts[7])
	assert.Equal(t, 2, data.TagCounts[8])
	assert.Equal(t, entity.FavoriteID(1), data.ShortcutOwners[3].ID)

	filter := entity.TagID(8)
	data = buildFavoritesRenderData(favoritesSource{
		Favorites:       favorites.favorites,
		Tags:            app.tags,
		HiddenFavorites: map[int64]string{2: "News"},
		HiddenTags:      map[int64]string{8: "news"},
		TagFilter:       &filter,
	})
	assert.Nil(t, data.TagFilter, "a filter on a hidden tag falls back to All")
	assert.Len(t, data.Favorites, 2)
	assert.Len(t, data.Tags, 1)
	assert.Equal(t, 1, data.UntaggedCount, "Plain is untagged; Go keeps dev")
	assert.Equal(t, []entity.Tag{{ID: 7, Name: "dev"}}, data.Favorites[0].Tags)
}

func TestFavoriteSaveDiffsTagsAndKeepsAbsentFields(t *testing.T) {
	t.Parallel()
	app, favorites := favoritesFixture(t, &recordingDOM{})

	require.NoError(t, app.HandleDOMAction(context.Background(), DOMAction{
		Action: favoriteActionSave,
		Data:   map[string]string{"id": "1", "title": "Go site", "tags": ",8"},
	}))
	assert.Empty(t, app.favoritesError)
	assert.Equal(t, "https://go.dev/icon.png", favorites.updatedFavorite.FaviconURL, "absent icon keeps the stored one")
	assert.False(t, favorites.updatedFavorite.ShortcutKeySet, "absent shortcut is left alone")
	assert.Empty(t, favorites.assigned)
	assert.Equal(t, [][2]int64{{1, 7}}, favorites.removed)
}

func TestFavoriteSaveUncheckingEveryTagRemovesThem(t *testing.T) {
	t.Parallel()
	app, favorites := favoritesFixture(t, &recordingDOM{})

	// The hidden empty "tags" field is all that is sent when nothing is ticked.
	require.NoError(t, app.HandleDOMAction(context.Background(), DOMAction{
		Action: favoriteActionSave,
		Data:   map[string]string{"id": "1", "title": "Go", "tags": ""},
	}))
	assert.ElementsMatch(t, [][2]int64{{1, 7}, {1, 8}}, favorites.removed)
}

func TestFavoriteSaveMovesShortcutFromOtherFavorite(t *testing.T) {
	t.Parallel()
	app, favorites := favoritesFixture(t, &recordingDOM{})

	require.NoError(t, app.HandleDOMAction(context.Background(), DOMAction{
		Action: favoriteActionSave,
		Data:   map[string]string{"id": "2", "title": "News", "shortcut_key": "3"},
	}))
	require.Contains(t, favorites.shortcuts, int64(1))
	assert.Nil(t, favorites.shortcuts[1], "the previous owner loses the key")
	require.NotNil(t, favorites.updatedFavorite.ShortcutKey)
	assert.Equal(t, 3, *favorites.updatedFavorite.ShortcutKey)

	require.NoError(t, app.HandleDOMAction(context.Background(), DOMAction{
		Action: favoriteActionSave,
		Data:   map[string]string{"id": "2", "title": "News", "shortcut_key": ""},
	}))
	assert.True(t, favorites.updatedFavorite.ShortcutKeySet)
	assert.Nil(t, favorites.updatedFavorite.ShortcutKey, "None clears the shortcut")
}

func TestFavoriteSaveErrorKeepsOpenPanels(t *testing.T) {
	t.Parallel()
	dom := &regionDOM{}
	app, favorites := favoritesFixture(t, dom)
	mounts := dom.mountCount

	require.NoError(t, app.HandleDOMAction(context.Background(), DOMAction{
		Action: favoriteActionSave,
		Data:   map[string]string{"id": "1", "title": "  "},
	}))
	assert.Equal(t, mounts, dom.mountCount, "an error must not remount the page")
	require.Len(t, dom.regions, 1)
	require.Len(t, dom.regions[0], 1)
	assert.Contains(t, regionHTML(t, dom.regions[0], favoritesRegionAlerts), "title is required")
	assert.Zero(t, favorites.updatedFavorite.ID)
}

func TestFavoriteSaveIgnoresTagsPendingDelete(t *testing.T) {
	t.Parallel()
	app, favorites := favoritesFixture(t, &toastDOM{})

	require.NoError(t, app.HandleDOMAction(context.Background(), DOMAction{
		Action: tagActionDelete, Data: map[string]string{"id": "7", undoDataKey: "true"},
	}))
	// The panel no longer shows tag 7, so it is not in the submitted list.
	require.NoError(t, app.HandleDOMAction(context.Background(), DOMAction{
		Action: favoriteActionSave,
		Data:   map[string]string{"id": "1", "title": "Go", "tags": ",8"},
	}))
	assert.Empty(t, favorites.removed, "undo must still find the assignment")
}

func TestFavoriteFilterSwapsRegions(t *testing.T) {
	t.Parallel()
	dom := &regionDOM{}
	app, _ := favoritesFixture(t, dom)
	mounts := dom.mountCount

	require.NoError(t, app.HandleDOMAction(context.Background(), DOMAction{
		Action: favoriteActionFilterTag, Data: map[string]string{"tagId": "7"},
	}))
	assert.Equal(t, mounts, dom.mountCount)
	require.Len(t, dom.regions, 1)
	assert.True(t, dom.transitions[0])
	list := regionHTML(t, dom.regions[0], favoritesRegionList)
	assert.Contains(t, list, "Go")
	assert.NotContains(t, list, "Plain")
	assert.Contains(t, list, `name="tags" value="7"`, "new favorites get the filtered tag")
}

func TestFavoriteDeferredDeleteUndoAndCommit(t *testing.T) {
	t.Parallel()
	dom := &toastDOM{}
	app, favorites := favoritesFixture(t, dom)
	ctx := context.Background()

	require.NoError(t, app.HandleDOMAction(ctx, DOMAction{
		Action: favoriteActionDelete, Data: map[string]string{"id": "2", undoDataKey: "true"},
	}))
	assert.Zero(t, favorites.deletedFavorite)
	require.Len(t, dom.toasts, 1)
	assert.Equal(t, "Deleted “News”", dom.toasts[0].Message)
	assert.NotContains(t, regionHTML(t, dom.regions[len(dom.regions)-1], favoritesRegionList), "News")

	require.NoError(t, app.HandleDOMAction(ctx, dom.toasts[0].OnAction))
	assert.Contains(t, regionHTML(t, dom.regions[len(dom.regions)-1], favoritesRegionList), "News")
	require.NoError(t, app.HandleDOMAction(ctx, dom.toasts[0].OnDismiss))
	assert.Zero(t, favorites.deletedFavorite, "an undone delete is never committed")

	require.NoError(t, app.HandleDOMAction(ctx, DOMAction{
		Action: favoriteActionDelete, Data: map[string]string{"id": "2", undoDataKey: "true"},
	}))
	updates := len(dom.regions)
	require.NoError(t, app.HandleDOMAction(ctx, dom.toasts[1].OnDismiss))
	assert.Equal(t, int64(2), favorites.deletedFavorite)
	assert.Len(t, dom.regions, updates, "a successful commit renders nothing")
}

func TestFavoriteCommitFailureShowsError(t *testing.T) {
	t.Parallel()
	dom := &toastDOM{}
	app, favorites := favoritesFixture(t, dom)
	favorites.failDelete = errors.New("db locked")

	require.NoError(t, app.HandleDOMAction(context.Background(), DOMAction{
		Action: favoriteActionDelete, Data: map[string]string{"id": "2", undoDataKey: "true"},
	}))
	require.NoError(t, app.HandleDOMAction(context.Background(), dom.toasts[0].OnDismiss))
	assert.Contains(t, regionHTML(t, dom.regions[len(dom.regions)-1], favoritesRegionAlerts), "db locked")
}

func TestFlushDeletesCommitsPendingTagsAndFavorites(t *testing.T) {
	t.Parallel()
	dom := &toastDOM{}
	app, favorites := favoritesFixture(t, dom)
	ctx := context.Background()

	require.NoError(t, app.HandleDOMAction(ctx, DOMAction{
		Action: tagActionDelete, Data: map[string]string{"id": "8", undoDataKey: "true"},
	}))
	require.NoError(t, app.HandleDOMAction(ctx, DOMAction{
		Action: favoriteActionDelete, Data: map[string]string{"id": "3", undoDataKey: "true"},
	}))
	assert.Equal(t, "Deleted tag “news”", dom.toasts[0].Message)

	require.NoError(t, app.HandleDOMAction(ctx, DOMAction{Action: actionFlushDeletes}))
	assert.Equal(t, int64(8), favorites.deletedTagID)
	assert.Equal(t, int64(3), favorites.deletedFavorite)
}

func TestTagColorInputValidates(t *testing.T) {
	t.Parallel()
	color, err := tagColorInput(map[string]string{"color": "#3B82F6"})
	require.NoError(t, err)
	assert.Equal(t, "#3b82f6", color)

	color, err = tagColorInput(map[string]string{})
	require.NoError(t, err)
	assert.Empty(t, color)

	_, err = tagColorInput(map[string]string{"color": "red"})
	require.Error(t, err)
}

func TestNextTagColorSkipsUsedPresets(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "#ef4444", nextTagColor(nil))
	assert.Equal(t, "#f97316", nextTagColor([]*entity.Tag{{Color: "#EF4444"}}))
}

func TestFavoritesPaletteItems(t *testing.T) {
	t.Parallel()
	app, _ := favoritesFixture(t, &recordingDOM{})
	app.lockState()
	items := app.paletteItemsLocked()
	app.unlockState()

	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	assert.Contains(t, ids, "favorites.add")
	assert.Contains(t, ids, "favorites.filter")
	assert.Contains(t, ids, "favorites.newTag")
	assert.Contains(t, ids, "favorites.tag.7")
	assert.Contains(t, ids, "favorites.untagged")
}

func TestFavoriteSavePartialFailureRemounts(t *testing.T) {
	t.Parallel()
	dom := &regionDOM{}
	app, favorites := favoritesFixture(t, dom)
	favorites.failAssign = errors.New("tag gone")
	mounts := dom.mountCount

	require.NoError(t, app.HandleDOMAction(context.Background(), DOMAction{
		Action: favoriteActionSave,
		Data:   map[string]string{"id": "3", "title": "Plain", "tags": ",7"},
	}))
	assert.Equal(t, mounts+1, dom.mountCount, "a partial write re-renders from storage")
	assert.Contains(t, dom.HTML(), "saved part of the changes; tag gone")
}

func TestRepeatedFavoritesDeferredDeleteIsIgnored(t *testing.T) {
	t.Parallel()
	for _, action := range []string{favoriteActionDelete, tagActionDelete} {
		dom := &toastDOM{}
		app, _ := favoritesFixture(t, dom)
		id := "1"
		if action == tagActionDelete {
			id = "7"
		}
		event := DOMAction{Action: action, Data: map[string]string{"id": id, undoDataKey: "true"}}

		require.NoError(t, app.HandleDOMAction(context.Background(), event))
		require.NoError(t, app.HandleDOMAction(context.Background(), event))

		assert.Len(t, dom.toasts, 1, "%s: a pending id gets no second toast", action)
	}
}

func TestFavoritesCommitDeleteRejectsInvalidID(t *testing.T) {
	t.Parallel()
	app, favorites := favoritesFixture(t, &toastDOM{})

	err := app.HandleDOMAction(context.Background(), DOMAction{Action: favoriteActionCommitDelete, Data: map[string]string{"id": "0"}})
	require.Error(t, err)
	assert.Zero(t, favorites.deletedFavorite)
}
