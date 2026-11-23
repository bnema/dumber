package systemviews

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"bnema.dev/dumber/internal/domain/entity"
)

// barrierDeletes returns a delete func that records each id and blocks until
// all n deletes have started. Sequential deletes would never get past the
// first call, which synctest reports as a deadlock.
func barrierDeletes(n int, failing map[int64]bool) (func(id int64) error, func() []int64) {
	var (
		arrived sync.WaitGroup
		mu      sync.Mutex
		ids     []int64
	)
	arrived.Add(n)
	del := func(id int64) error {
		mu.Lock()
		ids = append(ids, id)
		mu.Unlock()
		arrived.Done()
		arrived.Wait()
		if failing[id] {
			return fmt.Errorf("locked %d", id)
		}
		return nil
	}
	called := func() []int64 {
		mu.Lock()
		defer mu.Unlock()
		out := slices.Clone(ids)
		slices.Sort(out)
		return out
	}
	return del, called
}

func TestFlushIssuesHistoryDeletesConcurrentlyAndJoinsErrors(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		now := time.Now()
		history := newHistoryStore(t, &historyStore{entries: []*entity.HistoryEntry{
			{ID: 3, URL: "https://c.example", Title: "Third", LastVisited: now},
			{ID: 2, URL: "https://b.example", Title: "Second", LastVisited: now.Add(-time.Minute)},
			{ID: 1, URL: "https://a.example", Title: "First", LastVisited: now.Add(-2 * time.Minute)},
		}})
		del, called := barrierDeletes(3, map[int64]bool{1: true, 3: true})
		history.service.EXPECT().DeleteEntry(mock.Anything, mock.Anything).
			RunAndReturn(func(_ context.Context, id int64) error { return del(id) }).Times(3)
		app := loadedHistoryApp(t, &toastDOM{}, history)
		ctx := context.Background()
		for _, id := range []string{"1", "2", "3"} {
			require.NoError(t, app.HandleDOMAction(ctx, deferredDelete(id)))
		}

		require.NoError(t, app.HandleDOMAction(ctx, DOMAction{Action: actionFlushDeletes}))

		assert.Equal(t, []int64{1, 2, 3}, called(), "every pending delete is issued")
		assert.Equal(t, "Could not delete history entry: locked 1; locked 3", app.historyError, "errors are joined in id order")
		assert.Contains(t, app.renderedHTML, "First", "failed deletes show the entry again")
		assert.Contains(t, app.renderedHTML, "Third")
		assert.NotContains(t, app.renderedHTML, "Second")
		assert.Empty(t, app.pendingHistoryDeletes)
	})
}

func TestFlushIssuesFavoritesAndTagDeletesConcurrentlyAndJoinsErrors(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		favorites := newFavoritesStore(t, &favoritesStore{
			favorites: []*entity.Favorite{
				{ID: 2, URL: "https://news.example", Title: "News"},
				{ID: 3, URL: "https://plain.example", Title: "Plain"},
			},
			tags: []*entity.Tag{{ID: 8, Name: "news", Color: "#ef4444"}},
		})
		del, called := barrierDeletes(3, map[int64]bool{3: true, 8: true})
		favorites.service.EXPECT().DeleteFavorite(mock.Anything, mock.Anything).
			RunAndReturn(func(_ context.Context, id int64) error { return del(id) }).Times(2)
		favorites.service.EXPECT().DeleteTag(mock.Anything, mock.Anything).
			RunAndReturn(func(_ context.Context, id int64) error { return del(id) }).Once()
		app := NewApp(Dependencies{DOM: &toastDOM{}, Favorites: favorites.service, LocationURI: "dumb://favorites"})
		ctx := context.Background()
		require.NoError(t, app.LoadInitial(ctx))
		for _, action := range []DOMAction{
			{Action: favoriteActionDelete, Data: map[string]string{"id": "2", undoDataKey: "true"}},
			{Action: favoriteActionDelete, Data: map[string]string{"id": "3", undoDataKey: "true"}},
			{Action: tagActionDelete, Data: map[string]string{"id": "8", undoDataKey: "true"}},
		} {
			require.NoError(t, app.HandleDOMAction(ctx, action))
		}

		require.NoError(t, app.HandleDOMAction(ctx, DOMAction{Action: actionFlushDeletes}))

		assert.Equal(t, []int64{2, 3, 8}, called(), "every pending favorite and tag delete is issued")
		assert.Equal(t, "Could not delete: locked 3; locked 8", app.favoritesError, "favorites then tags, in id order")
		assert.Empty(t, app.pendingFavoriteDeletes)
		assert.Empty(t, app.pendingTagDeletes)
	})
}

func TestFlushIssuesHistoryAndFavoritesDeletesConcurrently(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		history := newHistoryStore(t, &historyStore{})
		favorites := newFavoritesStore(t, &favoritesStore{
			favorites: []*entity.Favorite{{ID: 2, URL: "https://news.example", Title: "News"}},
		})
		del, called := barrierDeletes(2, nil)
		history.service.EXPECT().DeleteEntry(mock.Anything, int64(1)).
			RunAndReturn(func(_ context.Context, id int64) error { return del(id) }).Once()
		favorites.service.EXPECT().DeleteFavorite(mock.Anything, int64(2)).
			RunAndReturn(func(_ context.Context, id int64) error { return del(id) }).Once()
		app := NewApp(Dependencies{
			DOM:         &toastDOM{},
			History:     history.service,
			Favorites:   favorites.service,
			LocationURI: "dumb://favorites",
		})
		ctx := context.Background()
		require.NoError(t, app.LoadInitial(ctx))
		require.NoError(t, app.HandleDOMAction(ctx, deferredDelete("1")))
		require.NoError(t, app.HandleDOMAction(ctx, DOMAction{
			Action: favoriteActionDelete, Data: map[string]string{"id": "2", undoDataKey: "true"},
		}))

		require.NoError(t, app.HandleDOMAction(ctx, DOMAction{Action: actionFlushDeletes}))

		assert.Equal(t, []int64{1, 2}, called(), "history and favorites deletes start together")
	})
}
