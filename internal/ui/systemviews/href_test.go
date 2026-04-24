package systemviews

import (
	"testing"

	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHistoryHTMLSanitizesHrefSchemes(t *testing.T) {
	t.Parallel()

	html := historyHTML(historyRenderData{Entries: []*entity.HistoryEntry{
		{URL: "javascript:alert(1)", Title: "Bad"},
		{URL: "https://example.com", Title: "Good"},
	}})

	require.NotContains(t, html, `href="javascript:alert(1)"`)
	assert.Contains(t, html, `href="#"`)
	assert.Contains(t, html, `href="https://example.com"`)
}

func TestFavoriteItemsHTMLSanitizesHrefSchemes(t *testing.T) {
	t.Parallel()

	html := favoritesHTML(favoritesRenderData{Favorites: []*entity.Favorite{
		{URL: "javascript:alert(1)", Title: "Bad"},
		{URL: "https://example.com", Title: "Good"},
	}})

	require.NotContains(t, html, `href="javascript:alert(1)"`)
	assert.Contains(t, html, `href="#"`)
	assert.Contains(t, html, `href="https://example.com"`)
}

func TestFavoritesHTMLSanitizesTagColors(t *testing.T) {
	t.Parallel()

	html := favoritesHTML(favoritesRenderData{
		Favorites: []*entity.Favorite{{ID: 1, URL: "https://example.com", Title: "Example"}},
		Tags:      []*entity.Tag{{ID: 7, Name: "Bad", Color: `red;background:url(javascript:alert(1))`}},
	})

	require.NotContains(t, html, `style="background:red`)
	assert.Contains(t, html, "background:#808080")
}
