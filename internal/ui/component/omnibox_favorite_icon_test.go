package component

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOmniboxFavoriteStarSize(t *testing.T) {
	assert.Equal(t, 18, favoriteStarSize(1.0))
	assert.Equal(t, 27, favoriteStarSize(1.5))
}

func TestOmniboxFavoriteStarVisibility(t *testing.T) {
	assert.True(t, shouldShowFavoriteStar(Suggestion{IsFavorite: true}))
	assert.False(t, shouldShowFavoriteStar(Suggestion{IsFavorite: false}))
}
