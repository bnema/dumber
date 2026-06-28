package homepage

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/bnema/dumber/internal/application/dto"
	"github.com/bnema/dumber/internal/application/port"
	portmocks "github.com/bnema/dumber/internal/application/port/mocks"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestHandleFavoriteCreatePassesTagsOnlyPayload(t *testing.T) {
	t.Parallel()

	favorites := portmocks.NewMockHomepageFavorites(t)
	favorites.EXPECT().
		AddFavorite(mock.Anything, dto.FavoriteCreateInput{
			URL:   "https://example.com",
			Title: "Example",
			Tags:  []entity.TagID{7, 9},
		}).
		Return(&entity.Favorite{ID: 1, URL: "https://example.com", Title: "Example"}, nil).
		Once()

	handler := NewFavoritesHandlers(favorites).HandleCreate()
	got, err := handler.Handle(context.Background(), port.WebViewID(0), json.RawMessage(`{"requestId":"req-1","url":" https://example.com ","title":"Example","tags":[7,9]}`))
	require.NoError(t, err)

	resp, ok := got.(Response)
	require.True(t, ok)
	require.True(t, resp.Success)
}

func TestHandleFavoriteCreateRejectsInvalidTagIDs(t *testing.T) {
	t.Parallel()

	handler := NewFavoritesHandlers(portmocks.NewMockHomepageFavorites(t)).HandleCreate()
	got, err := handler.Handle(context.Background(), port.WebViewID(0), json.RawMessage(`{"requestId":"req-2","url":"https://example.com","tags":[0]}`))
	require.NoError(t, err)

	resp, ok := got.(Response)
	require.True(t, ok)
	require.False(t, resp.Success)
	require.Contains(t, resp.Error, "tag id must be positive")
}

func TestHandleFavoriteUpdatePassesTagsOnlyPayload(t *testing.T) {
	t.Parallel()

	shortcut := 4
	favorites := portmocks.NewMockHomepageFavorites(t)
	favorites.EXPECT().
		UpdateFavorite(mock.Anything, dto.FavoriteUpdateInput{
			ID:             entity.FavoriteID(42),
			Title:          "Updated",
			FaviconURL:     "https://example.com/favicon.ico",
			ShortcutKey:    &shortcut,
			ShortcutKeySet: true,
		}).
		Return(&entity.Favorite{ID: 42, URL: "https://example.com", Title: "Updated", ShortcutKey: &shortcut}, nil).
		Once()

	handler := NewFavoritesHandlers(favorites).HandleUpdate()
	got, err := handler.Handle(context.Background(), port.WebViewID(0), json.RawMessage(`{"requestId":"req-3","id":42,"title":"Updated","favicon_url":"https://example.com/favicon.ico","shortcut_key":4}`))
	require.NoError(t, err)

	resp, ok := got.(Response)
	require.True(t, ok)
	require.True(t, resp.Success)
}
func TestHandleFavoriteUpdatePreservesShortcutWhenOmitted(t *testing.T) {
	t.Parallel()

	favorites := portmocks.NewMockHomepageFavorites(t)
	favorites.EXPECT().
		UpdateFavorite(mock.Anything, dto.FavoriteUpdateInput{
			ID:         entity.FavoriteID(42),
			Title:      "Updated",
			FaviconURL: "https://example.com/favicon.ico",
		}).
		Return(&entity.Favorite{ID: 42, URL: "https://example.com", Title: "Updated"}, nil).
		Once()

	handler := NewFavoritesHandlers(favorites).HandleUpdate()
	got, err := handler.Handle(context.Background(), port.WebViewID(0), json.RawMessage(`{"requestId":"req-4","id":42,"title":"Updated","favicon_url":"https://example.com/favicon.ico"}`))
	require.NoError(t, err)

	resp, ok := got.(Response)
	require.True(t, ok)
	require.True(t, resp.Success)
}
func TestHandleFavoriteUpdateClearsShortcutWhenExplicitNull(t *testing.T) {
	t.Parallel()

	favorites := portmocks.NewMockHomepageFavorites(t)
	favorites.EXPECT().
		UpdateFavorite(mock.Anything, dto.FavoriteUpdateInput{
			ID:             entity.FavoriteID(42),
			Title:          "Updated",
			FaviconURL:     "https://example.com/favicon.ico",
			ShortcutKeySet: true,
		}).
		Return(&entity.Favorite{ID: 42, URL: "https://example.com", Title: "Updated"}, nil).
		Once()

	handler := NewFavoritesHandlers(favorites).HandleUpdate()
	got, err := handler.Handle(context.Background(), port.WebViewID(0), json.RawMessage(`{"requestId":"req-5","id":42,"title":"Updated","favicon_url":"https://example.com/favicon.ico","shortcut_key":null}`))
	require.NoError(t, err)

	resp, ok := got.(Response)
	require.True(t, ok)
	require.True(t, resp.Success)
}
