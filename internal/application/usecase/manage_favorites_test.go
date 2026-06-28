package usecase_test

import (
	"context"
	"errors"
	"testing"

	"github.com/bnema/dumber/internal/application/dto"
	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/domain/entity"
	repomocks "github.com/bnema/dumber/internal/domain/repository/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestManageFavoritesUseCase_GetAllURLs_ReturnsURLSet(t *testing.T) {
	ctx := testContext()

	favoriteRepo := repomocks.NewMockFavoriteRepository(t)
	tagRepo := repomocks.NewMockTagRepository(t)

	favorites := []*entity.Favorite{
		{ID: 1, URL: "https://example.com"},
		{ID: 2, URL: "https://github.com"},
		{ID: 3, URL: "https://google.com"},
	}

	favoriteRepo.EXPECT().GetAll(mock.Anything).Return(favorites, nil)

	uc := usecase.NewManageFavoritesUseCase(favoriteRepo, tagRepo)

	urls, err := uc.GetAllURLs(ctx)
	require.NoError(t, err)
	require.NotNil(t, urls)

	assert.Len(t, urls, 3)
	_, hasExample := urls["https://example.com"]
	_, hasGithub := urls["https://github.com"]
	_, hasGoogle := urls["https://google.com"]
	assert.True(t, hasExample)
	assert.True(t, hasGithub)
	assert.True(t, hasGoogle)
}

func TestManageFavoritesUseCase_GetAllURLs_ReturnsEmptySetWhenNoFavorites(t *testing.T) {
	ctx := testContext()

	favoriteRepo := repomocks.NewMockFavoriteRepository(t)
	tagRepo := repomocks.NewMockTagRepository(t)

	favoriteRepo.EXPECT().GetAll(mock.Anything).Return([]*entity.Favorite{}, nil)

	uc := usecase.NewManageFavoritesUseCase(favoriteRepo, tagRepo)

	urls, err := uc.GetAllURLs(ctx)
	require.NoError(t, err)
	require.NotNil(t, urls)
	assert.Empty(t, urls)
}

func TestManageFavoritesUseCase_GetAllURLs_ReturnsErrorOnRepoFailure(t *testing.T) {
	ctx := testContext()

	favoriteRepo := repomocks.NewMockFavoriteRepository(t)
	tagRepo := repomocks.NewMockTagRepository(t)

	favoriteRepo.EXPECT().GetAll(mock.Anything).Return(nil, errors.New("db connection failed"))

	uc := usecase.NewManageFavoritesUseCase(favoriteRepo, tagRepo)

	urls, err := uc.GetAllURLs(ctx)
	require.Error(t, err)
	assert.Nil(t, urls)
	assert.Contains(t, err.Error(), "failed to get favorites")
}

func TestManageFavoritesUseCase_Toggle_AddsWhenNotFavorite(t *testing.T) {
	ctx := testContext()

	favoriteRepo := repomocks.NewMockFavoriteRepository(t)
	tagRepo := repomocks.NewMockTagRepository(t)

	// URL is not a favorite
	favoriteRepo.EXPECT().FindByURL(mock.Anything, "https://example.com").Return(nil, nil)
	favoriteRepo.EXPECT().Save(mock.Anything, mock.MatchedBy(func(f *entity.Favorite) bool {
		return f.URL == "https://example.com" && f.Title == "Example"
	})).Return(nil)

	uc := usecase.NewManageFavoritesUseCase(favoriteRepo, tagRepo)

	result, err := uc.Toggle(ctx, "https://example.com", "Example")
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.True(t, result.Added)
	assert.Equal(t, "https://example.com", result.URL)
	assert.Equal(t, "Example", result.Title)
	assert.Equal(t, "Favorite added", result.Message)
}

func TestManageFavoritesUseCase_Toggle_RemovesWhenAlreadyFavorite(t *testing.T) {
	ctx := testContext()

	favoriteRepo := repomocks.NewMockFavoriteRepository(t)
	tagRepo := repomocks.NewMockTagRepository(t)

	existingFav := &entity.Favorite{
		ID:    42,
		URL:   "https://example.com",
		Title: "Example Site",
	}

	// URL is already a favorite
	favoriteRepo.EXPECT().FindByURL(mock.Anything, "https://example.com").Return(existingFav, nil)
	favoriteRepo.EXPECT().Delete(mock.Anything, entity.FavoriteID(42)).Return(nil)

	uc := usecase.NewManageFavoritesUseCase(favoriteRepo, tagRepo)

	result, err := uc.Toggle(ctx, "https://example.com", "Example")
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.False(t, result.Added)
	assert.Equal(t, "https://example.com", result.URL)
	assert.Equal(t, "Example Site", result.Title) // Uses existing title
	assert.Equal(t, "Favorite removed", result.Message)
}

