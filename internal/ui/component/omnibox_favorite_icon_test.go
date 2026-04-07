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

func TestShouldApplyEmptyResultsState(t *testing.T) {
	tests := []struct {
		name     string
		rowCount int
		visible  bool
		want     bool
	}{
		{name: "negative rows uses empty state", rowCount: -1, visible: false, want: true},
		{name: "zero rows uses empty state", rowCount: 0, visible: false, want: true},
		{name: "positive rows clears empty state", rowCount: 2, visible: true, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, shouldApplyEmptyResultsState(tt.rowCount, tt.visible))
		})
	}
}
