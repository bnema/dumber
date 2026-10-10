package component

import "testing"

func TestGhostCompletion(t *testing.T) {
	history := []Suggestion{
		{URL: "https://www.google.com/url?q=https://x"},
		{URL: "https://github.com/bnema/dumber"},
	}
	favorites := []Favorite{{URL: "https://gitlab.com/"}}

	tests := []struct {
		name       string
		req        ghostCompletionRequest
		wantFull   string
		wantSuffix string
		wantOK     bool
	}{
		{
			name:       "host-like input completes to the domain, not the redirect URL",
			req:        ghostCompletionRequest{EntryText: "goo", Mode: ViewModeHistory, MaxResults: 5, Suggestions: history},
			wantFull:   "google.com",
			wantSuffix: "gle.com",
			wantOK:     true,
		},
		{
			name:       "leading whitespace is ignored",
			req:        ghostCompletionRequest{EntryText: "  gith", Mode: ViewModeHistory, MaxResults: 5, Suggestions: history},
			wantFull:   "github.com",
			wantSuffix: "ub.com",
			wantOK:     true,
		},
		{
			name:   "hidden rows never complete",
			req:    ghostCompletionRequest{EntryText: "gith", Mode: ViewModeHistory, MaxResults: 1, Suggestions: history},
			wantOK: false,
		},
		{
			name:       "favorites mode uses favorites",
			req:        ghostCompletionRequest{EntryText: "gitl", Mode: ViewModeFavorites, MaxResults: 5, Favorites: favorites},
			wantFull:   "gitlab.com",
			wantSuffix: "ab.com",
			wantOK:     true,
		},
		{
			name: "explicit selection completes from the selected URL, not the best match",
			req: ghostCompletionRequest{
				EntryText: "gi", SelectedURL: "https://gitlab.com/", HasExplicitSelection: true,
				Mode: ViewModeHistory, MaxResults: 5, Suggestions: history,
			},
			wantFull:   "gitlab.com",
			wantSuffix: "tlab.com",
			wantOK:     true,
		},
		{
			name:   "blank input shows nothing",
			req:    ghostCompletionRequest{EntryText: "   ", Mode: ViewModeHistory, MaxResults: 5, Suggestions: history},
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			full, suffix, ok := ghostCompletion(tt.req)
			if ok != tt.wantOK || full != tt.wantFull || suffix != tt.wantSuffix {
				t.Fatalf("ghostCompletion() = (%q, %q, %v), want (%q, %q, %v)",
					full, suffix, ok, tt.wantFull, tt.wantSuffix, tt.wantOK)
			}
		})
	}
}

func TestNavigableCount(t *testing.T) {
	limit := OmniboxListDefaults.MaxResults
	tests := []struct {
		name                          string
		bangMode                      bool
		mode                          ViewMode
		bangs, suggestions, favorites int
		want                          int
	}{
		{"history beyond visible rows", false, ViewModeHistory, 0, limit, 30, limit},
		{"favorites capped at results limit", false, ViewModeFavorites, 0, 3, 30, limit},
		{"bangs win over view mode", true, ViewModeHistory, 2, 8, 0, 2},
		{"empty list", false, ViewModeFavorites, 4, 4, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := navigableCount(tt.bangMode, tt.mode, tt.bangs, tt.suggestions, tt.favorites)
			if got != tt.want {
				t.Fatalf("navigableCount() = %d, want %d", got, tt.want)
			}
		})
	}
}