func TestManageFavoritesUseCase_Toggle_EmptyURLReturnsError(t *testing.T) {
	ctx := testContext()

	favoriteRepo := repomocks.NewMockFavoriteRepository(t)
	tagRepo := repomocks.NewMockTagRepository(t)

	uc := usecase.NewManageFavoritesUseCase(favoriteRepo, tagRepo)

	result, err := uc.Toggle(ctx, "", "Title")
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "favorite URL is required")
}

func TestManageFavoritesUseCase_ToggleNormalizesURLBeforeLookup(t *testing.T) {
	ctx := testContext()

	favoriteRepo := repomocks.NewMockFavoriteRepository(t)
	tagRepo := repomocks.NewMockTagRepository(t)

	favoriteRepo.EXPECT().FindByURL(mock.Anything, "https://example.com").Return(nil, nil).Once()
	favoriteRepo.EXPECT().FindByURL(mock.Anything, "example.com").Return(nil, nil).Once()
	favoriteRepo.EXPECT().Save(mock.Anything, mock.MatchedBy(func(fav *entity.Favorite) bool {
		return fav != nil && fav.URL == "https://example.com" && fav.Title == "Example"
	})).Return(nil).Once()

	uc := usecase.NewManageFavoritesUseCase(favoriteRepo, tagRepo)

	result, err := uc.Toggle(ctx, " example.com ", " Example ")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Added)
	assert.Equal(t, "https://example.com", result.URL)
	assert.Equal(t, "Example", result.Title)
}

func TestManageFavoritesUseCase_Toggle_FindByURLErrorReturnsError(t *testing.T) {
	ctx := testContext()

	favoriteRepo := repomocks.NewMockFavoriteRepository(t)
	tagRepo := repomocks.NewMockTagRepository(t)

	favoriteRepo.EXPECT().FindByURL(mock.Anything, "https://example.com").Return(nil, errors.New("db error"))

	uc := usecase.NewManageFavoritesUseCase(favoriteRepo, tagRepo)

	result, err := uc.Toggle(ctx, "https://example.com", "Example")
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "failed to check existing favorite")
}

func TestManageFavoritesUseCase_Toggle_SaveErrorReturnsError(t *testing.T) {
	ctx := testContext()

	favoriteRepo := repomocks.NewMockFavoriteRepository(t)
	tagRepo := repomocks.NewMockTagRepository(t)

	favoriteRepo.EXPECT().FindByURL(mock.Anything, "https://example.com").Return(nil, nil)
	favoriteRepo.EXPECT().Save(mock.Anything, mock.Anything).Return(errors.New("db error"))

	uc := usecase.NewManageFavoritesUseCase(favoriteRepo, tagRepo)

	result, err := uc.Toggle(ctx, "https://example.com", "Example")
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "failed to add favorite")
}

func TestManageFavoritesUseCase_Toggle_DeleteErrorReturnsError(t *testing.T) {
	ctx := testContext()

	favoriteRepo := repomocks.NewMockFavoriteRepository(t)
	tagRepo := repomocks.NewMockTagRepository(t)

	existingFav := &entity.Favorite{ID: 42, URL: "https://example.com", Title: "Example"}
	favoriteRepo.EXPECT().FindByURL(mock.Anything, "https://example.com").Return(existingFav, nil)
	favoriteRepo.EXPECT().Delete(mock.Anything, entity.FavoriteID(42)).Return(errors.New("db error"))

	uc := usecase.NewManageFavoritesUseCase(favoriteRepo, tagRepo)

	result, err := uc.Toggle(ctx, "https://example.com", "Example")
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "failed to remove favorite")
}

