package usecase_test

import (
	"testing"

	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/domain/autocomplete"
	"github.com/bnema/dumber/internal/domain/entity"
	repomocks "github.com/bnema/dumber/internal/domain/repository/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestAutocompleteUseCase_GetSuggestion_EmptyInput(t *testing.T) {
	ctx := testContext()
	uc := usecase.NewAutocompleteUseCase(nil, nil, nil)

	output := uc.GetSuggestion(ctx, usecase.GetSuggestionInput{Input: ""})

	require.NotNil(t, output)
	assert.False(t, output.Found)
	assert.Nil(t, output.Suggestion)
}

func TestAutocompleteUseCase_GetSuggestion_NilDependencies(t *testing.T) {
	ctx := testContext()
	uc := usecase.NewAutocompleteUseCase(nil, nil, nil)

	output := uc.GetSuggestion(ctx, usecase.GetSuggestionInput{Input: "github"})

	require.NotNil(t, output)
	assert.False(t, output.Found)
}

func TestAutocompleteUseCase_GetSuggestion_FavoritesPriority(t *testing.T) {
	ctx := testContext()

	favoriteRepo := repomocks.NewMockFavoriteRepository(t)
	folderRepo := repomocks.NewMockFolderRepository(t)
	tagRepo := repomocks.NewMockTagRepository(t)

	favorites := []*entity.Favorite{
		{ID: 1, URL: "https://github.com/user/repo", Title: "My Repo"},
		{ID: 2, URL: "https://google.com", Title: "Google"},
	}
	favoriteRepo.EXPECT().GetAll(mock.Anything).Return(favorites, nil)

	favoritesUC := usecase.NewManageFavoritesUseCase(favoriteRepo, folderRepo, tagRepo)
	uc := usecase.NewAutocompleteUseCase(nil, favoritesUC, nil)

	output := uc.GetSuggestion(ctx, usecase.GetSuggestionInput{Input: "github"})

	require.NotNil(t, output)
	assert.True(t, output.Found)
	assert.NotNil(t, output.Suggestion)
	assert.Equal(t, autocomplete.SourceFavorite, output.Suggestion.Source)
	assert.Equal(t, "github.com/user/repo", output.Suggestion.FullText)
	assert.Equal(t, ".com/user/repo", output.Suggestion.Suffix)
	assert.Equal(t, "My Repo", output.Suggestion.Title)
}

func TestAutocompleteUseCase_GetSuggestion_HistoryFallback(t *testing.T) {
	ctx := testContext()

	// Favorites returns empty
	favoriteRepo := repomocks.NewMockFavoriteRepository(t)
	folderRepo := repomocks.NewMockFolderRepository(t)
	tagRepo := repomocks.NewMockTagRepository(t)
	favoriteRepo.EXPECT().GetAll(mock.Anything).Return([]*entity.Favorite{}, nil)

	// History returns a match
	historyRepo := repomocks.NewMockHistoryRepository(t)
	historyMatches := []entity.HistoryMatch{
		{Entry: &entity.HistoryEntry{URL: "https://news.ycombinator.com", Title: "Hacker News"}},
	}
	historyRepo.EXPECT().Search(mock.Anything, "news", 10).Return(historyMatches, nil)

	favoritesUC := usecase.NewManageFavoritesUseCase(favoriteRepo, folderRepo, tagRepo)
	historyUC := usecase.NewSearchHistoryUseCase(historyRepo)
	uc := usecase.NewAutocompleteUseCase(historyUC, favoritesUC, nil)

	output := uc.GetSuggestion(ctx, usecase.GetSuggestionInput{Input: "news"})

	require.NotNil(t, output)
	assert.True(t, output.Found)
	assert.NotNil(t, output.Suggestion)
	assert.Equal(t, autocomplete.SourceHistory, output.Suggestion.Source)
	assert.Equal(t, "news.ycombinator.com", output.Suggestion.FullText)
	assert.Equal(t, ".ycombinator.com", output.Suggestion.Suffix)
}

