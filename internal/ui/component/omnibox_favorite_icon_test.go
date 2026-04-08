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

func TestResolveFavoriteRowIndicatorUpdate(t *testing.T) {
	tests := []struct {
		name        string
		mode        ViewMode
		index       int
		expectedURL string
		suggestions []Suggestion
		isFavorite  bool
		want        favoriteRowIndicatorUpdate
	}{
		{
			name:        "matching history row keeps update and shows star",
			mode:        ViewModeHistory,
			index:       0,
			expectedURL: "https://example.com",
			suggestions: []Suggestion{{URL: "https://example.com"}},
			isFavorite:  true,
			want:        favoriteRowIndicatorUpdate{Apply: true, ShowStarSlot: true},
		},
		{
			name:        "matching history row keeps update and hides star when removed",
			mode:        ViewModeHistory,
			index:       0,
			expectedURL: "https://example.com",
			suggestions: []Suggestion{{URL: "https://example.com"}},
			isFavorite:  false,
			want:        favoriteRowIndicatorUpdate{Apply: true, ShowStarSlot: false},
		},
		{
			name:        "mismatched URL skips stale row update",
			mode:        ViewModeHistory,
			index:       0,
			expectedURL: "https://expected.example.com",
			suggestions: []Suggestion{{URL: "https://current.example.com"}},
			isFavorite:  true,
			want:        favoriteRowIndicatorUpdate{},
		},
		{
			name:        "favorites mode never applies history row update",
			mode:        ViewModeFavorites,
			index:       0,
			expectedURL: "https://example.com",
			suggestions: []Suggestion{{URL: "https://example.com"}},
			isFavorite:  true,
			want:        favoriteRowIndicatorUpdate{},
		},
		{
			name:        "out of range index skips update",
			mode:        ViewModeHistory,
			index:       1,
			expectedURL: "https://example.com",
			suggestions: []Suggestion{{URL: "https://example.com"}},
			isFavorite:  true,
			want:        favoriteRowIndicatorUpdate{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveFavoriteRowIndicatorUpdate(tt.mode, tt.index, tt.expectedURL, tt.suggestions, tt.isFavorite)
			assert.Equal(t, tt.want, got)
		})
	}
}