func TestManageFavoritesUseCase_UpdateFavoritePreservesShortcutWhenOmitted(t *testing.T) {
	ctx := testContext()
	shortcut := 3
	existing := &entity.Favorite{
		ID:          42,
		URL:         "https://example.com",
		Title:       "Example",
		ShortcutKey: &shortcut,
	}

	favoriteRepo := repomocks.NewMockFavoriteRepository(t)
	tagRepo := repomocks.NewMockTagRepository(t)
	favoriteRepo.EXPECT().FindByID(mock.Anything, entity.FavoriteID(42)).Return(existing, nil)
	favoriteRepo.EXPECT().Save(mock.Anything, mock.MatchedBy(func(fav *entity.Favorite) bool {
		return fav.ID == 42 && fav.Title == "Updated" && fav.ShortcutKey != nil && *fav.ShortcutKey == 3
	})).Return(nil)

	uc := usecase.NewManageFavoritesUseCase(favoriteRepo, tagRepo)
	updated, err := uc.UpdateFavorite(ctx, dto.FavoriteUpdateInput{
		ID:    42,
		Title: "Updated",
	})
	require.NoError(t, err)
	require.NotNil(t, updated.ShortcutKey)
	assert.Equal(t, 3, *updated.ShortcutKey)
}

func TestManageFavoritesUseCase_UpdateFavoriteClearsShortcutWhenExplicit(t *testing.T) {
	ctx := testContext()
	shortcut := 3
	existing := &entity.Favorite{
		ID:          42,
		URL:         "https://example.com",
		Title:       "Example",
		ShortcutKey: &shortcut,
	}

	favoriteRepo := repomocks.NewMockFavoriteRepository(t)
	tagRepo := repomocks.NewMockTagRepository(t)
	favoriteRepo.EXPECT().FindByID(mock.Anything, entity.FavoriteID(42)).Return(existing, nil)
	favoriteRepo.EXPECT().Save(mock.Anything, mock.MatchedBy(func(fav *entity.Favorite) bool {
		return fav.ID == 42 && fav.Title == "Updated" && fav.ShortcutKey == nil
	})).Return(nil)

	uc := usecase.NewManageFavoritesUseCase(favoriteRepo, tagRepo)
	updated, err := uc.UpdateFavorite(ctx, dto.FavoriteUpdateInput{
		ID:             42,
		Title:          "Updated",
		ShortcutKeySet: true,
	})
	require.NoError(t, err)
	assert.Nil(t, updated.ShortcutKey)
}

func TestManageFavoritesUseCase_FilterForOmnibox_RanksPrefixBeforeContains(t *testing.T) {
	ctx := testContext()

	favoriteRepo := repomocks.NewMockFavoriteRepository(t)
	tagRepo := repomocks.NewMockTagRepository(t)

	favorites := []*entity.Favorite{
		{ID: 1, URL: "https://example.com/landing", Title: "GitHub docs mirror", Position: 0},
		{ID: 2, URL: "https://github.com/bnema/dumber", Title: "Project repo", Position: 1},
		{ID: 3, URL: "https://gitlab.com/team/project", Title: "GitLab", Position: 2},
	}
	favoriteRepo.EXPECT().GetAll(mock.Anything).Return(favorites, nil)

	uc := usecase.NewManageFavoritesUseCase(favoriteRepo, tagRepo)

	results, err := uc.FilterForOmnibox(ctx, "git")
	require.NoError(t, err)
	require.Len(t, results, 3)
	assert.Equal(t, "https://github.com/bnema/dumber", results[0].URL)
	assert.Equal(t, "https://gitlab.com/team/project", results[1].URL)
	assert.Equal(t, "https://example.com/landing", results[2].URL)
}

func TestManageFavoritesUseCase_FilterForOmnibox_EmptyQueryKeepsRepositoryOrder(t *testing.T) {
	ctx := testContext()

	favoriteRepo := repomocks.NewMockFavoriteRepository(t)
	tagRepo := repomocks.NewMockTagRepository(t)

	favorites := []*entity.Favorite{
		{ID: 1, URL: "https://first.example"},
		{ID: 2, URL: "https://second.example"},
	}
	favoriteRepo.EXPECT().GetAll(mock.Anything).Return(favorites, nil)

	uc := usecase.NewManageFavoritesUseCase(favoriteRepo, tagRepo)

	results, err := uc.FilterForOmnibox(ctx, "")
	require.NoError(t, err)
	require.Len(t, results, 2)
	assert.Equal(t, "https://first.example", results[0].URL)
	assert.Equal(t, "https://second.example", results[1].URL)
}