func TestAutocompleteUseCase_GetSuggestion_BangShortcut(t *testing.T) {
	ctx := testContext()

	shortcuts := map[string]usecase.SearchShortcut{
		"gh":  {URL: "https://github.com/search?q=%s", Description: "GitHub search"},
		"ddg": {URL: "https://duckduckgo.com/?q=%s", Description: "DuckDuckGo"},
	}
	shortcutsUC := usecase.NewSearchShortcutsUseCase(shortcuts)
	uc := usecase.NewAutocompleteUseCase(nil, nil, shortcutsUC)

	output := uc.GetSuggestion(ctx, usecase.GetSuggestionInput{Input: "!g"})

	require.NotNil(t, output)
	assert.True(t, output.Found)
	assert.NotNil(t, output.Suggestion)
	assert.Equal(t, autocomplete.SourceBangShortcut, output.Suggestion.Source)
	assert.Equal(t, "!gh", output.Suggestion.FullText)
	assert.Equal(t, "h", output.Suggestion.Suffix)
	assert.Equal(t, "GitHub search", output.Suggestion.Title)
}

func TestAutocompleteUseCase_GetSuggestion_BangWithoutPrefix(t *testing.T) {
	ctx := testContext()

	shortcuts := map[string]usecase.SearchShortcut{
		"gh": {URL: "https://github.com/search?q=%s", Description: "GitHub search"},
	}
	shortcutsUC := usecase.NewSearchShortcutsUseCase(shortcuts)
	uc := usecase.NewAutocompleteUseCase(nil, nil, shortcutsUC)

	// Input without ! should not match bang shortcuts
	output := uc.GetSuggestion(ctx, usecase.GetSuggestionInput{Input: "gh"})

	require.NotNil(t, output)
	assert.False(t, output.Found)
}

func TestAutocompleteUseCase_GetSuggestion_NoMatch(t *testing.T) {
	ctx := testContext()

	favoriteRepo := repomocks.NewMockFavoriteRepository(t)
	folderRepo := repomocks.NewMockFolderRepository(t)
	tagRepo := repomocks.NewMockTagRepository(t)
	favoriteRepo.EXPECT().GetAll(mock.Anything).Return([]*entity.Favorite{}, nil)

	historyRepo := repomocks.NewMockHistoryRepository(t)
	historyRepo.EXPECT().Search(mock.Anything, "xyz123", 10).Return([]entity.HistoryMatch{}, nil)

	favoritesUC := usecase.NewManageFavoritesUseCase(favoriteRepo, folderRepo, tagRepo)
	historyUC := usecase.NewSearchHistoryUseCase(historyRepo)
	uc := usecase.NewAutocompleteUseCase(historyUC, favoritesUC, nil)

	output := uc.GetSuggestion(ctx, usecase.GetSuggestionInput{Input: "xyz123"})

	require.NotNil(t, output)
	assert.False(t, output.Found)
}

func TestAutocompleteUseCase_GetSuggestionForURL_EmptyInput(t *testing.T) {
	ctx := testContext()
	uc := usecase.NewAutocompleteUseCase(nil, nil, nil)

	tests := []struct {
		name      string
		input     string
		targetURL string
	}{
		{name: "empty input", input: "", targetURL: "https://example.com"},
		{name: "empty URL", input: "exa", targetURL: ""},
		{name: "both empty", input: "", targetURL: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := uc.GetSuggestionForURL(ctx, tt.input, tt.targetURL)
			require.NotNil(t, output)
			assert.False(t, output.Found)
		})
	}
}

