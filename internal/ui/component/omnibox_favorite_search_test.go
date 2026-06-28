package component

import (
	"testing"

	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/stretchr/testify/assert"
)

func TestOmniboxFavoriteSearchResultsAllowTagsPresent(t *testing.T) {
	results := []*entity.Favorite{
		{
			ID:    1,
			URL:   "https://example.com/docs",
			Title: "Docs",
			Tags:  []entity.Tag{{ID: 7, Name: "Go"}},
		},
	}

	favorites := favoriteResultsForOmnibox(results)

	assert.Equal(t, []Favorite{
		{
			ID:       1,
			URL:      "https://example.com/docs",
			Title:    "Docs",
			Position: 0,
		},
	}, favorites)
}