func TestManageFavoritesUseCase_GetAllURLs_UsesCache(t *testing.T) {
	ctx := testContext()

	favoriteRepo := repomocks.NewMockFavoriteRepository(t)
	tagRepo := repomocks.NewMockTagRepository(t)

	favorites := []*entity.Favorite{
		{ID: 1, URL: "https://example.com"},
	}
	favoriteRepo.EXPECT().GetAll(mock.Anything).Return(favorites, nil).Once()

	uc := usecase.NewManageFavoritesUseCase(favoriteRepo, tagRepo)

	urls, err := uc.GetAllURLs(ctx)
	require.NoError(t, err)
	require.Contains(t, urls, "https://example.com")

	urls, err = uc.GetAllURLs(ctx)
	require.NoError(t, err)
	require.Contains(t, urls, "https://example.com")
}

func TestManageFavoritesUseCase_Toggle_InvalidatesCache(t *testing.T) {
	ctx := testContext()

	favoriteRepo := repomocks.NewMockFavoriteRepository(t)
	tagRepo := repomocks.NewMockTagRepository(t)

	initialFavorites := []*entity.Favorite{
		{ID: 1, URL: "https://cached.example"},
	}
	favoriteRepo.EXPECT().GetAll(mock.Anything).Return(initialFavorites, nil).Once()
	favoriteRepo.EXPECT().FindByURL(mock.Anything, "https://new.example").Return(nil, nil)
	favoriteRepo.EXPECT().Save(mock.Anything, mock.MatchedBy(func(f *entity.Favorite) bool {
		return f.URL == "https://new.example"
	})).Return(nil)
	updatedFavorites := []*entity.Favorite{
		{ID: 1, URL: "https://cached.example"},
		{ID: 2, URL: "https://new.example"},
	}
	favoriteRepo.EXPECT().GetAll(mock.Anything).Return(updatedFavorites, nil).Once()

	uc := usecase.NewManageFavoritesUseCase(favoriteRepo, tagRepo)

	urls, err := uc.GetAllURLs(ctx)
	require.NoError(t, err)
	require.Contains(t, urls, "https://cached.example")

	_, err = uc.Toggle(ctx, "https://new.example", "New")
	require.NoError(t, err)

	urls, err = uc.GetAllURLs(ctx)
	require.NoError(t, err)
	require.Contains(t, urls, "https://new.example")
}

func TestManageFavoritesUseCase_AddFavoriteNormalizesURL(t *testing.T) {
	ctx := testContext()

	favoriteRepo := repomocks.NewMockFavoriteRepository(t)
	tagRepo := repomocks.NewMockTagRepository(t)

	favoriteRepo.EXPECT().FindByURL(mock.Anything, "https://example.com").Return(nil, nil).Once()
	favoriteRepo.EXPECT().FindByURL(mock.Anything, "example.com").Return(nil, nil).Once()
	favoriteRepo.EXPECT().Save(mock.Anything, mock.MatchedBy(func(fav *entity.Favorite) bool {
		return fav != nil && fav.URL == "https://example.com" && fav.Title == "Example"
	})).Return(nil).Once()

	uc := usecase.NewManageFavoritesUseCase(favoriteRepo, tagRepo)

	fav, err := uc.AddFavorite(ctx, dto.FavoriteCreateInput{URL: "example.com", Title: " Example "})
	require.NoError(t, err)
	require.NotNil(t, fav)
	assert.Equal(t, "https://example.com", fav.URL)
	assert.Equal(t, "Example", fav.Title)
}