func TestAutocompleteUseCase_GetSuggestionForURL_Match(t *testing.T) {
	ctx := testContext()
	uc := usecase.NewAutocompleteUseCase(nil, nil, nil)

	tests := []struct {
		name           string
		input          string
		targetURL      string
		wantSuffix     string
		wantMatchedURL string
	}{
		{
			name:           "prefix match without protocol",
			input:          "github",
			targetURL:      "https://github.com/user/repo",
			wantSuffix:     ".com/user/repo",
			wantMatchedURL: "github.com/user/repo",
		},
		{
			name:           "prefix match with protocol",
			input:          "https://github",
			targetURL:      "https://github.com/user/repo",
			wantSuffix:     ".com/user/repo",
			wantMatchedURL: "https://github.com/user/repo",
		},
		{
			name:           "case insensitive match",
			input:          "GITHUB",
			targetURL:      "https://github.com",
			wantSuffix:     ".com",
			wantMatchedURL: "github.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := uc.GetSuggestionForURL(ctx, tt.input, tt.targetURL)
			require.NotNil(t, output)
			assert.True(t, output.Found)
			require.NotNil(t, output.Suggestion)
			assert.Equal(t, tt.wantSuffix, output.Suggestion.Suffix)
			assert.Equal(t, tt.wantMatchedURL, output.Suggestion.FullText)
		})
	}
}

func TestAutocompleteUseCase_GetSuggestionForURL_NoMatch(t *testing.T) {
	ctx := testContext()
	uc := usecase.NewAutocompleteUseCase(nil, nil, nil)

	output := uc.GetSuggestionForURL(ctx, "xyz", "https://github.com")

	require.NotNil(t, output)
	assert.False(t, output.Found)
}

func TestAutocompleteUseCase_GetSuggestion_FavoritesOverHistory(t *testing.T) {
	ctx := testContext()

	// Both favorites and history have matches - favorites should win
	favoriteRepo := repomocks.NewMockFavoriteRepository(t)
	folderRepo := repomocks.NewMockFolderRepository(t)
	tagRepo := repomocks.NewMockTagRepository(t)
	favorites := []*entity.Favorite{
		{ID: 1, URL: "https://github.com/favorite", Title: "Favorite Repo"},
	}
	favoriteRepo.EXPECT().GetAll(mock.Anything).Return(favorites, nil)

	// History would also match, but should not be called because favorites matched
	historyRepo := repomocks.NewMockHistoryRepository(t)
	// Note: no EXPECT on historyRepo - it shouldn't be called

	favoritesUC := usecase.NewManageFavoritesUseCase(favoriteRepo, folderRepo, tagRepo)
	historyUC := usecase.NewSearchHistoryUseCase(historyRepo)
	uc := usecase.NewAutocompleteUseCase(historyUC, favoritesUC, nil)

	output := uc.GetSuggestion(ctx, usecase.GetSuggestionInput{Input: "github"})

	require.NotNil(t, output)
	assert.True(t, output.Found)
	assert.Equal(t, autocomplete.SourceFavorite, output.Suggestion.Source)
	assert.Equal(t, "Favorite Repo", output.Suggestion.Title)
}

func TestAutocompleteUseCase_GetSuggestion_HistoryNotPrefixMatch(t *testing.T) {
	ctx := testContext()

	// Favorites returns empty
	favoriteRepo := repomocks.NewMockFavoriteRepository(t)
	folderRepo := repomocks.NewMockFolderRepository(t)
	tagRepo := repomocks.NewMockTagRepository(t)
	favoriteRepo.EXPECT().GetAll(mock.Anything).Return([]*entity.Favorite{}, nil)

	// History returns results but none are prefix matches
	historyRepo := repomocks.NewMockHistoryRepository(t)
	historyMatches := []entity.HistoryMatch{
		// URL doesn't start with "git", so no prefix match
		{Entry: &entity.HistoryEntry{URL: "https://example.com/git", Title: "Example"}},
	}
	historyRepo.EXPECT().Search(mock.Anything, "git", 10).Return(historyMatches, nil)

	favoritesUC := usecase.NewManageFavoritesUseCase(favoriteRepo, folderRepo, tagRepo)
	historyUC := usecase.NewSearchHistoryUseCase(historyRepo)
	uc := usecase.NewAutocompleteUseCase(historyUC, favoritesUC, nil)

	output := uc.GetSuggestion(ctx, usecase.GetSuggestionInput{Input: "git"})

	require.NotNil(t, output)
	assert.False(t, output.Found)
}
