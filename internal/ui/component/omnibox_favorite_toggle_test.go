package component

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolveFavoriteToggleTarget(t *testing.T) {
	suggestions := []Suggestion{
		{URL: "https://example.com", Title: "Example"},
		{URL: "https://favorite.example.com", Title: "Favorite", IsFavorite: true},
	}
	favorites := []Favorite{
		{ID: 42, URL: "https://saved.example.com", Title: "Saved"},
		{ID: 0, URL: "https://invalid.example.com", Title: "Invalid"},
	}

	tests := []struct {
		name            string
		mode            ViewMode
		index           int
		suggestions     []Suggestion
		favorites       []Favorite
		wantTarget      favoriteToggleTarget
		wantValidTarget bool
	}{
		{
			name:        "history add target uses selected suggestion URL and title",
			mode:        ViewModeHistory,
			index:       0,
			suggestions: suggestions,
			favorites:   favorites,
			wantTarget: favoriteToggleTarget{
				url:   "https://example.com",
				title: "Example",
			},
			wantValidTarget: true,
		},
		{
			name:        "history remove target preserves prior favorite state",
			mode:        ViewModeHistory,
			index:       1,
			suggestions: suggestions,
			favorites:   favorites,
			wantTarget: favoriteToggleTarget{
				url:         "https://favorite.example.com",
				title:       "Favorite",
				wasFavorite: true,
			},
			wantValidTarget: true,
		},
		{
			name:            "history invalid index has no target",
			mode:            ViewModeHistory,
			index:           2,
			suggestions:     suggestions,
			favorites:       favorites,
			wantValidTarget: false,
		},
		{
			name:            "history empty URL has no target",
			mode:            ViewModeHistory,
			index:           0,
			suggestions:     []Suggestion{{Title: "Missing URL"}},
			favorites:       favorites,
			wantValidTarget: false,
		},
		{
			name:        "favorites remove target uses favorite ID",
			mode:        ViewModeFavorites,
			index:       0,
			suggestions: suggestions,
			favorites:   favorites,
			wantTarget: favoriteToggleTarget{
				url:        "https://saved.example.com",
				title:      "Saved",
				favoriteID: 42,
			},
			wantValidTarget: true,
		},
		{
			name:            "favorites invalid index has no target",
			mode:            ViewModeFavorites,
			index:           -1,
			suggestions:     suggestions,
			favorites:       favorites,
			wantValidTarget: false,
		},
		{
			name:            "favorites zero ID has no target",
			mode:            ViewModeFavorites,
			index:           1,
			suggestions:     suggestions,
			favorites:       favorites,
			wantValidTarget: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotTarget, gotValidTarget := resolveFavoriteToggleTarget(tt.mode, tt.index, tt.suggestions, tt.favorites)
			assert.Equal(t, tt.wantValidTarget, gotValidTarget)
			assert.Equal(t, tt.wantTarget, gotTarget)
		})
	}
}

func TestResolveFavoriteToggleResultUpdate(t *testing.T) {
	suggestions := []Suggestion{{URL: "https://example.com"}}

	tests := []struct {
		name        string
		index       int
		expectedURL string
		added       bool
		want        favoriteToggleResultUpdate
	}{
		{
			name:        "matching URL updates selected suggestion favorite state",
			index:       0,
			expectedURL: "https://example.com",
			added:       true,
			want:        favoriteToggleResultUpdate{Apply: true, Index: 0, IsFavorite: true},
		},
		{
			name:        "stale URL skips update",
			index:       0,
			expectedURL: "https://stale.example.com",
			added:       true,
			want:        favoriteToggleResultUpdate{},
		},
		{
			name:        "out of range index skips update",
			index:       1,
			expectedURL: "https://example.com",
			added:       true,
			want:        favoriteToggleResultUpdate{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveFavoriteToggleResultUpdate(suggestions, tt.index, tt.expectedURL, tt.added)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestFavoriteToggleErrorMessage(t *testing.T) {
	assert.Equal(t, "Failed to add favorite", favoriteToggleErrorMessage(false))
	assert.Equal(t, "Failed to remove favorite", favoriteToggleErrorMessage(true))
}