func TestManageFavoritesUseCase_AddFavoriteRejectsInvalidTagIDs(t *testing.T) {
	ctx := testContext()

	favoriteRepo := repomocks.NewMockFavoriteRepository(t)
	tagRepo := repomocks.NewMockTagRepository(t)
	uc := usecase.NewManageFavoritesUseCase(favoriteRepo, tagRepo)

	_, err := uc.AddFavorite(ctx, dto.FavoriteCreateInput{URL: "https://example.com", Tags: []entity.TagID{1, 0}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "favorite tag id must be positive")
}

func TestManageFavoritesUseCase_AddFavoriteAssignsTagsWithoutFolder(t *testing.T) {
	ctx := testContext()

	favoriteRepo := repomocks.NewMockFavoriteRepository(t)
	tagRepo := repomocks.NewMockTagRepository(t)

	favoriteRepo.EXPECT().FindByURL(mock.Anything, "https://example.com").Return(nil, nil).Once()
	favoriteRepo.EXPECT().Save(mock.Anything, mock.MatchedBy(func(fav *entity.Favorite) bool {
		return fav != nil && fav.URL == "https://example.com" && fav.Title == "Example" && fav.FaviconURL == "https://example.com/favicon.ico"
	})).Run(func(_ context.Context, fav *entity.Favorite) {
		fav.ID = entity.FavoriteID(42)
	}).Return(nil).Once()
	tagRepo.EXPECT().AssignToFavorite(mock.Anything, entity.TagID(7), entity.FavoriteID(42)).Return(nil).Once()
	tagRepo.EXPECT().AssignToFavorite(mock.Anything, entity.TagID(8), entity.FavoriteID(42)).Return(nil).Once()

	uc := usecase.NewManageFavoritesUseCase(favoriteRepo, tagRepo)

	fav, err := uc.AddFavorite(ctx, dto.FavoriteCreateInput{
		URL:        "https://example.com",
		Title:      "Example",
		FaviconURL: " https://example.com/favicon.ico ",
		Tags:       []entity.TagID{7, 8},
	})
	require.NoError(t, err)
	require.NotNil(t, fav)
	assert.Equal(t, entity.FavoriteID(42), fav.ID)
}

func TestManageFavoritesUseCase_AddFavoriteReturnsAssignTagError(t *testing.T) {
	ctx := testContext()

	favoriteRepo := repomocks.NewMockFavoriteRepository(t)
	tagRepo := repomocks.NewMockTagRepository(t)

	favoriteRepo.EXPECT().FindByURL(mock.Anything, "https://example.com").Return(nil, nil).Once()
	favoriteRepo.EXPECT().Save(mock.Anything, mock.Anything).Run(func(_ context.Context, fav *entity.Favorite) {
		fav.ID = entity.FavoriteID(42)
	}).Return(nil).Once()
	tagRepo.EXPECT().AssignToFavorite(mock.Anything, entity.TagID(7), entity.FavoriteID(42)).Return(errors.New("tag missing")).Once()

	uc := usecase.NewManageFavoritesUseCase(favoriteRepo, tagRepo)

	fav, err := uc.AddFavorite(ctx, dto.FavoriteCreateInput{
		URL:   "https://example.com",
		Title: "Example",
		Tags:  []entity.TagID{7},
	})
	require.Error(t, err)
	assert.Nil(t, fav)
	assert.Contains(t, err.Error(), "failed to assign tag 7 to favorite 42")
}

func TestManageFavoritesUseCase_AddFavoriteRejectsUnsafeOrMalformedURL(t *testing.T) {
	ctx := testContext()

	tests := []string{
		"",
		"not a url",
		"javascript:alert(1)",
		"data:text/html,<script>alert(1)</script>",
		"https:///missing-host",
		"dumb:///missing-host",
	}
	for _, raw := range tests {
		t.Run(raw, func(t *testing.T) {
			favoriteRepo := repomocks.NewMockFavoriteRepository(t)
			tagRepo := repomocks.NewMockTagRepository(t)
			uc := usecase.NewManageFavoritesUseCase(favoriteRepo, tagRepo)

			_, err := uc.AddFavorite(ctx, dto.FavoriteCreateInput{URL: raw})
			require.Error(t, err)
		})
	}
}

func TestManageFavoritesUseCase_AddFavoriteNormalizesOpaqueDumbRoute(t *testing.T) {
	ctx := testContext()

	favoriteRepo := repomocks.NewMockFavoriteRepository(t)
	tagRepo := repomocks.NewMockTagRepository(t)

	favoriteRepo.EXPECT().FindByURL(mock.Anything, "dumb://history").Return(nil, nil).Once()
	favoriteRepo.EXPECT().FindByURL(mock.Anything, "dumb:history").Return(nil, nil).Once()
	favoriteRepo.EXPECT().Save(mock.Anything, mock.MatchedBy(func(fav *entity.Favorite) bool {
		return fav != nil && fav.URL == "dumb://history" && fav.Title == "History"
	})).Return(nil).Once()

	uc := usecase.NewManageFavoritesUseCase(favoriteRepo, tagRepo)

	fav, err := uc.AddFavorite(ctx, dto.FavoriteCreateInput{URL: "dumb:history", Title: "History"})
	require.NoError(t, err)
	require.NotNil(t, fav)
	assert.Equal(t, "dumb://history", fav.URL)
}
